// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package apispec records every HTTP route as it is registered so the OpenAPI
// document is generated from the router that actually runs, never hand-written
// beside it.
package apispec

import (
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Group names the middleware band a route was mounted in, which decides whether
// a consumer needs a session, an API key, or nothing at all.
type Group string

// The three bands router.New mounts, in the order it mounts them.
const (
	// GroupPublic is reachable with no credentials.
	GroupPublic Group = "public"
	// GroupAuth is the rate-limited login surface, also uncredentialed.
	GroupAuth Group = "auth"
	// GroupProtected requires a session cookie or an API key.
	GroupProtected Group = "protected"
)

// Param is one query parameter an operation reads.
type Param struct {
	// Name is the query key as the handler parses it.
	Name string
	// Description says what it selects; it is shown in the reference.
	Description string
	// Required marks a parameter the handler rejects the request without.
	Required bool
	// Example seeds the generated documentation and the CLI's help text.
	Example string
}

// Operation is one method-and-path pair, with everything the spec needs about it.
type Operation struct {
	// Method is the uppercase HTTP verb.
	Method string
	// Path is the full chi pattern including the /api prefix.
	Path string
	// Group is the middleware band the route was mounted in.
	Group Group
	// Handler is the fully qualified name of the registered handler func.
	Handler string
	// Module is the feature directory the handler lives in, used as the tag.
	Module string
	// Perms holds every permission gate recovered from the route's middleware.
	Perms []rbac.PermRequirement
	// Exempt states why a mutating route carries no permission gate.
	Exempt string
	// Summary is a one-line description; it defaults to the handler name.
	Summary string
	// Description is the longer prose shown in the rendered reference.
	Description string
	// Tags override the module-derived tag when a route belongs elsewhere.
	Tags []string
	// Request is a zero value of the decoded request body, if the route takes one.
	Request any
	// Response is a zero value of the payload inside the success envelope.
	Response any
	// Query lists the query parameters the handler reads.
	Query []Param
	// Stream marks a route that holds the connection open rather than replying once.
	Stream StreamKind
	// Deprecated marks a route that still works and should not be used.
	Deprecated bool
}

// StreamKind marks how a long-lived route delivers data, since OpenAPI has no
// native way to say "this never returns a single body".
type StreamKind string

// The stream shapes this API serves.
const (
	// StreamNone is an ordinary request and response.
	StreamNone StreamKind = ""
	// StreamSSE is a text/event-stream the client consumes until it closes.
	StreamSSE StreamKind = "sse"
	// StreamWebSocket is an HTTP upgrade; OpenAPI describes only the handshake.
	StreamWebSocket StreamKind = "websocket"
	// StreamBinary is an open-ended octet stream, such as a download or an export.
	StreamBinary StreamKind = "binary"
	// StreamText is a plain-text body outside the JSON envelope, such as the
	// Prometheus exposition a scraper reads.
	StreamText StreamKind = "text"
)

// Mutating reports whether the operation changes state, which is what decides
// that a missing permission gate is a finding rather than a read.
func (o Operation) Mutating() bool {
	switch o.Method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// Gated reports whether any permission middleware guards the operation.
func (o Operation) Gated() bool { return len(o.Perms) > 0 }

// PermissionKeys returns every permission key the operation's gates name, so a
// contract test can resolve them against the catalog without walking the gates.
func (o Operation) PermissionKeys() []string {
	var keys []string
	for _, req := range o.Perms {
		for _, k := range req.Keys {
			keys = append(keys, string(k))
		}
	}
	return keys
}

// Registry collects operations as modules mount, and is read once at the end.
type Registry struct {
	mu  sync.Mutex
	ops []Operation
}

// NewRegistry returns an empty Registry ready to be handed to a Router.
func NewRegistry() *Registry { return &Registry{} }

// Add records one operation. Routers call it; nothing else should.
func (reg *Registry) Add(op Operation) {
	if reg == nil {
		return
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.ops = append(reg.ops, op)
}

// Operations returns every recorded operation sorted by path then method, so
// the generated document is byte-stable across runs and CI can diff it.
func (reg *Registry) Operations() []Operation {
	if reg == nil {
		return nil
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()

	out := make([]Operation, len(reg.ops))
	copy(out, reg.ops)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// Len reports how many operations have been recorded.
func (reg *Registry) Len() int {
	if reg == nil {
		return 0
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return len(reg.ops)
}

// joinPath concatenates a router prefix and a chi pattern into the full route,
// collapsing the duplicate slashes that "/" patterns would otherwise produce.
func joinPath(prefix, pattern string) string {
	switch {
	case pattern == "" || pattern == "/":
		if prefix == "" {
			return "/"
		}
		return prefix
	case prefix == "":
		return ensureLeadingSlash(pattern)
	}
	return strings.TrimSuffix(prefix, "/") + ensureLeadingSlash(pattern)
}

func ensureLeadingSlash(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

// handlerName resolves a registered handler to its fully qualified Go name,
// which is the only identifier a route carries without being annotated.
func handlerName(h any) string {
	if h == nil {
		return ""
	}
	v := reflect.ValueOf(h)
	if v.Kind() != reflect.Func {
		return reflect.TypeOf(h).String()
	}
	fn := runtime.FuncForPC(v.Pointer())
	if fn == nil {
		return ""
	}
	// Method values registered as h.HandleList compile to a wrapper whose name
	// carries a -fm suffix; it is noise in an operationId.
	return strings.TrimSuffix(fn.Name(), "-fm")
}

// moduleOf extracts the feature directory from a handler name, which is what
// groups operations into tags without any module having to declare one.
func moduleOf(handler string) string {
	const marker = "/modules/"
	i := strings.Index(handler, marker)
	if i < 0 {
		if j := strings.LastIndex(handler, "/core/"); j >= 0 {
			rest := handler[j+len("/core/"):]
			return strings.SplitN(rest, ".", 2)[0]
		}
		return ""
	}
	rest := handler[i+len(marker):]
	return strings.SplitN(rest, ".", 2)[0]
}

// operationID builds a stable, unique identifier for a generated client method.
func operationID(op Operation) string {
	name := op.Handler
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	name = trimHandlePrefix(name)
	if name == "" {
		name = strings.ToLower(op.Method)
	}
	if op.Module == "" {
		return lowerFirst(name)
	}
	return lowerFirst(camel(op.Module) + name)
}

func camel(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' || r == '/' })
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// trimHandlePrefix drops the Handle convention in either case, because the
// unexported handleX methods of nested packages would otherwise leak into names.
func trimHandlePrefix(name string) string {
	for _, prefix := range []string{"Handle", "handle"} {
		if rest := strings.TrimPrefix(name, prefix); rest != name {
			if rest == "" {
				return ""
			}
			return strings.ToUpper(rest[:1]) + rest[1:]
		}
	}
	return name
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
