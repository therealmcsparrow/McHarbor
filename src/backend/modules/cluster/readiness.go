// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

// Readiness targets accepted by GET /api/cluster/readiness.
const (
	TargetActiveActive  = "active-active"
	TargetActivePassive = "active-passive"
)

// ModeActiveActive is the MCHARBOR_HA_MODE value that lets every node serve every
// request. It resolves to itself since cross-node agent routing landed; whether it is
// usable is still decided per node by the readiness gates, not by the variable.
const ModeActiveActive = "active-active"

// Gate verdicts. A warning is shown but does not block the target.
const (
	GatePass = "pass"
	GateWarn = "warn"
	GateFail = "fail"
)

// Gate is one readiness condition with the reason it holds or not and what to do.
type Gate struct {
	ID          string   `json:"id"`
	Status      string   `json:"status"`
	Detail      string   `json:"detail"`
	Remediation string   `json:"remediation,omitempty"`
	Values      []string `json:"values,omitempty"`
}

// Readiness is the verdict for one HA target; Ready means no gate failed.
type Readiness struct {
	Target string `json:"target"`
	Ready  bool   `json:"ready"`
	// Enabled says whether a mode value can select this target today. Active/active
	// stays false while any gate fails, and agent routing always does.
	Enabled bool   `json:"enabled"`
	Gates   []Gate `json:"gates"`
}

// ReadinessInputs is everything the gates read, gathered once so the evaluation is
// a pure function the tests can drive.
type ReadinessInputs struct {
	Driver        string
	KeyFromEnv    bool
	KeyMatches    bool
	KeyCheckErr   bool
	PluginStore   string
	Instances     []Instance
	RegistryOK    bool
	CoordinatorOK bool
	DatabaseOK    bool
	// RoutingReady says this node is configured to forward an agent call to the node
	// that holds the agent: a shared secret to sign with, and an address peers reach
	// it on.
	RoutingReady bool
	// RoutingProof is what the link last actually learned, not what it is configured
	// to do, so the gate cannot pass on an assertion.
	RoutingProof nodelink.Proof
	// StreamProof is the same for the session relay that exec, the host terminal, the
	// guest console, host logs and agent messages take: a ping that went through it.
	StreamProof nodelink.Proof
	// SharedTickets says the console ticket table answered from this node.
	SharedTickets bool
	// PlaintextAllowed is the operator's MCHARBOR_NODE_LINK_ALLOW_HTTP opt-in to node
	// calls over plain http.
	PlaintextAllowed bool
	// SkipVerify is this node's MCHARBOR_NODE_LINK_SKIP_VERIFY=1 with no pinned CA file: it
	// accepts any peer certificate.
	SkipVerify bool
	// CAFileError is why this node's MCHARBOR_NODE_LINK_CA_FILE cannot be used, or empty.
	CAFileError string
}

// ResolveHAMode maps the configured mode to the one this build runs.
//
// Active/active used to be downgraded here because its agent-routing gate could not pass.
// It now resolves to itself: the gates decide, and a node whose gates fail says so
// through /api/cluster/readiness rather than through a silent change of mode.
func ResolveHAMode(requested string) (mode string, refused bool) {
	return strings.TrimSpace(requested), false
}

// EvaluateReadiness applies every gate for target to in.
func EvaluateReadiness(target string, in ReadinessInputs) Readiness {
	aa := target == TargetActiveActive
	gates := []Gate{
		databaseGate(in),
		encryptionKeyGate(in),
		pluginStoreGate(in),
		versionGate(in),
		agentRoutingGate(aa, in),
		nodeLinkTransportGate(in),
		{ID: "sessions", Status: GatePass,
			Detail: "Sessions are stored in the shared database, so any node can serve any request without sticky sessions."},
		perNodeStateGate(aa, in),
		coordinatorGate(in),
	}
	r := Readiness{Target: target, Ready: true, Gates: gates}
	for _, g := range gates {
		if g.Status == GateFail {
			r.Ready = false
		}
	}
	// Every gate, and nothing else. Active/active is no longer refused by construction;
	// it is refused by whichever gate fails.
	r.Enabled = r.Ready
	return r
}

// sharedDatabaseDrivers names the backends two nodes can point at the same copy of. It is
// keyed on the driver string the status payload carries rather than on db.Driver, so a new
// backend has to be added here deliberately instead of inheriting readiness.
var sharedDatabaseDrivers = map[string]bool{
	string(db.DriverPostgres): true,
	string(db.DriverMySQL):    true,
	string(db.DriverMSSQL):    true,
}

func databaseGate(in ReadinessInputs) Gate {
	g := Gate{ID: "externalDatabase", Values: []string{in.Driver}}
	switch {
	case !sharedDatabaseDrivers[in.Driver]:
		g.Status, g.Detail = GateFail, "SQLite is a file on one node and cannot be shared."
		g.Remediation = "Set MCHARBOR_DB_DRIVER=postgres, mysql or mssql and MCHARBOR_DB_DSN on every node, then import the SQLite data."
	case !in.DatabaseOK:
		g.Status, g.Detail = GateFail, "The shared database does not answer from this node."
		g.Remediation = "Check the DSN, network path and credentials from this node."
	default:
		g.Status, g.Detail = GatePass, "The database driver is supported for the whole runtime and answers."
	}
	return g
}

func encryptionKeyGate(in ReadinessInputs) Gate {
	g := Gate{ID: "encryptionKey"}
	switch {
	case in.KeyCheckErr:
		g.Status, g.Detail = GateFail, "The key check row could not be read."
		g.Remediation = "Check database connectivity; the row lives in cluster_state."
	case !in.KeyMatches:
		g.Status, g.Detail = GateFail, "This node's encryption key is not the one the database was written with."
		g.Remediation = "Set the same ENCRYPTION_KEY (or ENCRYPTION_KEY_FILE) on every node; MCHARBOR_RESET_KEY_CHECK=1 accepts a deliberate key change."
	case !in.KeyFromEnv:
		g.Status, g.Detail = GateFail, "The key was generated in DATA_DIR, so another node cannot hold the same one."
		g.Remediation = "Copy the key into ENCRYPTION_KEY or a file named by ENCRYPTION_KEY_FILE, and set it on every node."
	default:
		g.Status, g.Detail = GatePass, "The key comes from ENCRYPTION_KEY or ENCRYPTION_KEY_FILE and matches the key check row every node verifies at boot."
	}
	return g
}

func pluginStoreGate(in ReadinessInputs) Gate {
	g := Gate{ID: "pluginStore", Values: []string{in.PluginStore}}
	if in.PluginStore == "" || in.PluginStore == "local" {
		g.Status, g.Detail = GateFail, "Plugin packages live only on the node that installed them."
		g.Remediation = "Set MCHARBOR_PLUGIN_STORE to database, shared-fs or object storage on every node."
		return g
	}
	g.Status, g.Detail = GatePass, "Every node can fetch installed plugin packages."
	return g
}

func versionGate(in ReadinessInputs) Gate {
	g := Gate{ID: "sameVersion"}
	if !in.RegistryOK {
		g.Status, g.Detail = GateWarn, "The instance registry could not be read, so node versions are unknown."
		return g
	}
	seen := map[string]bool{}
	for _, inst := range in.Instances {
		if inst.Alive {
			seen[inst.Version+"+"+inst.Build] = true
		}
	}
	for v := range seen {
		g.Values = append(g.Values, v)
	}
	sort.Strings(g.Values)
	if len(seen) > 1 {
		g.Status, g.Detail = GateFail, "Live nodes run different versions or builds."
		g.Remediation = "Upgrade every node to the same image before serving from more than one."
		return g
	}
	g.Status, g.Detail = GatePass, "Every live node runs the same version and build."
	return g
}

// agentRoutingGate reports whether an agent held by another node is reachable from here.
//
// It reads the proof the link records from real forwarded calls, never the configuration:
// a gate that passed because forwarding was wired would be an assertion, and the reason
// active/active was refused is precisely that nobody could prove the hop worked.
func agentRoutingGate(activeActive bool, in ReadinessInputs) Gate {
	g := Gate{ID: "agentRouting"}
	if !activeActive {
		g.Status, g.Detail = GatePass, "Only the active node serves, and every agent reconnects to it."
		return g
	}
	if !in.RoutingReady {
		g.Status = GateFail
		g.Detail = "This node cannot forward an agent call to the node holding the agent: it has no shared secret to sign with, or no address peers can reach it on."
		g.Remediation = "Set the same MCHARBOR_SECRET on every node, and MCHARBOR_ADVERTISE_URL to the address the other nodes reach this one on."
		return g
	}
	switch in.RoutingProof.Status {
	case nodelink.ProofFailed:
		g.Status = GateFail
		g.Detail = "The last cross-node agent call failed: " + in.RoutingProof.Error
		g.Remediation = "Check that MCHARBOR_ADVERTISE_URL on the owning node resolves and its certificate is trusted from here, then re-run POST /api/cluster/agent-routing-check."
		g.Values = []string{in.RoutingProof.NodeID, in.RoutingProof.EnvID}
		return g
	case nodelink.ProofProven:
		return provenRoutingGate(g, in)
	}
	g.Status = GateWarn
	g.Detail = "Cross-node agent routing is configured and has not been exercised yet: no agent is currently held by another node, so nothing has been proven."
	g.Remediation = "Connect an agent to another node and run POST /api/cluster/agent-routing-check."
	return g
}

// provenRoutingGate grades a node whose Docker calls already reach a remote agent by the
// second hop: it passes only when a ping through the session relay has come back too.
func provenRoutingGate(g Gate, in ReadinessInputs) Gate {
	pinned := "/api/terminal/ws, /api/system/host-terminal/ws, /api/compute/console/ and /api/system/host-logs"
	switch in.StreamProof.Status {
	case nodelink.ProofProven:
		g.Status = GatePass
		g.Detail = "Agent calls reach an agent on another node over both hops: Docker API calls were executed by node " +
			in.RoutingProof.NodeID + ", and a ping through the session relay that carries terminal exec, the host terminal, " +
			"the guest console, host logs and agent messages came back from node " + in.StreamProof.NodeID + "."
		g.Values = []string{in.StreamProof.NodeID}
		return g
	case nodelink.ProofFailed:
		g.Status = GateWarn
		g.Detail = "Docker API calls reach an agent on another node, but the session relay to node " + in.StreamProof.NodeID +
			" failed: " + in.StreamProof.Error + ". Terminal exec, the host terminal, the guest console, host logs and agent messages work only from the node holding the agent until it does."
		g.Remediation = "Let the path between the nodes carry a WebSocket upgrade to GET /api/internal/node/agent-stream/{envId} on the owning node's MCHARBOR_ADVERTISE_URL " +
			"(a proxy in between must pass the Upgrade and Connection headers), then re-run POST /api/cluster/agent-routing-check. Until then route " +
			pinned + " to the node holding the agent."
		g.Values = []string{in.StreamProof.NodeID, in.StreamProof.EnvID}
		return g
	}
	g.Status = GateWarn
	g.Detail = "Docker API calls reach an agent on another node: a call to environment " + in.RoutingProof.EnvID +
		" was executed by node " + in.RoutingProof.NodeID + ". The session relay for terminal exec, the host terminal, the guest console, host logs and agent messages has not been exercised yet."
	g.Remediation = "Run POST /api/cluster/agent-routing-check, which pings every remote agent through both hops."
	g.Values = []string{in.RoutingProof.NodeID}
	return g
}

// nodeLinkTransportGate fails, in every multi-node mode, while more than one node is live
// and one of them advertises an address that is not https, or this node's pinned CA file
// cannot be used: peer and active/active nodes forward Docker calls (registry credentials
// included), session relays, stats reads and trigger fires to each other. It warns when an
// opt-out weakens TLS, because streamed bodies, agent reply bodies and relayed sessions have
// no integrity but TLS's, so those opt-outs make them forgeable, not only readable.
func nodeLinkTransportGate(in ReadinessInputs) Gate {
	g := Gate{ID: "nodeLinkTransport"}
	live := 0
	var plaintext []string
	for _, inst := range in.Instances {
		if !inst.Alive {
			continue
		}
		live++
		if nodelink.IsPlaintextURL(inst.AdvertiseURL) {
			plaintext = append(plaintext, inst.ID+" "+inst.AdvertiseURL)
		}
	}
	if live <= 1 {
		g.Status, g.Detail = GatePass, "Only one node is live, so no node-to-node calls are made."
		return g
	}
	const forgeable = "anyone on the path can read them, and can forge streamed request bodies, agent reply bodies and relayed terminal, console and log sessions, which only TLS protects"
	var fails, warns, fixes []string
	if len(plaintext) > 0 {
		g.Values = append(g.Values, plaintext...)
		if in.PlaintextAllowed {
			warns = append(warns, "Some live nodes advertise plain http and "+nodelink.EnvAllowHTTP+"=1 lets node-to-node calls reach them unencrypted: "+forgeable+".")
			fixes = append(fixes, "Keep "+nodelink.EnvAllowHTTP+" only on a private network you trust; otherwise give each node an https MCHARBOR_ADVERTISE_URL.")
		} else {
			fails = append(fails, "Some live nodes advertise an address that is not https, so the other nodes refuse to send them node-to-node calls: forwarded Docker calls, session relays, stats reads and workflow-trigger fires.")
			fixes = append(fixes, "Set MCHARBOR_ADVERTISE_URL to an https address on those nodes, or set "+nodelink.EnvAllowHTTP+"=1 on every node for a trusted private network.")
		}
	}
	if in.CAFileError != "" {
		g.Values = append(g.Values, nodelink.EnvCAFile)
		fails = append(fails, nodelink.EnvCAFile+" cannot be used on this node ("+in.CAFileError+"), so no peer certificate verifies and every node-to-node call it sends fails.")
		fixes = append(fixes, "Point "+nodelink.EnvCAFile+" at a readable PEM file holding the CA that signs the nodes' certificates, or unset it.")
	} else if in.SkipVerify {
		g.Values = append(g.Values, nodelink.EnvSkipVerify+"=1")
		warns = append(warns, nodelink.EnvSkipVerify+"=1 makes this node accept any certificate a peer presents, so whoever can intercept the path can stand in for a node: "+forgeable+".")
		fixes = append(fixes, "Pin the CA that signs the nodes' certificates with "+nodelink.EnvCAFile+" and remove "+nodelink.EnvSkipVerify+".")
	}
	switch {
	case len(fails) > 0:
		g.Status = GateFail
		g.Detail = strings.Join(append(fails, warns...), " ")
	case len(warns) > 0:
		g.Status = GateWarn
		g.Detail = strings.Join(warns, " ")
	default:
		g.Status, g.Detail = GatePass, "Every live node advertises an https address for node-to-node calls, and this node verifies their certificates."
	}
	g.Remediation = strings.Join(fixes, " ")
	return g
}

// perNodeStateGate reports the state that is still per node, and says which of it is a
// deliberate choice rather than a gap.
func perNodeStateGate(activeActive bool, in ReadinessInputs) Gate {
	g := Gate{ID: "perNodeState"}
	if !activeActive {
		g.Status, g.Detail = GatePass, "One node serves, so in-memory tickets and rate limits behave as on a single node."
		return g
	}
	if !in.SharedTickets {
		g.Status = GateFail
		g.Detail = "The console ticket table did not answer from this node, so a console opened here could not be redeemed elsewhere."
		g.Remediation = "Check the shared database from this node; the tickets live in console_tickets."
		return g
	}
	g.Status = GateWarn
	g.Detail = "Console tickets are in the shared database, so any node can redeem a ticket any node issued. Rate-limit counters and the console viewer cap stay per node by design: a shared counter would cost a database round trip on every request to buy a limit nobody asked to be exact."
	g.Remediation = "Size the auth rate limit per node, not per cluster: the effective limit is the configured one times the number of nodes."
	return g
}

func coordinatorGate(in ReadinessInputs) Gate {
	g := Gate{ID: "coordinator"}
	if in.Driver == string(db.DriverSQLite) || !in.CoordinatorOK {
		g.Status, g.Detail = GateFail, "The lock session that elects the active node and singleton jobs is not available."
		g.Remediation = "Use an external database; the coordinator holds its locks there."
		return g
	}
	g.Status, g.Detail = GatePass, "The lock session answers."
	return g
}

// SetKeyHasher gives the readiness check the node's key digest to compare against the
// key check row.
func (s *Service) SetKeyHasher(k KeyHasher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = k
}

// Readiness gathers the inputs and evaluates target.
func (s *Service) Readiness(ctx context.Context, target string) Readiness {
	ctx, cancel := context.WithTimeout(ctx, clusterCallTimeout)
	defer cancel()
	s.mu.Lock()
	keys, store, reg, link := s.keys, s.pkgStore, s.registry, s.link
	s.mu.Unlock()
	in := ReadinessInputs{
		Driver:      string(db.DriverOf()),
		KeyFromEnv:  s.keyIsPinned(),
		PluginStore: store,
		DatabaseOK:  s.pingOK(ctx),
	}
	in.CoordinatorOK = in.Driver != string(db.DriverSQLite) && s.coord != nil && s.coord.Ping(ctx) == nil
	match, err := s.keyMatches(ctx, keys)
	in.KeyMatches, in.KeyCheckErr = match, err != nil
	if err != nil && s.logger != nil {
		s.logger.Warn("cluster: reading the key check row", "error", err)
	}
	if reg != nil {
		list, err := reg.List(ctx)
		in.RegistryOK = err == nil
		in.Instances = list
	}
	in.SharedTickets = s.consoleTicketsShared(ctx)
	in.PlaintextAllowed = nodelink.PlaintextAllowed()
	in.SkipVerify = nodelink.SkipVerifyEnabled()
	if err := nodelink.CAFileProblem(); err != nil {
		in.CAFileError = err.Error()
	}
	if link != nil {
		in.RoutingReady = link.CanForward()
		in.RoutingProof = link.Proof()
		in.StreamProof = link.StreamProof()
	}
	return EvaluateReadiness(target, in)
}

func (s *Service) keyMatches(ctx context.Context, keys KeyHasher) (bool, error) {
	if keys == nil {
		return false, errors.New("cluster: no key hasher attached")
	}
	var stored string
	err := s.database.QueryRowContext(ctx, `SELECT value FROM cluster_state WHERE name = ?`, keyCheckName).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cluster: reading key check: %w", err)
	}
	return stored == keys.StableHash("mcharbor-key-check"), nil
}

// SetNodeLink gives readiness the node-to-node link whose recorded proof the agent
// routing gate reads.
func (s *Service) SetNodeLink(link *nodelink.Link) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.link = link
}

// NodeLink is the link this service was given, nil on a single-node install.
func (s *Service) NodeLink() *nodelink.Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.link
}

// consoleTicketsShared answers the per-node-state gate by reading the table rather than
// trusting that the build has one: a node whose shared database is unreachable would
// otherwise report shared tickets it cannot write.
func (s *Service) consoleTicketsShared(ctx context.Context) bool {
	var count int
	if err := s.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM console_tickets`).Scan(&count); err != nil {
		if s.logger != nil {
			s.logger.Warn("cluster: reading the console ticket table", "error", err)
		}
		return false
	}
	return true
}
