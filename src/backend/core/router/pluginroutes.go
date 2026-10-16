// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

// PluginRouter is the live routing table for plugin-contributed HTTP
// routes.
//
// It exists because plugins arrive and leave while the server is
// running. Host modules register their routes once, before router.New
// builds the tree, and never change; a plugin is installed, enabled,
// updated, disabled and uninstalled from the App Store at any moment.
// The old path could not express that: RegisterProtectedRoutes appended
// a mount function to a slice that MountProtectedRoutes drained exactly
// once inside router.New, so a plugin loaded after that point recorded
// its routes in `plugin_routes`, registered its adapters, showed its
// sidebar entry — and served 404 on every call until the process was
// restarted. Nothing in the product said so.
//
// # Why a rebuild-and-swap rather than a mutable mux
//
// chi.Mux is not safe for route registration concurrent with serving:
// its trie has no lock, and adding a route while a request is being
// matched is a data race in the literal sense. So no live router is
// ever mutated here. The source of truth is the per-origin route map
// below; every change rebuilds a complete, fresh chi.Mux and installs
// it with a single atomic store. A request in flight keeps serving from
// the router it started with, and the next request sees the new one.
// Rebuilding a few dozen routes costs microseconds and happens only on
// install / enable / disable / uninstall.
//
// It is also what makes *removal* expressible. An append-only slice of
// mount functions has no way to say "and this plugin's routes are gone";
// a rebuild simply leaves them out.
//
// # Auth classes
//
// router.New mounts this once, under `/api`, outside the three
// middleware groups — chi would panic on three catch-alls in one mux,
// and a catch-all inside the protected group would answer 401 for every
// mistyped API path instead of 404. Instead the groups hand their
// middleware chains here at construction, and each route is wrapped
// with the chain for the class it declared. The result is identical to
// having been registered in the group, in the same order.
type PluginRouter struct {
	mu sync.Mutex
	// byOrigin is the source of truth, keyed by plugin slug.
	byOrigin map[string][]PluginRoute
	// order keeps rebuilds deterministic. Map iteration order would
	// otherwise decide which of two plugins claiming the same pattern
	// wins, and "which plugin answers" must not change per restart.
	order  []string
	chains pluginChains
	logger *slog.Logger

	// live is the router currently serving. Read on every request,
	// written only by rebuild.
	live atomic.Pointer[chi.Mux]
}

// PluginRoute is one route a plugin contributed.
type PluginRoute struct {
	// Method is the HTTP method, e.g. http.MethodGet.
	Method string
	// Pattern is the chi pattern *relative to /api* — "/kubernetes/summary",
	// not "/api/kubernetes/summary". The loader trims the prefix, because
	// a plugin manifest declares the absolute form.
	Pattern string
	// AuthClass is "public", "auth" or "protected". Anything else is
	// treated as "protected": an unrecognised class must fail closed,
	// never open.
	AuthClass string
	// Handler is the plugin's handler, already wrapped by the loader in
	// its per-route permission and audit middleware. This type adds only
	// the session/rate-limit chain the auth class implies.
	Handler http.Handler
}

// Auth class names. Exported so the plugin runtime names the same
// strings this package switches on.
const (
	PluginAuthPublic    = "public"
	PluginAuthAuth      = "auth"
	PluginAuthProtected = "protected"
)

type pluginChains struct {
	public    func(http.Handler) http.Handler
	auth      func(http.Handler) http.Handler
	protected func(http.Handler) http.Handler
}

// NewPluginRouter returns an empty router. It serves 404 until routes
// are set, which is the correct answer for an installation with no
// plugins.
func NewPluginRouter(logger *slog.Logger) *PluginRouter {
	return &PluginRouter{
		byOrigin: make(map[string][]PluginRoute),
		logger:   logger,
	}
}

// SetChains installs the per-auth-class middleware. Called once by
// router.New with the same chains its three groups use.
//
// A nil chain is a pass-through rather than an error: the public class
// genuinely has no middleware of its own beyond what the /api subrouter
// already applies.
func (p *PluginRouter) SetChains(public, auth, protected func(http.Handler) http.Handler) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chains = pluginChains{public: public, auth: auth, protected: protected}
	p.rebuildLocked()
}

// Set replaces one plugin's entire route set and swaps in a router that
// reflects it. Passing nil removes the plugin's routes.
//
// Whole-set replacement rather than incremental add: a plugin being
// updated re-registers everything it has, and merging would leave a
// route the new version dropped still answering from the old one.
func (p *PluginRouter) Set(origin string, routes []PluginRoute) {
	if p == nil || origin == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(routes) == 0 {
		delete(p.byOrigin, origin)
		p.order = slices.DeleteFunc(p.order, func(s string) bool { return s == origin })
	} else {
		if _, existing := p.byOrigin[origin]; !existing {
			p.order = append(p.order, origin)
		}
		p.byOrigin[origin] = slices.Clone(routes)
	}
	p.rebuildLocked()
}

// Remove drops every route one plugin contributed.
func (p *PluginRouter) Remove(origin string) { p.Set(origin, nil) }

// Routes reports how many routes each origin currently serves. For the
// loader's diagnostics; never on a request path.
func (p *PluginRouter) Routes() map[string]int {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]int, len(p.byOrigin))
	for origin, routes := range p.byOrigin {
		out[origin] = len(routes)
	}
	return out
}

// ServeHTTP delegates to whichever router is current.
func (p *PluginRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p == nil {
		http.NotFound(w, r)
		return
	}
	mux := p.live.Load()
	if mux == nil {
		// No plugin has contributed a route yet. 404 is the same answer
		// the host gave before this type existed.
		http.NotFound(w, r)
		return
	}
	mux.ServeHTTP(w, r)
}

// rebuildLocked constructs a fresh chi.Mux from the current table and
// installs it. Caller holds p.mu.
//
// Two hostile inputs are handled rather than propagated, because a
// plugin must not be able to take the host's HTTP server down:
//
//   - Two plugins claiming the same method+pattern. chi panics on a
//     duplicate registration. First registration wins (deterministic via
//     p.order) and the loser is logged and skipped.
//   - A malformed pattern. chi panics on those too. The recover below
//     abandons the rebuild and keeps the previous router serving, which
//     is strictly better than a half-built one or a dead process.
func (p *PluginRouter) rebuildLocked() {
	mux := chi.NewRouter()
	seen := make(map[string]string) // "METHOD pattern" -> origin
	installed := 0

	defer func() {
		if rec := recover(); rec != nil {
			// The previous router stays live: p.live is only stored to on
			// the success path below.
			if p.logger != nil {
				p.logger.Error("plugin router: rebuild failed, keeping previous routes",
					"panic", rec)
			}
		}
	}()

	for _, origin := range p.order {
		for _, route := range p.byOrigin[origin] {
			if route.Method == "" || route.Pattern == "" || route.Handler == nil {
				continue
			}
			key := route.Method + " " + route.Pattern
			if owner, taken := seen[key]; taken {
				if p.logger != nil {
					p.logger.Warn("plugin router: duplicate route ignored",
						"method", route.Method, "pattern", route.Pattern,
						"registered_by", owner, "ignored_from", origin)
				}
				continue
			}
			seen[key] = origin
			mux.Method(route.Method, route.Pattern, p.wrap(route))
			installed++
		}
	}

	p.live.Store(mux)
	if p.logger != nil {
		p.logger.Debug("plugin router: rebuilt", "routes", installed, "plugins", len(p.order))
	}
}

// wrap applies the middleware chain the route's auth class implies.
func (p *PluginRouter) wrap(route PluginRoute) http.Handler {
	switch route.AuthClass {
	case PluginAuthPublic:
		return applyChain(p.chains.public, route.Handler)
	case PluginAuthAuth:
		return applyChain(p.chains.auth, route.Handler)
	default:
		// "protected", and anything unrecognised. Failing closed matters:
		// a typo in an auth class must not publish an endpoint.
		return applyChain(p.chains.protected, route.Handler)
	}
}

func applyChain(chain func(http.Handler) http.Handler, h http.Handler) http.Handler {
	if chain == nil {
		return h
	}
	return chain(h)
}

// chain composes middleware left-to-right, so chain(a, b, c) runs a
// first. Mirrors how chi applies a group's Use() calls, which is what
// keeps a plugin route's behaviour identical to the group it used to be
// registered in.
func chain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			if middlewares[i] == nil {
				continue
			}
			next = middlewares[i](next)
		}
		return next
	}
}
