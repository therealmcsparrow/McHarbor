// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Unrecorded walks the live chi tree and returns every "METHOD /path" it serves
// that the registry never saw, which is how a route mounted through Mux() escapes
// both the document and the route-gating contract.
func Unrecorded(reg *Registry, root chi.Routes) ([]string, error) {
	known := make(map[string]bool, reg.Len())
	anyMethod := make(map[string]bool)
	for _, op := range reg.Operations() {
		path := normalizeRoute(op.Path)
		if op.Method == MethodAny {
			anyMethod[path] = true
			continue
		}
		known[op.Method+" "+path] = true
	}

	seen := make(map[string]bool)
	err := chi.Walk(root, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		path := normalizeRoute(route)
		if !strings.HasPrefix(path, "/api") {
			return nil
		}
		if known[method+" "+path] || anyMethod[path] {
			return nil
		}
		seen[method+" "+path] = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// normalizeRoute folds chi's trailing slash and "/*" subtree markers, so the
// walked pattern of r.Route("/x", r.Get("/")) matches the recorded "/api/x".
func normalizeRoute(route string) string {
	route = strings.ReplaceAll(route, "/*/", "/")
	route = strings.TrimSuffix(route, "/*")
	if len(route) > 1 {
		route = strings.TrimSuffix(route, "/")
	}
	return route
}
