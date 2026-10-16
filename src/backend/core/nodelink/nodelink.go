// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package nodelink carries one node's agent calls to the node that holds the agent.
//
// A remote agent keeps a single outbound WebSocket to a single McHarbor node. In
// active/passive that is always the active node, so nothing else is needed. In
// active/active any node may serve any request, and a request that needs an agent the
// serving node does not hold has to reach the node that does.
//
//	node B (serving)                      node A (holds the agent socket)
//	 docker SDK                            /api/internal/node/agent-rpc/{envId}
//	   └─ RoutingTransport ──HTTPS+HMAC──►  └─ AgentTransport ──WS──► agent ──► daemon
//
// Three decisions are worth knowing before reading the rest:
//
// **Forwarding, not a bus.** The instance registry already knows which nodes are alive
// and `cluster_agent_owners` records which one holds each agent, as a lease. One HTTP
// hop needs no new infrastructure and fails in a way an operator can read.
//
// **The hop is the Docker HTTP request itself.** The agent transport is an
// http.RoundTripper, so a forwarded call is the same request with a different
// RoundTripper under it. Nothing about the Docker API has to be modelled here, and a
// streaming response (logs, events, stats) streams through the hop unchanged.
//
// **Node identity, not user identity, authenticates the hop.** A node that holds
// MCHARBOR_SECRET already holds the database; there is no privilege for the receiving
// node to grant that the asking node did not have. So the receiving node verifies that
// the caller *is* a node and records whose request it was, and does not re-run the
// permission check the asking node already ran. What it does refuse is forwarding
// onwards: a node that does not own the agent answers 409 rather than chaining a hop.
//
// Every path that is not an HTTP round trip — the terminal and host-terminal `exec_*`
// messages, the compute console, the host log stream, agent-side Compose and prune,
// transfers and every other request/reply message — crosses a second hop: a signed
// node-to-node WebSocket over which the asking node runs an ordinary agent transport and
// the owning node relays frames verbatim (stream.go states the ordering, backpressure and
// close contract). The stats push stays with the owning node, whose stats hub the other
// nodes read (stats.go).
package nodelink

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/agent"
)

// LocalAgents is the slice of the agent pool the link needs: the transport for an agent
// connected to this node, or nil. *agent.AgentPool satisfies it.
type LocalAgents interface {
	Transport(envID string) *agent.AgentTransport
	List() []agent.AgentConnection
}

// Link is this node's half of the node-to-node agent link.
type Link struct {
	owners *Owners
	signer *Signer
	local  LocalAgents
	client *http.Client
	logger *slog.Logger

	mu          sync.Mutex
	proof       Proof
	streamProof Proof

	// streamMu guards the stream links: the virtual connections this node dials to
	// agents other nodes hold, and the relays it runs for other nodes.
	streamMu sync.Mutex
	remotes  map[string]*remoteAgent
	dialing  map[string]chan struct{}
	failedAt map[string]time.Time
	relays   map[*relay]struct{}

	// fireMu guards the workflow-trigger fire hop's wiring (trigger_fire.go).
	fireMu       sync.Mutex
	peers        PeerSource
	fireReceiver TriggerFireReceiver
	lastLeader   string
	peerCache    []Peer
	peerCacheAt  time.Time
	// peerRefreshAt is when an unknown signing node last forced a fresh registry read.
	peerRefreshAt time.Time

	// requireTLS refuses to send a signed call to a peer advertising plain http, unless
	// the operator opted in with MCHARBOR_NODE_LINK_ALLOW_HTTP (plaintext.go).
	requireTLS atomic.Bool
	refusals   *refusalLog
	// tlsCfg is the client TLS configuration both hops dial with, built once in New.
	tlsCfg *tls.Config
	// admission holds the route budgets modules/nodelink installs (admission.go).
	admission atomic.Pointer[admissionBox]
}

// New builds the link. A nil signer (no MCHARBOR_SECRET) leaves it unable to forward,
// which the readiness gate reports rather than discovering at the first request. The link
// requires TLS to every peer from the start, in every HA mode; only
// MCHARBOR_NODE_LINK_ALLOW_HTTP=1 lets it dial an http:// peer.
func New(owners *Owners, signer *Signer, local LocalAgents, logger *slog.Logger) *Link {
	tlsCfg, tlsErr := buildNodeTLSConfig(os.Getenv(EnvCAFile), os.Getenv(EnvSkipVerify) == "1")
	if tlsErr != nil && logger != nil {
		logger.Error("nodelink: "+EnvCAFile+" cannot be used, so no peer certificate verifies and every node-to-node call from this node fails",
			"error", tlsErr)
	}
	l := &Link{
		owners: owners,
		signer: signer,
		local:  local,
		logger: logger,
		client: &http.Client{
			// A signed call is never re-sent elsewhere: a 307/308 would carry the body and
			// every X-Mcharbor-Node-* header to whatever host the peer names, so the
			// redirect itself comes back and the caller treats it as a refusal.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			// No client timeout: the per-request deadline is the caller's, forwarded
			// in a header and applied on both sides, and a fixed ceiling here would
			// cut an image pull short.
			// Proxy is nil on purpose, as on the stream dialer: a node-to-node call goes
			// straight to the peer's advertise URL, never through HTTP(S)_PROXY, so a proxy
			// configured for outbound internet traffic never sees a signed call.
			Transport: &http.Transport{
				Proxy:               nil,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     tlsCfg,
			},
		},
		proof:       Proof{Status: ProofUnproven},
		streamProof: Proof{Status: ProofUnproven},
		remotes:     map[string]*remoteAgent{},
		dialing:     map[string]chan struct{}{},
		failedAt:    map[string]time.Time{},
		relays:      map[*relay]struct{}{},
		refusals:    newRefusalLog(),
		tlsCfg:      tlsCfg,
	}
	l.requireTLS.Store(true)
	return l
}

// EnvCAFile names a PEM file of the private CA that signs the nodes' certificates. When set,
// both hops verify peers against it alone, and it wins over EnvSkipVerify.
const EnvCAFile = "MCHARBOR_NODE_LINK_CA_FILE"

// EnvSkipVerify is the operator's opt-out of peer certificate verification. It removes the
// only integrity streamed bodies, agent reply bodies and relayed sessions have, so the
// readiness gate warns about it and EnvCAFile is the recommended alternative.
const EnvSkipVerify = "MCHARBOR_NODE_LINK_SKIP_VERIFY"

// SkipVerifyEnabled reports whether this node accepts any peer certificate: EnvSkipVerify=1
// with no EnvCAFile, since a pinned CA always wins.
func SkipVerifyEnabled() bool {
	return strings.TrimSpace(os.Getenv(EnvSkipVerify)) == "1" && strings.TrimSpace(os.Getenv(EnvCAFile)) == ""
}

// CAFileProblem returns why EnvCAFile cannot be used, or nil when it is unset or loads.
func CAFileProblem() error {
	_, err := loadCAPool(os.Getenv(EnvCAFile))
	return err
}

// loadCAPool reads the pinned CA file; an empty path is no pin, and a file with no PEM
// certificate in it is an error rather than an empty pin.
func loadCAPool(path string) (*x509.CertPool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-configured CA file path
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", EnvCAFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("%s at %s holds no PEM certificate", EnvCAFile, path)
	}
	return pool, nil
}

// buildNodeTLSConfig is the client TLS configuration for both hops. A pinned CA file makes
// the peer verify against that CA only; a CA file that cannot be loaded leaves an empty pool,
// so nothing verifies (fail closed). Without one, verification is on unless the operator
// opts out, which is their statement that the path between the nodes is already trusted.
func buildNodeTLSConfig(caFile string, skipVerify bool) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	pool, err := loadCAPool(caFile)
	if err != nil {
		cfg.RootCAs = x509.NewCertPool()
		return cfg, err
	}
	if pool != nil {
		cfg.RootCAs = pool
		return cfg, nil
	}
	cfg.InsecureSkipVerify = skipVerify // #nosec G402 -- operator opt-in for a private node-to-node path; the readiness gate warns
	return cfg, nil
}

// tlsConfig is the stream dialer's TLS configuration, the same as the HTTP hop's.
func (l *Link) tlsConfig() *tls.Config {
	if l.tlsCfg == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return l.tlsCfg.Clone()
}

// Owners exposes the lease store so the cluster status can show who holds what.
func (l *Link) Owners() *Owners { return l.owners }

// NodeID is this node's identity in the link.
func (l *Link) NodeID() string {
	if l == nil || l.owners == nil {
		return ""
	}
	return l.owners.NodeID()
}

// CanForward reports whether this node is configured to forward and to be forwarded to:
// a shared secret to sign with, and an address other nodes can reach it on.
func (l *Link) CanForward() bool {
	return l != nil && l.signer != nil && l.owners != nil && strings.TrimSpace(l.owners.AdvertiseURL()) != ""
}

// Claim records that this node now holds envID's agent connection.
func (l *Link) Claim(ctx context.Context, envID string) error {
	if l == nil || l.owners == nil {
		return nil
	}
	return l.owners.Claim(ctx, envID)
}

// Release drops this node's claim on envID's agent connection.
func (l *Link) Release(ctx context.Context, envID string) error {
	if l == nil || l.owners == nil {
		return nil
	}
	return l.owners.Release(ctx, envID)
}

// StartLeaseRenewal renews the claim on every locally connected agent until ctx ends.
//
// The renewal is what makes the claim a lease: a node that stops running stops renewing,
// and the agent's next node takes the row over once it expires.
func (l *Link) StartLeaseRenewal(ctx context.Context) {
	if l == nil || l.owners == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(OwnerRenewEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Indexed rather than ranged: AgentConnection carries a mutex
				// and a range variable would copy it.
				connected := l.local.List()
				for i := range connected {
					envID := connected[i].EnvID
					if err := l.owners.Claim(ctx, envID); err != nil {
						l.logger.Warn("nodelink: renewing the agent claim failed", "env", envID, "error", err)
					}
				}
			}
		}
	}()
}

// StartNonceSweep keeps the signer's replay store bounded until ctx ends.
func (l *Link) StartNonceSweep(ctx context.Context) {
	if l == nil || l.signer == nil {
		return
	}
	l.signer.StartNonceSweep(ctx)
}

// Transport returns the round tripper for envID: the local agent transport while this
// node holds the connection, and the forwarding hop while another node does.
func (l *Link) Transport(envID string) http.RoundTripper {
	return &RoutingTransport{link: l, envID: envID}
}

// RemoteOwner returns the live claim on envID when it belongs to another node.
func (l *Link) RemoteOwner(ctx context.Context, envID string) (Owner, bool) {
	if l == nil || l.owners == nil {
		return Owner{}, false
	}
	owner, ok, err := l.owners.Owner(ctx, envID)
	if err != nil {
		l.logger.Warn("nodelink: reading the agent owner failed", "env", envID, "error", err)
		return Owner{}, false
	}
	if !ok || owner.NodeID == l.owners.NodeID() {
		return Owner{}, false
	}
	return owner, true
}
