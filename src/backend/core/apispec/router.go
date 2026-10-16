// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Router mirrors the slice of chi.Router that module route files use, and
// records every route it registers into a Registry on the way through.
//
// It is a drop-in: a module swaps chi.Router for *apispec.Router in its mount
// closures and changes nothing else, because the permission a route requires is
// recovered from the middleware chain rather than declared a second time.
type Router struct {
	mux    chi.Router
	reg    *Registry
	prefix string
	group  Group
	perms  []rbac.PermRequirement
	opts   []Option
}

// Option adjusts the operation a route records, for the parts of a contract
// that cannot be inferred from the handler.
type Option func(*Operation)

// NewRouter wraps mux so routes registered through it are recorded in reg.
func NewRouter(mux chi.Router, reg *Registry, prefix string, group Group) *Router {
	return &Router{mux: mux, reg: reg, prefix: prefix, group: group}
}

// Mux returns the underlying chi router, for the rare caller that needs a
// chi.Router itself rather than the recording wrapper.
func (rt *Router) Mux() chi.Router { return rt.mux }

// Registry returns the registry this router records into.
func (rt *Router) Registry() *Registry { return rt.reg }

func (rt *Router) derive(mux chi.Router, prefix string, perms []rbac.PermRequirement, opts []Option) *Router {
	next := &Router{mux: mux, reg: rt.reg, prefix: prefix, group: rt.group}
	next.perms = append(append([]rbac.PermRequirement(nil), rt.perms...), perms...)
	next.opts = append(append([]Option(nil), rt.opts...), opts...)
	return next
}

// Route mounts a subtree under pattern, carrying the prefix into the recorded
// paths so a nested handler reports the full route rather than its last segment.
func (rt *Router) Route(pattern string, fn func(*Router)) {
	rt.mux.Route(pattern, func(sub chi.Router) {
		fn(rt.derive(sub, joinPath(rt.prefix, pattern), nil, nil))
	})
}

// Group mounts an inline subtree at the same path, for middleware that should
// apply to several routes without changing their prefix.
func (rt *Router) Group(fn func(*Router)) {
	rt.mux.Group(func(sub chi.Router) {
		fn(rt.derive(sub, rt.prefix, nil, nil))
	})
}

// With returns a router whose routes carry middlewares, and probes them so the
// permissions they enforce are recorded without being named twice.
func (rt *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	return rt.derive(rt.mux.With(middlewares...), rt.prefix, rbac.ProbeChain(middlewares...), nil)
}

// Meta returns a router whose routes carry extra specification detail, for the
// summary, request and response shapes a handler cannot report about itself.
func (rt *Router) Meta(opts ...Option) *Router {
	return rt.derive(rt.mux, rt.prefix, nil, opts)
}

// Use adds middleware to every route registered on this router afterwards.
// Permissions from Use are probed exactly as With probes them.
func (rt *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	rt.perms = append(rt.perms, rbac.ProbeChain(middlewares...)...)
	rt.mux.Use(middlewares...)
}

// Mount attaches a whole handler subtree. The subtree's internal routes are not
// recorded, because chi cannot enumerate them from here; MountedAt records the
// fact that the prefix exists.
func (rt *Router) Mount(pattern string, h http.Handler) {
	rt.mux.Mount(pattern, h)
}

// Get registers a GET route.
func (rt *Router) Get(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodGet, pattern, h, opts)
	rt.mux.Get(pattern, h)
}

// Post registers a POST route.
func (rt *Router) Post(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodPost, pattern, h, opts)
	rt.mux.Post(pattern, h)
}

// Put registers a PUT route.
func (rt *Router) Put(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodPut, pattern, h, opts)
	rt.mux.Put(pattern, h)
}

// Patch registers a PATCH route.
func (rt *Router) Patch(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodPatch, pattern, h, opts)
	rt.mux.Patch(pattern, h)
}

// Delete registers a DELETE route.
func (rt *Router) Delete(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodDelete, pattern, h, opts)
	rt.mux.Delete(pattern, h)
}

// Head registers a HEAD route.
func (rt *Router) Head(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodHead, pattern, h, opts)
	rt.mux.Head(pattern, h)
}

// Options registers an OPTIONS route.
func (rt *Router) Options(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(http.MethodOptions, pattern, h, opts)
	rt.mux.Options(pattern, h)
}

// Method registers a route for an arbitrary verb.
func (rt *Router) Method(method, pattern string, h http.Handler, opts ...Option) {
	rt.record(method, pattern, h, opts)
	rt.mux.Method(method, pattern, h)
}

// HandleFunc registers a handler for every method on pattern, which the spec
// records as the ANY pseudo-verb because OpenAPI has no wildcard method.
func (rt *Router) HandleFunc(pattern string, h http.HandlerFunc, opts ...Option) {
	rt.record(MethodAny, pattern, h, opts)
	rt.mux.HandleFunc(pattern, h)
}

// Handle registers a handler for every method on pattern.
func (rt *Router) Handle(pattern string, h http.Handler, opts ...Option) {
	rt.record(MethodAny, pattern, h, opts)
	rt.mux.Handle(pattern, h)
}

// MethodAny is the pseudo-verb recorded for a route registered without one.
const MethodAny = "ANY"

// foldEnvPins copies perms with every rbac.PinEnvToPath marker removed and its
// path parameter stamped on the environment- and stack-scoped gates after it (both
// read `?env=`, which the pin rewrites), so a pin is never counted as a gate but the
// spec still says which parameter scopes the route.
func foldEnvPins(perms []rbac.PermRequirement) []rbac.PermRequirement {
	out := make([]rbac.PermRequirement, 0, len(perms))
	pinned := ""
	for _, req := range perms {
		if param, ok := req.EnvPin(); ok {
			pinned = param
			continue
		}
		if (req.Scope == rbac.ScopeEnvironment || req.Scope == rbac.ScopeStack) && pinned != "" {
			req.EnvParam = pinned
		}
		out = append(out, req)
	}
	return out
}

func (rt *Router) record(method, pattern string, h any, opts []Option) {
	name := handlerName(h)
	op := Operation{
		Method:  method,
		Path:    joinPath(rt.prefix, pattern),
		Group:   rt.group,
		Handler: name,
		Module:  moduleOf(name),
		Perms:   foldEnvPins(rt.perms),
	}
	for _, opt := range rt.opts {
		if opt != nil {
			opt(&op)
		}
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&op)
		}
	}
	if op.Summary == "" {
		op.Summary = summaryFromHandler(op.Handler)
	}
	rt.reg.Add(op)
}
