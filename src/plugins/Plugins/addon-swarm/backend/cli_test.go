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
	for _, alias := range root.Aliases {
		if !cliRootNamePattern.MatchString(alias) {
			f.t.Fatalf("root alias %q does not match %s", alias, cliRootNamePattern)
		}
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
// Hand-scanned rather than unmarshalled on purpose. This module already has
// yaml.v3 for Compose parsing, but the compute, kvm and lxc plugins do not, and
// a check that reads differently per plugin is a check whose disagreements are
// nobody's fault. The routes block's shape is fixed, so the scan is exact.
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

// registeredRoutePattern matches one registration call in RegisterRoutes. The
// call's shape is the assertion: a route mounted in any other form is invisible
// to this test and fails the coverage check below.
var registeredRoutePattern = regexp.MustCompile(`reg\.(?:Protected|Public)\(http\.Method([A-Za-z]+),\s*"([^"]+)"`)

// registeredRoutes parses the registrations out of main.go.
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
		t.Fatal("no routes parsed out of main.go; the registration's shape changed and this test is now blind")
	}
	return out
}

// declared builds the command set through the fake registry.
func declared(t *testing.T) *fakeCLIRegistry {
	t.Helper()
	reg := &fakeCLIRegistry{t: t}
	plug := &SwarmPlugin{manifest: manifest, handler: &Handler{}}
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
	if len(reg.cmds) != 46 {
		t.Fatalf("declared %d commands, want 46 — one per registered route", len(reg.cmds))
	}
}

// TestEveryCommandNeedsItsRoutesDeclaredPermission is the assertion that matters
// most: the host refuses a command whose Needs differs from the
// requires_permission its route declares, and drops the plugin's entire command
// set with it. Nothing else in the repo compares these two strings.
func TestEveryCommandNeedsItsRoutesDeclaredPermission(t *testing.T) {
	routes := manifestRoutes(t)
	if len(routes) < 46 {
		t.Fatalf("parsed %d routes out of the manifest, expected at least 46; the parser is broken", len(routes))
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

// TestNoCommandAdvertisesAnUngatedRoute is the second half of the permission
// story, and the reason `swarm cluster capability` reads `swarm.view` here rather
// than the empty string the design document recorded: an empty
// requires_permission short-circuits the plugin permission middleware to a
// pass-through, so the route becomes reachable by any authenticated user.
func TestNoCommandAdvertisesAnUngatedRoute(t *testing.T) {
	for _, cmd := range declared(t).cmds {
		if strings.TrimSpace(cmd.Needs) == "" {
			t.Errorf("command %q declares no permission; an empty key is a pass-through, not a gate",
				strings.Join(cmd.Path, " "))
		}
	}
	for _, route := range manifestRoutes(t) {
		if route.public || strings.TrimSpace(route.requires) != "" {
			continue
		}
		t.Errorf("route %s %s is declared with no permission and is not public",
			route.method, route.path)
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
	}
}

// TestEveryCommandIsEnvScoped pins the one thing about this plugin that is true
// of all of it: every handler resolves its daemon through response.ParseEnvID,
// so a command that forgot EnvScoped would silently address whichever
// environment the CLI context happened to hold.
func TestEveryCommandIsEnvScoped(t *testing.T) {
	for _, cmd := range declared(t).cmds {
		if !cmd.EnvScoped {
			t.Errorf("command %q is not EnvScoped, but %s %s reads ?env=",
				strings.Join(cmd.Path, " "), cmd.Method, cmd.Route)
		}
	}
}

// TestNoFlagShadowsTheEnvSelector guards a collision the contract cannot see: a
// service's environment entries are a body field also called `env`, and a flag
// named --env would shadow the CLI's own environment selector on an EnvScoped
// command.
func TestNoFlagShadowsTheEnvSelector(t *testing.T) {
	for _, cmd := range declared(t).cmds {
		for _, f := range cmd.Flags {
			if f.Name == "env" {
				t.Errorf("command %q declares a flag named --env, which shadows the environment selector",
					strings.Join(cmd.Path, " "))
			}
		}
	}
}
