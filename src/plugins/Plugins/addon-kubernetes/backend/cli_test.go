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

type fakeCLIRegistry struct {
	root     *plugin.CLIRoot
	commands []plugin.CLICommand
	err      error
}

func (f *fakeCLIRegistry) Root(root plugin.CLIRoot) error {
	if f.root != nil {
		f.err = errTwoRoots
		return f.err
	}
	f.root = &root
	return nil
}

func (f *fakeCLIRegistry) Add(cmd plugin.CLICommand) error {
	if f.root == nil {
		f.err = errAddBeforeRoot
		return f.err
	}
	f.commands = append(f.commands, cmd)
	return nil
}

var (
	errTwoRoots      = &cliTestError{"Root called twice"}
	errAddBeforeRoot = &cliTestError{"Add called before Root"}
)

type cliTestError struct{ msg string }

func (e *cliTestError) Error() string { return e.msg }

// manifestRoute is one `contributes.routes` entry as the manifest declares it.
type manifestRoute struct {
	method   string
	path     string
	requires string
}

var (
	reManifestMethod   = regexp.MustCompile(`^\s{4}- method:\s*(\S+)\s*$`)
	reManifestPath     = regexp.MustCompile(`^\s{6}path:\s*"?([^"\s]+)"?\s*$`)
	reManifestRequires = regexp.MustCompile(`^\s{6}requires_permission:\s*"?([^"\s]*)"?\s*$`)
	reManifestSection  = regexp.MustCompile(`^\s{2}[a-z_]+:\s*$`)
	reRegisteredRoute  = regexp.MustCompile(`reg\.(?:Protected|Public)\(\s*http\.Method(\w+),\s*"([^"]+)"`)
)

// readManifestRoutes parses the routes block of ../plugin.yaml by hand.
//
// A YAML library would be a dependency the .so does not otherwise need, and the
// point of this test is the permission agreement, not the parser: the manifest is
// written one key per line, so a line scanner reads it exactly.
func readManifestRoutes(t *testing.T) map[string]manifestRoute {
	t.Helper()
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	routes := map[string]manifestRoute{}
	inRoutes := false
	var current *manifestRoute
	flush := func() {
		if current != nil && current.method != "" && current.path != "" {
			routes[strings.ToUpper(current.method)+" "+current.path] = *current
		}
		current = nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(line) == "routes:" {
			inRoutes = true
			continue
		}
		if !inRoutes {
			continue
		}
		if reManifestSection.MatchString(line) {
			flush()
			inRoutes = false
			continue
		}
		if m := reManifestMethod.FindStringSubmatch(line); m != nil {
			flush()
			current = &manifestRoute{method: m[1]}
			continue
		}
		if current == nil {
			continue
		}
		if m := reManifestPath.FindStringSubmatch(line); m != nil {
			current.path = m[1]
			continue
		}
		if m := reManifestRequires.FindStringSubmatch(line); m != nil {
			current.requires = m[1]
		}
	}
	flush()
	if len(routes) == 0 {
		t.Fatal("parsed no routes from ../plugin.yaml")
	}
	return routes
}

// readRegisteredRoutes parses main.go's RegisterRoutes so a command naming a route
// nobody registers fails here rather than at runtime.
func readRegisteredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	found := map[string]bool{}
	for _, m := range reRegisteredRoute.FindAllStringSubmatch(string(raw), -1) {
		found[strings.ToUpper(m[1])+" "+m[2]] = true
	}
	if len(found) == 0 {
		t.Fatal("parsed no route registrations from main.go")
	}
	return found
}

func buildCLI(t *testing.T) *fakeCLIRegistry {
	t.Helper()
	reg := &fakeCLIRegistry{}
	p := &KubernetesPlugin{manifest: manifest}
	if err := p.RegisterCLI(reg); err != nil {
		t.Fatalf("RegisterCLI: %v", err)
	}
	if reg.err != nil {
		t.Fatalf("registry rejected the set: %v", reg.err)
	}
	return reg
}

func TestCLIRootAccepted(t *testing.T) {
	reg := buildCLI(t)
	if reg.root == nil {
		t.Fatal("no root declared")
	}
	if reg.root.Name != "k8s" {
		t.Fatalf("root name = %q, want k8s", reg.root.Name)
	}
	if reg.root.Short == "" {
		t.Error("root has no Short, so `mcharbor --help` would list a blank line")
	}
	if len(reg.commands) != 13 {
		t.Fatalf("declared %d commands, want 13 — one per registered route", len(reg.commands))
	}
}

// TestCLINeedsMatchesManifest is the assertion the whole set depends on: the host
// drops every command when one Needs disagrees with its route's declared
// permission, and nothing else in the build would notice the drift.
func TestCLINeedsMatchesManifest(t *testing.T) {
	declared := readManifestRoutes(t)
	for _, cmd := range buildCLI(t).commands {
		path := strings.Join(cmd.Path, " ")
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		route, found := declared[key]
		if !found {
			t.Errorf("command %q names %s, which ../plugin.yaml does not declare", path, key)
			continue
		}
		if route.requires == "" {
			t.Errorf("command %q names %s, whose declared permission is empty — the host cannot gate it", path, key)
			continue
		}
		if route.requires != cmd.Needs {
			t.Errorf("command %q needs %q but %s declares %q", path, cmd.Needs, key, route.requires)
		}
	}
}

func TestCLIRoutesAreRegistered(t *testing.T) {
	registered := readRegisteredRoutes(t)
	for _, cmd := range buildCLI(t).commands {
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		if !registered[key] {
			t.Errorf("command %q names %s, which RegisterRoutes does not register",
				strings.Join(cmd.Path, " "), key)
		}
	}
}

func TestCLIListCommandsHaveColumns(t *testing.T) {
	for _, cmd := range buildCLI(t).commands {
		switch cmd.Kind {
		case plugin.CLIList:
			if len(cmd.Columns) == 0 {
				t.Errorf("list command %q has no columns", strings.Join(cmd.Path, " "))
			}
		case plugin.CLIDownload:
			if len(cmd.Columns) > 0 {
				t.Errorf("download command %q declares columns", strings.Join(cmd.Path, " "))
			}
		case plugin.CLIAttach:
			if cmd.GuestKind == "" {
				t.Errorf("attach command %q has no guest kind", strings.Join(cmd.Path, " "))
			}
		}
	}
}

func TestCLIPathsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, cmd := range buildCLI(t).commands {
		path := strings.Join(cmd.Path, " ")
		if seen[path] {
			t.Errorf("duplicate command path %q", path)
		}
		seen[path] = true
		if len(cmd.Path) == 0 {
			t.Error("a command has an empty path")
		}
	}
}
