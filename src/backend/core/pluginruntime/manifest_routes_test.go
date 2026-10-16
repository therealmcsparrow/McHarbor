// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// manifestRoutesRoot is where the in-tree plugin sources live, relative to
// this package.
const manifestRoutesRoot = "../../../plugins/Plugins"

// knownRouteSetMismatches names a plugin's main.go routes that plugin.yaml
// does not declare, each recorded so the loader's silent gap is visible
// instead of assumed away. A route that gains a manifest entry fails the
// test below, so this list can only shrink.
var knownRouteSetMismatches = map[string]bool{}

// manifestRoute is the part of plugin.yaml's contributes.routes this test reads.
type manifestRoute struct {
	Method          string `yaml:"method"`
	Path            string `yaml:"path"`
	Public          bool   `yaml:"public"`
	AuditRequired   *bool  `yaml:"audit_required"`
	AuditAction     string `yaml:"audit_action"`
	AuditEntityType string `yaml:"audit_entity_type"`
}

// routeManifest is the slice of plugin.yaml this test reads.
type routeManifest struct {
	Slug        string `yaml:"slug"`
	Contributes struct {
		Routes []manifestRoute `yaml:"routes"`
	} `yaml:"contributes"`
}

// codeRoute is one route src/parser found registered from Go source.
type codeRoute struct {
	method string
	path   string
}

// key returns the method+path pair used to compare against a plugin.yaml route.
func (r codeRoute) key() string { return r.method + " " + r.path }

// methodFromSelector turns the identifier half of http.MethodGet into "GET".
// It returns "" for anything that is not an http.Method* selector, which the
// caller treats as "not a route this scanner recognises".
func methodFromSelector(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "http" || !strings.HasPrefix(sel.Sel.Name, "Method") {
		return ""
	}
	return strings.ToUpper(strings.TrimPrefix(sel.Sel.Name, "Method"))
}

// stringLiteral unquotes a *ast.BasicLit string literal, or reports ok=false
// for anything else (a variable, a concatenation, ...).
func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

// scanCodeRoutes walks every non-test .go file under dir and returns every
// route it can recognise from two shapes: a direct
// reg.Protected/Public/Auth(http.MethodX, "path", handler) call, and a route
// table entry {http.MethodX, "path", ...} inside a slice literal — the two
// shapes the in-tree plugins use. It never imports the plugin package; it
// only parses the source text with go/parser.
func scanCodeRoutes(t *testing.T, dir string) []codeRoute {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var routes []codeRoute
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Errorf("parsing %s: %v", filepath.Join(dir, name), err)
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name != "Protected" && sel.Sel.Name != "Public" && sel.Sel.Name != "Auth" {
					return true
				}
				if len(node.Args) < 2 {
					return true
				}
				method := methodFromSelector(node.Args[0])
				path, ok := stringLiteral(node.Args[1])
				if method == "" || !ok {
					return true
				}
				routes = append(routes, codeRoute{method: method, path: path})
			case *ast.CompositeLit:
				if len(node.Elts) < 2 {
					return true
				}
				method := methodFromSelector(node.Elts[0])
				if method == "" {
					return true
				}
				path, ok := stringLiteral(node.Elts[1])
				if !ok {
					return true
				}
				routes = append(routes, codeRoute{method: method, path: path})
			}
			return true
		})
	}
	return routes
}

// loadRouteManifest reads and parses one plugin's plugin.yaml.
func loadRouteManifest(t *testing.T, path string) (routeManifest, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return routeManifest{}, false
	}
	var man routeManifest
	if err := yaml.Unmarshal(raw, &man); err != nil {
		t.Errorf("parse %s: %v", path, err)
		return routeManifest{}, false
	}
	return man, true
}

// TestPluginManifestRoutesMatchMain fails when a route registered in a
// plugin's backend/main.go has no matching entry in plugin.yaml, or vice
// versa: the manifest is what the loader's PluginPermissionMiddleware and
// audit fallback both read, so a route missing from it is reachable (or
// audited) on nothing but the code's own say-so.
func TestPluginManifestRoutesMatchMain(t *testing.T) {
	entries, err := os.ReadDir(manifestRoutesRoot)
	if err != nil {
		t.Skipf("plugin tree not present: %v", err)
	}

	scanned := 0
	var mismatched []string
	stillMismatched := map[string]bool{}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		slug := entry.Name()
		man, ok := loadRouteManifest(t, filepath.Join(manifestRoutesRoot, slug, "plugin.yaml"))
		if !ok {
			continue
		}
		codeRoutes := scanCodeRoutes(t, filepath.Join(manifestRoutesRoot, slug, "backend"))
		if len(codeRoutes) == 0 {
			continue
		}
		scanned++

		inCode := map[string]bool{}
		for _, r := range codeRoutes {
			inCode[r.key()] = true
		}
		inManifest := map[string]bool{}
		for _, r := range man.Contributes.Routes {
			inManifest[strings.ToUpper(r.Method)+" "+r.Path] = true
		}

		for key := range inCode {
			if inManifest[key] {
				continue
			}
			label := slug + ": " + key
			if knownRouteSetMismatches[label] {
				stillMismatched[label] = true
				continue
			}
			mismatched = append(mismatched, label+" (registered in main.go, absent from plugin.yaml)")
		}
		for key := range inManifest {
			if inCode[key] {
				continue
			}
			label := slug + ": " + key
			if knownRouteSetMismatches[label] {
				stillMismatched[label] = true
				continue
			}
			mismatched = append(mismatched, label+" (declared in plugin.yaml, never registered)")
		}
	}

	if scanned < 4 {
		t.Fatalf("scanned only %d plugins with recognisable route registrations; the shapes this "+
			"test recognises have probably changed", scanned)
	}

	for label := range knownRouteSetMismatches {
		if !stillMismatched[label] {
			t.Errorf("%s no longer mismatches; remove it from knownRouteSetMismatches", label)
		}
	}
	sort.Strings(mismatched)
	if len(mismatched) > 0 {
		t.Errorf("%d routes disagree between backend/main.go and plugin.yaml:\n  %s",
			len(mismatched), strings.Join(mismatched, "\n  "))
	}
}

// knownUndeclaredAuditRoutes names mutating plugin routes with no
// audit_action/audit_entity_type and no explicit audit_required: false.
// The four this check first found were declared on 2026-09-29, so the list is
// empty; a route that gains audit fields fails the test below, so it can only shrink.
var knownUndeclaredAuditRoutes = map[string]bool{}

// TestPluginMutatingRoutesDeclareAudit fails when a mutating plugin route
// (POST/PUT/PATCH/DELETE) declares neither an audit action and entity type
// nor an explicit audit_required: false — the shape that leaves a plugin
// mutation falling back to whatever the generic audit middleware guesses.
func TestPluginMutatingRoutesDeclareAudit(t *testing.T) {
	entries, err := os.ReadDir(manifestRoutesRoot)
	if err != nil {
		t.Skipf("plugin tree not present: %v", err)
	}

	scanned := 0
	var undeclared []string
	stillUndeclared := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		slug := entry.Name()
		man, ok := loadRouteManifest(t, filepath.Join(manifestRoutesRoot, slug, "plugin.yaml"))
		if !ok {
			continue
		}
		for _, r := range man.Contributes.Routes {
			scanned++
			switch strings.ToUpper(r.Method) {
			case "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}
			if r.AuditRequired != nil && !*r.AuditRequired {
				continue
			}
			if strings.TrimSpace(r.AuditAction) != "" && strings.TrimSpace(r.AuditEntityType) != "" {
				continue
			}
			label := slug + ": " + strings.ToUpper(r.Method) + " " + r.Path
			if knownUndeclaredAuditRoutes[label] {
				stillUndeclared[label] = true
				continue
			}
			undeclared = append(undeclared, label)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned zero plugin.yaml routes; the manifest glob or field names have probably changed")
	}
	for label := range knownUndeclaredAuditRoutes {
		if !stillUndeclared[label] {
			t.Errorf("%s now declares audit fields; remove it from knownUndeclaredAuditRoutes", label)
		}
	}
	sort.Strings(undeclared)
	if len(undeclared) > 0 {
		t.Errorf("%d mutating plugin routes carry neither audit_action+audit_entity_type nor "+
			"audit_required: false:\n  %s", len(undeclared), strings.Join(undeclared, "\n  "))
	}
}
