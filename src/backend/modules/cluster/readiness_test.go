// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

func healthyInputs() ReadinessInputs {
	return ReadinessInputs{
		Driver: "postgres", KeyFromEnv: true, KeyMatches: true, PluginStore: "database",
		RegistryOK: true, CoordinatorOK: true, DatabaseOK: true,
		Instances: []Instance{
			{ID: "a", Version: "2026.09.01", Build: "1", Alive: true},
			{ID: "b", Version: "2026.09.01", Build: "1", Alive: true},
			{ID: "old", Version: "2026.08.01", Build: "0", Alive: false},
		},
	}
}

func gateStatus(r Readiness, id string) string {
	for _, g := range r.Gates {
		if g.ID == id {
			return g.Status
		}
	}
	return ""
}

// activeActiveInputs is healthyInputs plus the two things active/active needs beyond
// active/passive: a configured, proven routing hop and shared console tickets.
func activeActiveInputs() ReadinessInputs {
	in := healthyInputs()
	in.RoutingReady = true
	in.SharedTickets = true
	in.RoutingProof = nodelink.Proof{Status: nodelink.ProofProven, NodeID: "a", EnvID: "env-1"}
	return in
}

// TestActiveActiveStaysRefusedWhileRoutingCannotBeProven is the A1 gate test. Routing
// that is merely wired, or wired and failing, must not let the mode enable itself.
func TestActiveActiveStaysRefusedWhileRoutingCannotBeProven(t *testing.T) {
	// No shared secret and no advertised address: the hop cannot exist.
	in := activeActiveInputs()
	in.RoutingReady = false
	r := EvaluateReadiness(TargetActiveActive, in)
	if r.Ready || r.Enabled {
		t.Fatalf("unconfigured routing: ready=%v enabled=%v, want both false", r.Ready, r.Enabled)
	}
	if gateStatus(r, "agentRouting") != GateFail {
		t.Fatalf("unconfigured routing must fail the gate, got %s", gateStatus(r, "agentRouting"))
	}

	// Configured and the last real call failed. Configuration is not proof.
	in = activeActiveInputs()
	in.RoutingProof = nodelink.Proof{Status: nodelink.ProofFailed, NodeID: "a", EnvID: "env-1",
		Error: "dial tcp: connection refused"}
	r = EvaluateReadiness(TargetActiveActive, in)
	if r.Ready || r.Enabled {
		t.Fatalf("failing routing: ready=%v enabled=%v, want both false", r.Ready, r.Enabled)
	}
	if gateStatus(r, "agentRouting") != GateFail {
		t.Fatalf("a failed cross-node call must fail the gate, got %s", gateStatus(r, "agentRouting"))
	}
}

// TestActiveActiveStaysRefusedWithoutSharedTickets is the A2 gate test: a node whose
// shared ticket table does not answer cannot serve consoles for the cluster.
func TestActiveActiveStaysRefusedWithoutSharedTickets(t *testing.T) {
	in := activeActiveInputs()
	in.SharedTickets = false
	r := EvaluateReadiness(TargetActiveActive, in)
	if r.Ready || r.Enabled {
		t.Fatalf("ready=%v enabled=%v, want both false", r.Ready, r.Enabled)
	}
	if gateStatus(r, "perNodeState") != GateFail {
		t.Fatalf("an unreachable ticket table must fail the gate, got %s", gateStatus(r, "perNodeState"))
	}
}

// TestActiveActiveEnablesOnlyWhenEveryGatePasses is the other half of the same contract:
// once both blockers are genuinely closed the mode is allowed, and it is allowed for no
// other reason.
func TestActiveActiveEnablesOnlyWhenEveryGatePasses(t *testing.T) {
	r := EvaluateReadiness(TargetActiveActive, activeActiveInputs())
	if !r.Ready || !r.Enabled {
		t.Fatalf("a proven cluster must be able to run active-active, got %+v", r)
	}
	// Warnings are shown and do not block: a session relay not yet exercised and the
	// per-node rate limits are known caveats, not failures.
	if gateStatus(r, "agentRouting") != GateWarn {
		t.Fatalf("routing proven over one hop only must warn, got %s", gateStatus(r, "agentRouting"))
	}
	if gateStatus(r, "perNodeState") != GateWarn || gateStatus(r, "sessions") != GatePass {
		t.Fatalf("gates = %+v", r.Gates)
	}
	// Every active/passive prerequisite still blocks active/active.
	for _, breakIt := range []func(*ReadinessInputs){
		func(in *ReadinessInputs) { in.Driver = "sqlite" },
		func(in *ReadinessInputs) { in.KeyFromEnv = false },
		func(in *ReadinessInputs) { in.PluginStore = "local" },
		func(in *ReadinessInputs) { in.CoordinatorOK = false },
	} {
		in := activeActiveInputs()
		breakIt(&in)
		if got := EvaluateReadiness(TargetActiveActive, in); got.Ready || got.Enabled {
			t.Errorf("a broken prerequisite must still refuse active-active, got %+v", got)
		}
	}
}

// TestResolveHAModeNoLongerDowngrades records the policy change: the mode value is taken
// at its word and the gates, not the parser, decide whether it works.
func TestResolveHAModeNoLongerDowngrades(t *testing.T) {
	if mode, refused := ResolveHAMode(" active-active "); mode != ModeActiveActive || refused {
		t.Fatalf("ResolveHAMode(active-active) = %q, %v", mode, refused)
	}
	if mode, refused := ResolveHAMode("active-passive"); mode != ModeActivePassive || refused {
		t.Fatalf("ResolveHAMode(active-passive) = %q, %v", mode, refused)
	}
	if mode, refused := ResolveHAMode(""); mode != "" || refused {
		t.Fatalf("ResolveHAMode(\"\") = %q, %v", mode, refused)
	}
}

// TestActivePassiveGates checks each prerequisite blocks the target on its own.
func TestActivePassiveGates(t *testing.T) {
	if r := EvaluateReadiness(TargetActivePassive, healthyInputs()); !r.Ready || !r.Enabled {
		t.Fatalf("healthy active-passive = %+v", r)
	}
	breakers := map[string]func(*ReadinessInputs){
		"externalDatabase": func(in *ReadinessInputs) { in.Driver = "sqlite" },
		"encryptionKey":    func(in *ReadinessInputs) { in.KeyFromEnv = false },
		"pluginStore":      func(in *ReadinessInputs) { in.PluginStore = "local" },
		"sameVersion":      func(in *ReadinessInputs) { in.Instances[1].Build = "2" },
		"coordinator":      func(in *ReadinessInputs) { in.CoordinatorOK = false },
	}
	for gate, breakIt := range breakers {
		in := healthyInputs()
		breakIt(&in)
		r := EvaluateReadiness(TargetActivePassive, in)
		if r.Ready || gateStatus(r, gate) != GateFail {
			t.Errorf("%s: ready=%v status=%s", gate, r.Ready, gateStatus(r, gate))
		}
	}
	in := healthyInputs()
	in.KeyMatches = false
	if gateStatus(EvaluateReadiness(TargetActivePassive, in), "encryptionKey") != GateFail {
		t.Error("a mismatched key must fail")
	}
}

// TestStartupReportProbesDataDir checks the writability probe and key source.
func TestStartupReportProbesDataDir(t *testing.T) {
	r := BuildStartupReport(context.Background(), nil, StartupInputs{
		NodeID: "n1", Driver: "sqlite", DataDir: t.TempDir(), KeyFromEnv: true, TLSEnabled: true,
	})
	if !r.DataDirOK || r.KeySource != "env" || r.TLSMode != "https" || r.DatabaseOK || r.HAMode != RolePeer {
		t.Fatalf("report = %+v", r)
	}
	r = BuildStartupReport(context.Background(), nil, StartupInputs{DataDir: t.TempDir() + "/missing"})
	if r.DataDirOK || r.KeySource != "file" || r.TLSMode != "off" {
		t.Fatalf("report = %+v", r)
	}
}

// TestDatabaseGateAcceptsEverySharedBackend pins which drivers the HA readiness model
// believes two nodes can share. SQL Server is one; a driver nobody wired is not.
func TestDatabaseGateAcceptsEverySharedBackend(t *testing.T) {
	for _, driver := range []string{"postgres", "mysql", "mssql"} {
		in := healthyInputs()
		in.Driver = driver
		if g := databaseGate(in); g.Status != GatePass {
			t.Errorf("%s: status=%s detail=%q", driver, g.Status, g.Detail)
		}
	}
	for _, driver := range []string{"sqlite", "", "oracle"} {
		in := healthyInputs()
		in.Driver = driver
		if g := databaseGate(in); g.Status != GateFail {
			t.Errorf("%q must fail the gate, got %s", driver, g.Status)
		}
	}
}

// TestAgentRoutingPassesOnceBothHopsAreProven is the gate half of stream forwarding: the
// warning lifts only on a ping that came back through the session relay, and a relay
// that failed keeps it with a remediation naming the endpoint and the routes to pin.
func TestAgentRoutingPassesOnceBothHopsAreProven(t *testing.T) {
	in := activeActiveInputs()
	if g := agentRoutingGate(true, in); g.Status != GateWarn {
		t.Fatalf("an unexercised session relay must warn, got %s: %s", g.Status, g.Detail)
	}

	in.StreamProof = nodelink.Proof{Status: nodelink.ProofProven, NodeID: "a", EnvID: "env-1"}
	g := agentRoutingGate(true, in)
	if g.Status != GatePass {
		t.Fatalf("both hops proven must pass, got %s: %s", g.Status, g.Detail)
	}
	if r := EvaluateReadiness(TargetActiveActive, in); gateStatus(r, "agentRouting") != GatePass || !r.Ready {
		t.Fatalf("a fully proven cluster must show agentRouting as pass, got %+v", r)
	}

	in.StreamProof = nodelink.Proof{Status: nodelink.ProofFailed, NodeID: "a", EnvID: "env-1", Error: "bad handshake"}
	g = agentRoutingGate(true, in)
	if g.Status != GateWarn {
		t.Fatalf("a failed session relay must warn, not pass or fail, got %s", g.Status)
	}
	for _, want := range []string{"bad handshake", "/api/internal/node/agent-stream/", "/api/terminal/ws", "/api/system/host-logs"} {
		if !strings.Contains(g.Detail+" "+g.Remediation, want) {
			t.Errorf("a failed relay's gate must name %q, got detail %q remediation %q", want, g.Detail, g.Remediation)
		}
	}

	// A proven relay cannot rescue a failed HTTP hop.
	in.StreamProof.Status = nodelink.ProofProven
	in.RoutingProof = nodelink.Proof{Status: nodelink.ProofFailed, NodeID: "a", EnvID: "env-1", Error: "refused"}
	if g := agentRoutingGate(true, in); g.Status != GateFail {
		t.Fatalf("a failed HTTP hop must fail whatever the relay says, got %s", g.Status)
	}
}

// TestPlaintextAdvertiseFailsActiveActive: a live node advertising plain http fails the
// node-link transport gate, the operator's opt-in downgrades it to a warning, and https
// passes.
func TestPlaintextAdvertiseFailsActiveActive(t *testing.T) {
	in := activeActiveInputs()
	in.Instances[0].AdvertiseURL = "https://a.example:8705"
	in.Instances[1].AdvertiseURL = "http://b.example:8705"
	r := EvaluateReadiness(TargetActiveActive, in)
	if gateStatus(r, "nodeLinkTransport") != GateFail || r.Ready {
		t.Fatalf("an http advertise URL must fail active-active without the opt-in, got %s ready=%v",
			gateStatus(r, "nodeLinkTransport"), r.Ready)
	}
	if !strings.Contains(strings.Join(gateValues(r, "nodeLinkTransport"), " "), "http://b.example:8705") {
		t.Fatalf("the gate must name the plaintext node, got %v", gateValues(r, "nodeLinkTransport"))
	}
	in.PlaintextAllowed = true
	if got := gateStatus(EvaluateReadiness(TargetActiveActive, in), "nodeLinkTransport"); got != GateWarn {
		t.Fatalf("with the opt-in the gate must warn, got %s", got)
	}
	in.PlaintextAllowed = false
	in.Instances[1].AdvertiseURL = "https://b.example:8705"
	if got := gateStatus(EvaluateReadiness(TargetActiveActive, in), "nodeLinkTransport"); got != GatePass {
		t.Fatalf("https everywhere must pass, got %s", got)
	}
}

// TestPlaintextAdvertiseFailsEveryMultiNodeMode: the default peer mode forwards Docker calls
// and trigger fires between nodes too, so with two live nodes an http advertise URL fails the
// gate whatever target is asked for; one live node makes no node calls and passes.
func TestPlaintextAdvertiseFailsEveryMultiNodeMode(t *testing.T) {
	in := healthyInputs()
	in.Instances[0].AdvertiseURL = "https://a.example:8705"
	in.Instances[1].AdvertiseURL = "http://b.example:8705"
	for _, target := range []string{TargetActivePassive, TargetActiveActive, ""} {
		if got := gateStatus(EvaluateReadiness(target, in), "nodeLinkTransport"); got != GateFail {
			t.Fatalf("target %q: two live nodes with an http advertise URL must fail, got %s", target, got)
		}
	}
	g := nodeLinkTransportGate(in)
	if strings.Contains(g.Detail, "no node-to-node calls") {
		t.Fatalf("a multi-node failure must not claim no node calls are made: %s", g.Detail)
	}
	in.Instances[1].Alive = false
	if got := gateStatus(EvaluateReadiness(TargetActivePassive, in), "nodeLinkTransport"); got != GatePass {
		t.Fatalf("one live node makes no node calls and must pass, got %s", got)
	}
}

func gateValues(r Readiness, id string) []string {
	for _, g := range r.Gates {
		if g.ID == id {
			return g.Values
		}
	}
	return nil
}

// TestNodeLinkGateWarnsOnWeakenedTLS: skip-verify and the http opt-in each make streamed
// bodies and relayed sessions forgeable, so each warns and says so; an unusable CA file
// fails, and a pinned CA with https everywhere passes.
func TestNodeLinkGateWarnsOnWeakenedTLS(t *testing.T) {
	in := healthyInputs()
	in.Instances[0].AdvertiseURL = "https://a.example:8705"
	in.Instances[1].AdvertiseURL = "https://b.example:8705"
	if got := gateStatus(EvaluateReadiness(TargetActiveActive, in), "nodeLinkTransport"); got != GatePass {
		t.Fatalf("https everywhere with verification on must pass, got %s", got)
	}

	in.SkipVerify = true
	g := nodeLinkTransportGate(in)
	if g.Status != GateWarn {
		t.Fatalf("skip-verify must warn, got %s", g.Status)
	}
	if !strings.Contains(strings.Join(g.Values, " "), nodelink.EnvSkipVerify) || !strings.Contains(g.Remediation, nodelink.EnvCAFile) {
		t.Fatalf("the warning must name skip-verify and recommend the CA file: values %v remediation %q", g.Values, g.Remediation)
	}
	for _, want := range []string{"forge", "streamed request bodies", "relayed terminal"} {
		if !strings.Contains(g.Detail, want) {
			t.Fatalf("the skip-verify warning must say sessions and bodies become forgeable (%q): %s", want, g.Detail)
		}
	}

	in.SkipVerify = false
	in.Instances[1].AdvertiseURL = "http://b.example:8705"
	in.PlaintextAllowed = true
	if g := nodeLinkTransportGate(in); g.Status != GateWarn || !strings.Contains(g.Detail, "forge") {
		t.Fatalf("the http opt-in must warn that bodies and sessions become forgeable, got %s: %s", g.Status, g.Detail)
	}

	in.PlaintextAllowed = false
	in.Instances[1].AdvertiseURL = "https://b.example:8705"
	in.CAFileError = "reading MCHARBOR_NODE_LINK_CA_FILE: no such file"
	if g := nodeLinkTransportGate(in); g.Status != GateFail || !strings.Contains(strings.Join(g.Values, " "), nodelink.EnvCAFile) {
		t.Fatalf("an unusable CA file must fail and be named, got %s %v", g.Status, g.Values)
	}
}
