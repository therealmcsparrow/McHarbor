// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	corenodelink "github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// noAgents holds no agent, so a verified agent-rpc call is answered 409 by the handler.
type noAgents struct{}

func (noAgents) Transport(string) *agent.AgentTransport { return nil }
func (noAgents) List() []agent.AgentConnection          { return nil }

// mounted builds the public band with only these routes on it, over a link for node-a that
// holds the shared secret and no agent.
func mounted(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := corenodelink.New(corenodelink.NewOwners(nil, "node-a", "https://node-a:8705"),
		corenodelink.NewSigner("shared-secret"), noAgents{}, logger)
	link.SetPeers(func(context.Context) ([]corenodelink.Peer, error) {
		return []corenodelink.Peer{{NodeID: "node-b", AdvertiseURL: "https://node-b:8705"}, {NodeID: "node-c", AdvertiseURL: "https://node-c:8705"}}, nil
	})
	app := &router.AppDeps{Logger: logger, NodeLink: link}
	Mount(app)
	r := chi.NewRouter()
	app.MountPublicRoutes(r)
	return r
}

// junk is an unsigned request from remote, the kind anything on the network can send.
func junk(h http.Handler, remote string) int {
	req := httptest.NewRequest(http.MethodPost, "/internal/node/agent-rpc/env-1", nil)
	req.RemoteAddr = remote
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

// signedCall is an agent-rpc call node signed with the shared secret, from remote.
func signedCall(h http.Handler, node, remote string) int {
	req := httptest.NewRequest(http.MethodPost, "/internal/node/agent-rpc/env-1", nil)
	req.RemoteAddr = remote
	req.TLS = &tls.ConnectionState{}
	req.ContentLength = 0
	empty := sha256.Sum256(nil)
	if err := corenodelink.NewSigner("shared-secret").Sign(req, corenodelink.Claims{Hop: corenodelink.HopAgentRPC, NodeID: node,
		Audience: "node-a", EnvID: "env-1", Method: http.MethodGet, Path: "/v1.43/containers/json"},
		xid.New().String(), hex.EncodeToString(empty[:])); err != nil {
		return -1
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

// TestJunkDoesNotSpendAPeersBudget: unsigned requests from an address a real peer shares
// (a NAT or proxy egress) run out their own unauthenticated budget, and the signed peer
// behind the same address is still served.
func TestJunkDoesNotSpendAPeersBudget(t *testing.T) {
	t.Setenv(EnvRateLimit, "3")
	h := mounted(t)
	for i := 0; i < unauthenticatedRate; i++ {
		if got := junk(h, "192.0.2.1:4000"); got != http.StatusUnauthorized {
			t.Fatalf("junk request %d inside the unauthenticated budget must be refused 401, got %d", i+1, got)
		}
	}
	if got := junk(h, "192.0.2.1:4000"); got != http.StatusTooManyRequests {
		t.Fatalf("junk past the unauthenticated budget must be refused 429, got %d", got)
	}
	if got := signedCall(h, "node-b", "192.0.2.1:4001"); got != http.StatusConflict {
		t.Fatalf("a signed peer behind the same address must still reach the handler (409 here), got %d", got)
	}
}

// TestVerifiedNodesHaveTheirOwnBudget: an authenticated budget is per node, so one node
// running out does not stop another, even from the same address.
func TestVerifiedNodesHaveTheirOwnBudget(t *testing.T) {
	t.Setenv(EnvRateLimit, "3")
	h := mounted(t)
	for i := 0; i < 3; i++ {
		if got := signedCall(h, "node-b", "192.0.2.5:4000"); got != http.StatusConflict {
			t.Fatalf("signed call %d inside the node budget must reach the handler, got %d", i+1, got)
		}
	}
	if got := signedCall(h, "node-b", "192.0.2.5:4000"); got != http.StatusTooManyRequests {
		t.Fatalf("a node past its budget must be refused 429, got %d", got)
	}
	if got := signedCall(h, "node-c", "192.0.2.5:4000"); got != http.StatusConflict {
		t.Fatalf("another node keeps its own budget, got %d", got)
	}
}

// TestIPv6SourcesShareTheirSlash64: two addresses of one /64 are one source, so a host
// cannot rotate through its /64 for a fresh unauthenticated budget.
func TestIPv6SourcesShareTheirSlash64(t *testing.T) {
	a := httptest.NewRequest(http.MethodPost, "/", nil)
	a.RemoteAddr = "[2001:db8:1:2::1]:4000"
	b := httptest.NewRequest(http.MethodPost, "/", nil)
	b.RemoteAddr = "[2001:db8:1:2:ffff::9]:4000"
	c := httptest.NewRequest(http.MethodPost, "/", nil)
	c.RemoteAddr = "[2001:db8:1:3::1]:4000"
	if sourceKey(a, nil) != sourceKey(b, nil) {
		t.Fatalf("one /64 must be one source: %s vs %s", sourceKey(a, nil), sourceKey(b, nil))
	}
	if sourceKey(a, nil) == sourceKey(c, nil) {
		t.Fatalf("another /64 is another source, both keyed %s", sourceKey(a, nil))
	}
	v4 := httptest.NewRequest(http.MethodPost, "/", nil)
	v4.RemoteAddr = "192.0.2.7:4000"
	if got := sourceKey(v4, nil); got != "192.0.2.7" {
		t.Fatalf("an IPv4 source is keyed on its address, got %s", got)
	}
}

// TestFireBudgetCoversProbes: an offer to an unconfirmed leader is a probe and a fire, so
// the fire route's per-node budget carries twice the events it is sized for.
func TestFireBudgetCoversProbes(t *testing.T) {
	if triggerFireRate < 2*triggerFireEvents {
		t.Fatalf("the fire budget (%d) must cover a probe plus a fire for each of %d events", triggerFireRate, triggerFireEvents)
	}
	a := newAdmission(nil)
	for i := 0; i < 2*triggerFireEvents; i++ {
		if !a.Authenticated(nil, corenodelink.HopWorkflowTrigger, "node-b") {
			t.Fatalf("request %d of %d events' probes and fires was refused", i+1, triggerFireEvents)
		}
	}
}
