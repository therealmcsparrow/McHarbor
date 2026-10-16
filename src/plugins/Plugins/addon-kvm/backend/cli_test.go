// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// fakeCLIRegistry collects what RegisterCLI declares, in order, and enforces the
// one ordering rule the host does: Root before Add.
type fakeCLIRegistry struct {
	root     *plugin.CLIRoot
	commands []plugin.CLICommand
	err      error
}

func (f *fakeCLIRegistry) Root(root plugin.CLIRoot) error {
	if f.root != nil {
		f.err = fmt.Errorf("Root called twice")
		return f.err
	}
	f.root = &root
	return nil
}

func (f *fakeCLIRegistry) Add(cmd plugin.CLICommand) error {
	if f.root == nil {
		f.err = fmt.Errorf("Add called before Root")
		return f.err
	}
	f.commands = append(f.commands, cmd)
	return nil
}

// fakeRouteRegistry collects what RegisterRoutes mounts, so the command table is
// checked against the real registration rather than against the manifest twice.
type fakeRouteRegistry struct {
	mounted map[string]bool
}

func (f *fakeRouteRegistry) record(method, path string) {
	if f.mounted == nil {
		f.mounted = map[string]bool{}
	}
	f.mounted[strings.ToUpper(method)+" "+path] = true
}

func (f *fakeRouteRegistry) Public(method, path string, _ http.HandlerFunc) {
	f.record(method, path)
}

func (f *fakeRouteRegistry) Auth(method, path string, _ http.HandlerFunc) {
	f.record(method, path)
}

func (f *fakeRouteRegistry) Protected(method, path string, _ http.HandlerFunc) {
	f.record(method, path)
}

// manifestRoute is one declared route as the host reads it when it decides
// whether a command's Needs agrees with what the middleware enforces.
type manifestRoute struct {
	requires string
	public   bool
}

var (
	reManifestRouteMethod = regexp.MustCompile(`^\s*-\s+method:\s*"?([A-Za-z]+)"?\s*$`)
	reManifestRoutePath   = regexp.MustCompile(`^\s+path:\s*"([^"]+)"\s*$`)
	reManifestRequires    = regexp.MustCompile(`^\s+requires_permission:\s*"([^"]*)"\s*$`)
	reManifestPublic      = regexp.MustCompile(`^\s+public:\s*(true|false)\s*$`)
	reManifestSection     = regexp.MustCompile(`^  ([a-z_]+):\s*$`)
	reCLIRootName         = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)
)

// readManifestRoutes parses contributes.routes out of plugin.yaml by hand.
//
// A line scanner rather than a YAML decode on purpose: adding gopkg.in/yaml.v3 to
// this plugin's go.mod would move its dependency graph, and a plugin's build IDs
// are derived from that graph.
func readManifestRoutes(t *testing.T) map[string]manifestRoute {
	t.Helper()
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatalf("reading plugin.yaml: %v", err)
	}
	out := map[string]manifestRoute{}
	inRoutes := false
	method, path := "", ""
	route := manifestRoute{}
	flush := func() {
		if method != "" && path != "" {
			out[strings.ToUpper(method)+" "+path] = route
		}
		method, path, route = "", "", manifestRoute{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := reManifestSection.FindStringSubmatch(line); m != nil {
			flush()
			inRoutes = m[1] == "routes"
			continue
		}
		if !inRoutes {
			continue
		}
		if m := reManifestRouteMethod.FindStringSubmatch(line); m != nil {
			flush()
			method = m[1]
			continue
		}
		if m := reManifestRoutePath.FindStringSubmatch(line); m != nil {
			path = m[1]
			continue
		}
		if m := reManifestRequires.FindStringSubmatch(line); m != nil {
			route.requires = m[1]
			continue
		}
		if m := reManifestPublic.FindStringSubmatch(line); m != nil {
			route.public = m[1] == "true"
			continue
		}
	}
	flush()
	if len(out) == 0 {
		t.Fatal("parsed no routes out of plugin.yaml; the scanner has drifted from the manifest")
	}
	return out
}

// declaredCLI builds the whole command set once.
func declaredCLI(t *testing.T) *fakeCLIRegistry {
	t.Helper()
	reg := &fakeCLIRegistry{}
	plug := &KVMPlugin{}
	if err := plug.RegisterCLI(reg); err != nil {
		t.Fatalf("RegisterCLI: %v", err)
	}
	if reg.err != nil {
		t.Fatalf("registry rejected the set: %v", reg.err)
	}
	return reg
}

func TestCLIRootIsAccepted(t *testing.T) {
	reg := declaredCLI(t)
	if reg.root == nil {
		t.Fatal("RegisterCLI declared no root")
	}
	if !reCLIRootName.MatchString(reg.root.Name) {
		t.Errorf("root name %q does not match the host's %s", reg.root.Name, reCLIRootName)
	}
	if reg.root.Name != "kvm" {
		t.Errorf("root name = %q, want kvm", reg.root.Name)
	}
	if reg.root.Short == "" {
		t.Error("the root has no short description, so it is unlabelled in mcharbor --help")
	}
	if len(reg.root.Aliases) != 1 || reg.root.Aliases[0] != "vms" {
		t.Errorf("root aliases = %v, want [vms]", reg.root.Aliases)
	}
}

// TestCLINeedsMatchesManifest is the assertion that catches the drift no review
// will: the host refuses the whole command set when one Needs disagrees with the
// permission its route declares, and the plugin then loads with no CLI at all.
func TestCLINeedsMatchesManifest(t *testing.T) {
	reg := declaredCLI(t)
	manifest := readManifestRoutes(t)

	for _, cmd := range reg.commands {
		path := strings.Join(cmd.Path, " ")
		if cmd.Kind == plugin.CLIAttach {
			// Attach names a core route the plugin does not declare, which is
			// why the host exempts it.
			continue
		}
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		route, found := manifest[key]
		if !found {
			t.Errorf("%s: route %s is not declared in plugin.yaml", path, key)
			continue
		}
		if route.public {
			t.Errorf("%s: route %s is declared public, so no command may name it", path, key)
			continue
		}
		if route.requires != cmd.Needs {
			t.Errorf("%s: needs %q but plugin.yaml declares %q for %s",
				path, cmd.Needs, route.requires, key)
		}
	}
}

// TestCLIRoutesAreRegistered checks each Route against the table RegisterRoutes
// actually mounts, so a renamed route breaks here rather than as a 404.
func TestCLIRoutesAreRegistered(t *testing.T) {
	reg := declaredCLI(t)
	routes := &fakeRouteRegistry{}
	plug := &KVMPlugin{}
	if err := plug.RegisterRoutes(routes); err != nil {
		t.Fatalf("RegisterRoutes: %v", err)
	}
	for _, cmd := range reg.commands {
		if cmd.Kind == plugin.CLIAttach {
			continue
		}
		key := strings.ToUpper(cmd.Method) + " " + cmd.Route
		if !routes.mounted[key] {
			t.Errorf("%s: route %s is not mounted by RegisterRoutes",
				strings.Join(cmd.Path, " "), key)
		}
	}
}

// TestCLIKindRequirements mirrors validateCLIKind in the host, so a violation is
// a failing test here rather than a silently dropped command set in production.
func TestCLIKindRequirements(t *testing.T) {
	reg := declaredCLI(t)
	for _, cmd := range reg.commands {
		path := strings.Join(cmd.Path, " ")
		switch cmd.Kind {
		case plugin.CLIList:
			if len(cmd.Columns) == 0 {
				t.Errorf("%s: a list command needs at least one column", path)
			}
		case plugin.CLIAttach:
			if cmd.GuestKind != "vm" {
				t.Errorf("%s: guest kind = %q, want vm", path, cmd.GuestKind)
			}
		case plugin.CLIUpload:
			files := 0
			for _, f := range cmd.Flags {
				if f.Type == plugin.FlagFile {
					files++
				}
			}
			if files != 1 {
				t.Errorf("%s: an upload command needs exactly one file flag, found %d", path, files)
			}
		case plugin.CLIDownload:
			if len(cmd.Columns) > 0 {
				t.Errorf("%s: a download command renders no columns", path)
			}
		case plugin.CLIGet, plugin.CLIAction, plugin.CLIText:
		default:
			t.Errorf("%s: unknown interaction kind %q", path, cmd.Kind)
		}
		if cmd.Short == "" {
			t.Errorf("%s: no short description", path)
		}
		if cmd.Needs == "" {
			t.Errorf("%s: no permission", path)
		}
	}
}

// TestCLIPathsAreUnique guards the uniqueness the host checks before its unique
// index does, and the count §6.1 of the contract spec asks for.
func TestCLIPathsAreUnique(t *testing.T) {
	reg := declaredCLI(t)
	seen := map[string]bool{}
	for _, cmd := range reg.commands {
		path := strings.Join(cmd.Path, " ")
		if seen[path] {
			t.Errorf("duplicate command path %q", path)
		}
		seen[path] = true
	}
	// 69, not the 68 of spec section 6.1: disk inspect and disk repair split the
	// one mixed-risk command the spec had into a safe read and a disruptive write.
	// 73 since vm migrate, vm migration and vm migration-cancel (WS5 T5.5/T5.6).
	// 75 since reboot-drain list and stop (T2.7 step 5).
	// 76 since nic set (T5.17).
	// 82 since display-profile list|get|create|update|duplicate|delete (T5.19).
	// 83 since vm stats-history.
	if len(reg.commands) != 83 {
		t.Errorf("declared %d commands, want 83", len(reg.commands))
	}
}

// TestCLIOptionalBodyFlagsDoNotOverrideTheDataFile guards the one thing that
// makes a BodyBoth command safe.
//
// The CLI sends a flag when it was passed *or* when it declares a default, so an
// optional int or bool carrying one is sent on every invocation and silently
// overwrites the field --data supplied. Only an empty default means "contribute
// nothing unless asked".
func TestCLIOptionalBodyFlagsDoNotOverrideTheDataFile(t *testing.T) {
	reg := declaredCLI(t)
	for _, cmd := range reg.commands {
		if cmd.Body != plugin.BodyBoth {
			continue
		}
		for _, f := range cmd.Flags {
			if f.Target != plugin.TargetBody || f.Required {
				continue
			}
			switch f.Type {
			case plugin.FlagInt, plugin.FlagBool, plugin.FlagDuration:
				if f.Default != "" {
					t.Errorf("%s: optional flag --%s has default %q, so it would overwrite %q from --data on every run",
						strings.Join(cmd.Path, " "), f.Name, f.Default, f.Field)
				}
			}
		}
	}
}

// TestCLIFlagsAreUsable checks the two ways a flag declaration goes wrong
// silently: a name the CLI reserves for a persistent flag, which it skips, and a
// path flag or positional argument that fills no placeholder.
func TestCLIFlagsAreUsable(t *testing.T) {
	reserved := map[string]bool{
		"context": true, "url": true, "env": true, "ca-file": true, "insecure": true,
		"output": true, "json": true, "yaml": true, "yes": true, "confirm": true,
		"timeout": true, "help": true, "data": true, "output-file": true, "force": true,
		"console": true,
	}
	reg := declaredCLI(t)
	for _, cmd := range reg.commands {
		path := strings.Join(cmd.Path, " ")
		names := map[string]bool{}
		for _, f := range cmd.Flags {
			if reserved[f.Name] {
				t.Errorf("%s: flag --%s collides with a persistent CLI flag and would be skipped",
					path, f.Name)
			}
			if names[f.Name] {
				t.Errorf("%s: flag --%s declared twice", path, f.Name)
			}
			names[f.Name] = true
			if f.Field == "" {
				t.Errorf("%s: flag --%s targets no field", path, f.Name)
			}
			if f.Target == plugin.TargetBody && cmd.Body != plugin.BodyFlags && cmd.Body != plugin.BodyBoth &&
				cmd.Kind != plugin.CLIUpload {
				t.Errorf("%s: flag --%s targets the body but the command sends %q",
					path, f.Name, cmd.Body)
			}
		}
		if cmd.Kind == plugin.CLIAttach {
			continue
		}
		filled := cmd.Route
		for _, arg := range cmd.Args {
			if !strings.Contains(filled, "{"+arg+"}") {
				t.Errorf("%s: argument <%s> fills no placeholder in %s", path, arg, cmd.Route)
			}
			filled = strings.ReplaceAll(filled, "{"+arg+"}", arg)
		}
		if strings.Contains(filled, "{") {
			t.Errorf("%s: route %s still has an unfilled placeholder", path, cmd.Route)
		}
	}
}

// TestCLIReachesEveryDisplayProfileRoute keeps the display profile routes and
// the CLI together: a route added to the manifest with no command fails here.
func TestCLIReachesEveryDisplayProfileRoute(t *testing.T) {
	reg := declaredCLI(t)
	commands := map[string]bool{}
	for _, cmd := range reg.commands {
		commands[strings.ToUpper(cmd.Method)+" "+cmd.Route] = true
	}
	found := 0
	for key := range readManifestRoutes(t) {
		if !strings.Contains(key, " /api/vms/display-profiles") {
			continue
		}
		found++
		if !commands[key] {
			t.Errorf("route %s has no kvm CLI command", key)
		}
	}
	if found != 6 {
		t.Errorf("plugin.yaml declares %d display profile routes, want 6", found)
	}
}
