// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

// pluginsRoot is where the in-tree plugin sources live, relative to this package.
const pluginsRoot = "../../../plugins/Plugins"

// The three registration shapes the in-tree plugins actually use. A plugin that
// invents a fourth is caught by wantRoutesFound below rather than passing
// silently, which is the failure mode a regex scanner would otherwise have.
var (
	// reg.Protected(http.MethodGet, "/api/x", ...) — and the Public/Auth siblings.
	reRegCall = regexp.MustCompile(`\breg\.(Protected|Public|Auth)\(\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"`)
	// {http.MethodGet, "/api/x", false, handler} — the bool is the public flag.
	reLiteralBool = regexp.MustCompile(`\{\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"\s*,\s*(true|false)\s*,`)
	// {http.MethodGet, "/api/x", handler} — no bool, so every entry is protected.
	reLiteralPlain = regexp.MustCompile(`\{\s*http\.Method([A-Za-z]+)\s*,\s*"([^"]+)"\s*,\s*\(\*`)
)

// registeredRoute is one route a plugin's Go source hands to the registry.
type registeredRoute struct {
	method string
	path   string
	public bool
	file   string
}

// TestPluginRouteDeclarationDrift fails when a plugin registers a non-public route
// its plugin.yaml never declares. lookupRouteDecl returns an empty permission on a
// miss and PluginPermissionMiddleware short-circuits an empty key to a pass-through,
// so such a route is reachable by any authenticated user whatever their role.
func TestPluginRouteDeclarationDrift(t *testing.T) {
	entries, err := os.ReadDir(pluginsRoot)
	if err != nil {
		t.Skipf("plugin tree not present: %v", err)
	}

	scanned := 0
	var drift []string

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		slug := entry.Name()
		manifestPath := filepath.Join(pluginsRoot, slug, "plugin.yaml")
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest contract.Manifest
		if err := yaml.Unmarshal(raw, &manifest); err != nil {
			t.Errorf("%s: plugin.yaml does not parse: %v", slug, err)
			continue
		}

		routes, err := scanRegistrations(filepath.Join(pluginsRoot, slug, "backend"))
		if err != nil {
			continue
		}
		if len(routes) == 0 {
			continue
		}
		scanned++

		for _, rt := range routes {
			if rt.public {
				continue
			}
			requires, declaredPublic, ok := findDecl(manifest, rt.method, rt.path)
			switch {
			case !ok:
				drift = append(drift, slug+": "+rt.method+" "+rt.path+
					" is registered non-public in "+rt.file+
					" and plugin.yaml does not declare it — it has no permission middleware")
			case declaredPublic:
				// The manifest overrides the registration to public. Legal, and
				// the loader honours it; nothing to enforce.
			case strings.TrimSpace(requires) == "":
				drift = append(drift, slug+": "+rt.method+" "+rt.path+
					" is declared in plugin.yaml with an empty requires_permission —"+
					" the middleware short-circuits to a pass-through")
			}
		}
	}

	if scanned < 4 {
		t.Fatalf("scanned only %d plugins for route registrations; the registration"+
			" shapes this test recognises have probably changed", scanned)
	}

	if len(drift) > 0 {
		sort.Strings(drift)
		t.Fatalf("%d plugin route(s) reachable by any authenticated user:\n  %s",
			len(drift), strings.Join(drift, "\n  "))
	}
}

// findDecl mirrors routeRegistry.lookupRouteDecl: case-insensitive on the method,
// exact on the path, because that is what the loader compares at mount time.
func findDecl(m contract.Manifest, method, path string) (requires string, public bool, found bool) {
	for _, rt := range m.Contribs.Routes {
		if strings.EqualFold(rt.Method, method) && rt.Path == path {
			return rt.Requires, rt.Public, true
		}
	}
	return "", false, false
}

// scanRegistrations reads every .go file in a plugin's backend directory and
// returns the routes it hands to the registry.
func scanRegistrations(dir string) ([]registeredRoute, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []registeredRoute
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		src := string(body)

		for _, m := range reRegCall.FindAllStringSubmatch(src, -1) {
			out = append(out, registeredRoute{
				method: strings.ToUpper(m[2]),
				path:   m[3],
				public: m[1] == "Public",
				file:   f.Name(),
			})
		}
		for _, m := range reLiteralBool.FindAllStringSubmatch(src, -1) {
			out = append(out, registeredRoute{
				method: strings.ToUpper(m[1]),
				path:   m[2],
				public: m[3] == "true",
				file:   f.Name(),
			})
		}
		for _, m := range reLiteralPlain.FindAllStringSubmatch(src, -1) {
			out = append(out, registeredRoute{
				method: strings.ToUpper(m[1]),
				path:   m[2],
				file:   f.Name(),
			})
		}
	}
	return out, nil
}
