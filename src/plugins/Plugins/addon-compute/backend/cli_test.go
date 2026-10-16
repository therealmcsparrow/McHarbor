// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The CLI declaration has three parties that must agree and no compiler to make
// them: this file, `RegisterRoutes` in main.go, and the `contributes.routes`
// block in plugin.yaml. The host drops the whole command set when they disagree
// — silently, as far as an operator is concerned, because the plugin keeps
// working and simply has no commands. These tests are what turns that into a
// failed build.

// fakeCLIRegistry stands in for the host's registry, applying the same
// requirements `pluginruntime.cliRegistry` does so a set that passes here is one
// the loader will accept.
type fakeCLIRegistry struct {
	root  *plugin.CLIRoot
	cmds  []plugin.CLICommand
	paths map[string]bool
	t     *testing.T
}

// cliRootNamePattern is the shape the host requires of a root command name.
var cliRootNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// Root records the root and rejects a name the host would reject.
func (f *fakeCLIRegistry) Root(root plugin.CLIRoot) error {
	f.t.Helper()
	if !cliRootNamePattern.MatchString(root.Name) {
		f.t.Fatalf("root name %q does not match %s", root.Name, cliRootNamePattern)
	}
	if strings.TrimSpace(root.Short) == "" {
		f.t.Fatalf("root %q has no short description; it is what `mcharbor --help` prints", root.Name)
	}
	f.root = &root
	return nil
}

// Add records one command after the per-kind and uniqueness checks.
func (f *fakeCLIRegistry) Add(cmd plugin.CLICommand) error {
	f.t.Helper()
	if len(cmd.Path) == 0 {
		f.t.Fatalf("command for route %s %s has an empty path", cmd.Method, cmd.Route)
	}
	name := strings.Join(cmd.Path, " ")
	if f.paths == nil {
		f.paths = map[string]bool{}
	}
	if f.paths[name] {
		f.t.Fatalf("duplicate command path %q", name)
	}
	f.paths[name] = true
	f.cmds = append(f.cmds, cmd)
	return nil
}

// declaredRoute is one entry of the manifest's contributes.routes block.
type declaredRoute struct {
	method   string
	path     string
	requires string
	public   bool
}

// manifestRoutes parses the routes the manifest declares.
//
// Hand-scanned rather than unmarshalled on purpose: a YAML dependency in this
// module would change the module graph, and the plugin loader asserts the built
// `.so` against its manifest and its install directory, so a dependency added
// for a test is a dependency that has to travel with the artifact.
func manifestRoutes(t *testing.T) []declaredRoute {
	t.Helper()
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	start, indent := -1, 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "routes:" {
			start, indent = i, len(line)-len(strings.TrimLeft(line, " "))
			break
		}
	}
	if start < 0 {
		t.Fatal("the manifest has no routes: block")
	}

	var out []declaredRoute
	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(line)-len(strings.TrimLeft(line, " ")) <= indent {
			break
		}
		switch {
		case strings.HasPrefix(trimmed, "- method:"):
			out = append(out, declaredRoute{method: yamlValue(trimmed[len("- method:"):])})
		case len(out) == 0:
			continue
		case strings.HasPrefix(trimmed, "path:"):
			out[len(out)-1].path = yamlValue(trimmed[len("path:"):])
		case strings.HasPrefix(trimmed, "requires_permission:"):
			out[len(out)-1].requires = yamlValue(trimmed[len("requires_permission:"):])
		case strings.HasPrefix(trimmed, "public:"):
			out[len(out)-1].public = yamlValue(trimmed[len("public:"):]) == "true"
		}
	}
	return out
}

// yamlValue strips the quoting a scalar may carry.
func yamlValue(in string) string {
	return strings.Trim(strings.TrimSpace(in), `"'`)
}

// registeredRoutePattern matches one row of the struct-literal route table in
// RegisterRoutes. The table's shape is the assertion: a route added there in any
// other form is invisible to this test and fails the coverage check below.
var registeredRoutePattern = regexp.MustCompile(`\{http\.Method([A-Za-z]+),\s*"([^"]+)",\s*(true|false),`)

// registeredRoutes parses the route table out of main.go.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	out := map[string]bool{}
	for _, m := range registeredRoutePattern.FindAllStringSubmatch(string(raw), -1) {
		out[strings.ToUpper(m[1])+" "+m[2]] = true
	}
	if len(out) == 0 {
		t.Fatal("no routes parsed out of main.go; the route table's shape changed and this test is now blind")
	}
	return out
}

// declared builds the command set through the fake registry.
func declared(t *testing.T) *fakeCLIRegistry {
	t.Helper()
	reg := &fakeCLIRegistry{t: t}
	plug := &ComputePlugin{manifest: manifest}
	if err := plug.RegisterCLI(reg); err != nil {
		t.Fatalf("RegisterCLI: %v", err)
	}
	if reg.root == nil {
		t.Fatal("RegisterCLI returned without claiming a root")
	}
	return reg
}

// TestCLIRootIsAccepted checks the root the whole group hangs off.
func TestCLIRootIsAccepted(t *testing.T) {
	reg := declared(t)
	if reg.root.Name != cliRoot {
		t.Fatalf("root name = %q, want %q", reg.root.Name, cliRoot)
	}
	if len(reg.cmds) != 43 {
		t.Fatalf("declared %d commands, want 43 — one per Protected route", len(reg.cmds))
	}
}

// TestEveryCommandNeedsItsRoutesDeclaredPermission is the assertion that matters
// most: the host refuses a command whose Needs differs from the
// requires_permission its route declares, and drops the plugin's entire command
// set with it. Nothing else in the repo compares these two strings.
func TestEveryCommandNeedsItsRoutesDeclaredPermission(t *testing.T) {
	routes := manifestRoutes(t)
	if len(routes) < 42 {
		t.Fatalf("parsed %d routes out of the manifest, expected at least 42; the parser is broken", len(routes))
	}
	byKey := map[string]declaredRoute{}
	for _, r := range routes {
		byKey[strings.ToUpper(r.method)+" "+r.path] = r
	}

	for _, cmd := range declared(t).cmds {
		name := strings.Join(cmd.Path, " ")
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		route, found := byKey[key]
		if !found {
			t.Errorf("command %q names %s, which the manifest does not declare", name, key)
			continue
		}
		if route.public {
			t.Errorf("command %q names %s, a public route; a command may not address one", name, key)
			continue
		}
		if route.requires != cmd.Needs {
			t.Errorf("command %q needs %q but %s declares %q", name, cmd.Needs, key, route.requires)
		}
	}
}

// TestEveryCommandRouteIsRegistered proves the declaration names a route the
// plugin actually mounts. A command against an unregistered route is a 404 an
// operator discovers, which is the failure the shared route string exists to
// prevent.
func TestEveryCommandRouteIsRegistered(t *testing.T) {
	registered := registeredRoutes(t)
	for _, cmd := range declared(t).cmds {
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		if !registered[key] {
			t.Errorf("command %q names %s, which RegisterRoutes does not mount",
				strings.Join(cmd.Path, " "), key)
		}
	}
}

// TestCommandKindRequirements mirrors the host's per-kind validation, so a
// declaration that would be refused at load time fails here instead.
func TestCommandKindRequirements(t *testing.T) {
	for _, cmd := range declared(t).cmds {
		name := strings.Join(cmd.Path, " ")
		switch cmd.Kind {
		case plugin.CLIList:
			if len(cmd.Columns) == 0 {
				t.Errorf("list command %q has no columns", name)
			}
		case plugin.CLIAttach:
			if cmd.GuestKind == "" {
				t.Errorf("attach command %q has no guest kind", name)
			}
		case plugin.CLIUpload:
			files := 0
			for _, f := range cmd.Flags {
				if f.Type == plugin.FlagFile {
					files++
				}
			}
			if files != 1 {
				t.Errorf("upload command %q has %d file flags, want exactly 1", name, files)
			}
		case plugin.CLIDownload:
			if len(cmd.Columns) > 0 {
				t.Errorf("download command %q declares columns, which it cannot render", name)
			}
		case plugin.CLIGet, plugin.CLIAction, plugin.CLIText:
		default:
			t.Errorf("command %q has unknown kind %q", name, cmd.Kind)
		}
	}
}

// TestCommandPathsAreUniqueAndDescribed guards the uniqueness the host's index
// enforces, and the help text an operator has nothing else to read.
func TestCommandPathsAreUniqueAndDescribed(t *testing.T) {
	seen := map[string]bool{}
	for _, cmd := range declared(t).cmds {
		name := strings.Join(cmd.Path, " ")
		if seen[name] {
			t.Errorf("duplicate command path %q", name)
		}
		seen[name] = true
		if strings.TrimSpace(cmd.Short) == "" {
			t.Errorf("command %q has no short description", name)
		}
		if strings.TrimSpace(cmd.Needs) == "" {
			t.Errorf("command %q declares no permission", name)
		}
	}
}

// TestUploadCommandMatchesTheHandlersFieldName pins the one multipart contract
// the contract types cannot express: HandleUploadImage ends its part loop on the
// part named "file" and reads every other part as a metadata field, so a file
// flag under any other field name uploads nothing and reports an empty body.
func TestUploadCommandMatchesTheHandlersFieldName(t *testing.T) {
	var found bool
	for _, cmd := range declared(t).cmds {
		if cmd.Kind != plugin.CLIUpload {
			continue
		}
		found = true
		for _, f := range cmd.Flags {
			if f.Type != plugin.FlagFile {
				continue
			}
			if f.Field != "file" {
				t.Errorf("upload command %q sends its file as %q; HandleUploadImage reads %q",
					strings.Join(cmd.Path, " "), f.Field, "file")
			}
			if f.Target != plugin.TargetBody {
				t.Errorf("upload command %q targets its file flag at %q, want %q",
					strings.Join(cmd.Path, " "), f.Target, plugin.TargetBody)
			}
		}
	}
	if !found {
		t.Fatal("no upload command declared; POST /api/compute/images/upload is the one route that takes bytes")
	}
}
