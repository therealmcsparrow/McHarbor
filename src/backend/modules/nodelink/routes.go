// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package nodelink mounts the routes a McHarbor node offers another McHarbor node: run
// this agent call, relay these agent sessions, share this agent's live stats, and take
// this workflow-trigger fire, because you hold the agent or the lock and I do not.
package nodelink

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	corenodelink "github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// EnvRateLimit overrides the per-node request budget of each agent hop.
const EnvRateLimit = "MCHARBOR_NODE_LINK_RATE_LIMIT"

// defaultAgentHopRate is the per-minute budget, per verified live node, of each agent hop. A
// peer node forwards every Docker call its users make, so it is generous; it caps a runaway
// peer, not real traffic. It is no bound on a stolen secret, whose holder can sign as every
// live node in turn and spend each one's budget.
const defaultAgentHopRate = 3000

// triggerFireEvents is the plugin trigger events per minute one non-leader node may forward.
const triggerFireEvents = 600

// triggerFireRate is the per-minute budget, per verified node, of the workflow-trigger fire
// route. An offer to a peer not yet confirmed as leader costs two requests (the signed
// leader probe, then the fire), so the budget is twice the events it has to carry.
const triggerFireRate = 2 * triggerFireEvents

// unauthenticatedRate is the per-minute count, per source address (an IPv6 source counted by
// its /64), of requests to any node-to-node route that fail verification or name a node that
// is not live, past which they are answered 429 instead of 401 or 403. It protects nothing:
// such a request is refused either way and its HMAC is already computed. It only tells a
// misconfigured peer it is being ignored, and never costs a verified peer on that address.
const unauthenticatedRate = 120

// agentHopRate reads EnvRateLimit, falling back to the default on anything but a
// positive integer.
func agentHopRate() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvRateLimit))); err == nil && n > 0 {
		return n
	}
	return defaultAgentHopRate
}

// admission is the node link's budget: one bucket per source address for requests that fail
// verification, and one per verified node and hop for the rest.
type admission struct {
	clientKey func(*http.Request) string
	unauth    *middleware.Limiter
	perNode   map[string]*middleware.Limiter
}

func newAdmission(clientKey func(*http.Request) string) *admission {
	rate := agentHopRate()
	return &admission{
		clientKey: clientKey,
		unauth:    middleware.NewLimiter(unauthenticatedRate),
		perNode: map[string]*middleware.Limiter{
			corenodelink.HopAgentRPC:        middleware.NewLimiter(rate),
			corenodelink.HopAgentStream:     middleware.NewLimiter(rate),
			corenodelink.HopStats:           middleware.NewLimiter(rate),
			corenodelink.HopWorkflowTrigger: middleware.NewLimiter(triggerFireRate),
		},
	}
}

// Unauthenticated charges a failed verification to the request's source address.
func (a *admission) Unauthenticated(r *http.Request, _ string) bool {
	return a.unauth.Allow(sourceKey(r, a.clientKey))
}

// Authenticated charges a verified request to the node that signed it, per hop.
func (a *admission) Authenticated(_ *http.Request, hop, nodeID string) bool {
	limiter, ok := a.perNode[hop]
	if !ok {
		return true
	}
	return limiter.Allow(nodeID)
}

// sourceKey is the address a request is attributed to: the resolved client address behind a
// trusted proxy, else the socket peer. An IPv6 address is keyed on its /64, because one host
// commonly holds a whole /64 and would otherwise get a fresh budget per address.
func sourceKey(r *http.Request, clientKey func(*http.Request) string) string {
	host := ""
	if clientKey != nil {
		host = clientKey(r)
	}
	if host == "" {
		host = r.RemoteAddr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// Mount registers the internal agent-RPC, agent-stream, stats and workflow-trigger fire endpoints.
//
// It is mounted public — no session, no API key — because its caller is a node, not a
// user, and it authenticates as one: every request carries an HMAC-SHA256 signature over
// the sending and the addressed node, method, path, query, environment, actor id and name,
// timeout, hop, timestamp, nonce, body digest and every forwarded inner header, keyed by a
// value derived from MCHARBOR_SECRET, which every node of a cluster already shares. A request
// that did not arrive over TLS is refused first unless MCHARBOR_NODE_LINK_ALLOW_HTTP=1. The
// HMAC is verified in constant time from the headers and the claimed digest before any body
// byte is read; the signing node must then be a live instance in the registry. A digest-signed
// body of unknown or oversized length is refused, the body read is checked against the digest,
// and only then is the nonce spent. A request that fails verification is counted against its
// source address; a verified one spends its node's budget for that route. A node that does
// not hold the environment's agent refuses with 409 rather than forwarding onwards. It is not
// in the protected group deliberately: a session or API key would be the wrong credential
// and would invite an operator to call it by hand.
func Mount(app *router.AppDeps) {
	app.NodeLink.SetAdmission(newAdmission(app.ClientKey))
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Post("/internal/node/agent-rpc/{envId}", func(w http.ResponseWriter, r *http.Request) {
			app.NodeLink.ServeAgentRPC(w, r, chi.URLParam(r, "envId"))
		}, apispec.Exempt("node-to-node: authenticated by an HMAC node signature over the shared secret, not by a user session"),
			apispec.Binary())
		// The session relay: exec, host terminal, guest console, host logs and every
		// agent request/reply message for an agent this node holds, on behalf of the
		// node that was asked. Signed like agent-rpc, verified before the upgrade.
		r.Get("/internal/node/agent-stream/{envId}", func(w http.ResponseWriter, r *http.Request) {
			app.NodeLink.ServeAgentStream(w, r, chi.URLParam(r, "envId"))
		}, apispec.Exempt("node-to-node: authenticated by an HMAC node signature over the shared secret, not by a user session"),
			apispec.WebSocket())
		// The owning node's live stats snapshot, which its agent pushes to it alone.
		r.Get("/internal/node/stats/{envId}", func(w http.ResponseWriter, r *http.Request) {
			app.NodeLink.ServeStats(w, r, chi.URLParam(r, "envId"), app.StatsHub)
		}, apispec.Exempt("node-to-node: authenticated by an HMAC node signature over the shared secret, not by a user session"))
		// A plugin trigger fire, or the leader probe that precedes it, from a node that
		// does not lead the workflow trigger; only the lock holder accepts, every other
		// node answers 409.
		r.Post("/internal/node/workflow-trigger/fire", func(w http.ResponseWriter, r *http.Request) {
			app.NodeLink.ServeTriggerFire(w, r)
		}, apispec.Exempt("node-to-node: authenticated by an HMAC node signature over the shared secret, not by a user session"))
	})
}
