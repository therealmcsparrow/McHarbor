// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import "net/http"

// PermScope names how a Require* middleware resolves the permission it guards,
// which the API spec reports so a consumer knows what `?env=` does to a route.
type PermScope string

// The scopes a Require* middleware can declare to a Probe.
const (
	// ScopeEnvironment resolves the permission against `?env=` on the request.
	ScopeEnvironment PermScope = "environment"
	// ScopeGlobal ignores request scope and checks the permission globally.
	ScopeGlobal PermScope = "global"
	// ScopeStack resolves the permission against a stack named by the route.
	ScopeStack PermScope = "stack"
	// ScopeOwner passes for the owning user, or for holders of the permission.
	ScopeOwner PermScope = "owner"
	// ScopeAccount passes for the account named in the route, self-service.
	ScopeAccount PermScope = "account"
)

// PermRequirement is one permission gate recovered from a middleware chain.
type PermRequirement struct {
	// Keys are the permissions the gate accepts; more than one means any of them.
	Keys []Permission
	// Scope is how the gate resolves those keys against the request.
	Scope PermScope
	// Any reports that holding any one of Keys is sufficient.
	Any bool
	// EnvParam names the path parameter an environment-scoped gate is pinned to by
	// PinEnvToPath; empty means the scope comes from whatever `?env=` the caller sent.
	EnvParam string
}

// scopeEnvPin marks a PinEnvToPath entry in a probed chain. It is folded into the
// environment-scoped gates after it and never reported as a gate of its own.
const scopeEnvPin PermScope = "env-pin"

// EnvPin reports whether r is a PinEnvToPath marker rather than a gate, and the
// path parameter it pins, so the spec recorder can fold it into later gates.
func (r PermRequirement) EnvPin() (string, bool) {
	return r.EnvParam, r.Scope == scopeEnvPin
}

// Probe is a sentinel http.Handler that Require* middlewares recognise and
// report themselves to instead of guarding, so the API spec generator can read
// a route's permissions off the chain that is already registered.
//
// It is never reachable from a request: nothing constructs one during serving,
// and a middleware that sees one returns it unwrapped rather than calling it.
type Probe struct {
	// Requirements holds every gate the probed chain declared, in chain order.
	Requirements []PermRequirement
}

// ServeHTTP makes Probe an http.Handler so it can be passed to a middleware.
// A probe that is ever actually served is a bug, and answering 500 says so
// rather than letting an ungated request through.
func (p *Probe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "rbac probe is not a request handler", http.StatusInternalServerError)
}

func (p *Probe) record(scope PermScope, anyOf bool, keys ...Permission) {
	p.Requirements = append(p.Requirements, PermRequirement{Keys: keys, Scope: scope, Any: anyOf})
}

func (p *Probe) pinEnv(param string) {
	p.Requirements = append(p.Requirements, PermRequirement{Scope: scopeEnvPin, EnvParam: param})
}

// ProbeChain runs middlewares against a Probe and returns what they declared,
// so a caller can recover the permissions guarding a route without executing it.
func ProbeChain(middlewares ...func(http.Handler) http.Handler) []PermRequirement {
	probe := &Probe{}
	for _, mw := range middlewares {
		if mw == nil {
			continue
		}
		mw(probe)
	}
	return probe.Requirements
}

// probed reports a requirement to next when it is a Probe, and tells the caller
// to return next unwrapped. Every Require* middleware starts with it.
func probed(next http.Handler, scope PermScope, anyOf bool, keys ...Permission) (http.Handler, bool) {
	p, ok := next.(*Probe)
	if !ok {
		return nil, false
	}
	p.record(scope, anyOf, keys...)
	return p, true
}
