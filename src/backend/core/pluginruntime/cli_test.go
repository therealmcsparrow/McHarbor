// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

// fakeCLITracker records what the registry writes, so a test can prove that a
// refused command set wrote nothing rather than only that it returned an error.
type fakeCLITracker struct {
	ContributionTracker
	roots    []string
	commands []CLICommandRow
	dropped  []string
}

func (f *fakeCLITracker) RecordCLIRoot(_ context.Context, origin, name, _, _ string) error {
	f.roots = append(f.roots, origin+"/"+name)
	return nil
}

func (f *fakeCLITracker) RecordCLICommand(_ context.Context, _ string, cmd CLICommandRow) error {
	f.commands = append(f.commands, cmd)
	return nil
}

func (f *fakeCLITracker) DropCLI(_ context.Context, origin string) error {
	f.dropped = append(f.dropped, origin)
	return nil
}

// cliTestRuntime builds a Runtime whose manifest declares one route, which is
// what the permission-agreement rule is checked against.
func cliTestRuntime(t *testing.T, tracker ContributionTracker) *Runtime {
	t.Helper()
	rt := &Runtime{
		manifests: map[string]plugin.Manifest{
			"addon-kvm": {
				Slug: "addon-kvm",
				Contribs: plugin.Contributes{
					Routes: []plugin.RouteDecl{
						{Method: http.MethodGet, Path: "/api/vms", Requires: "vm.view"},
						{Method: http.MethodPost, Path: "/api/vms/{id}/stop", Requires: "vm.manage"},
						// A route declared with no permission — the state the six
						// ungated routes were in until 2026-09-27.
						{Method: http.MethodGet, Path: "/api/vms/ungated"},
					},
				},
			},
		},
		tracker: tracker,
	}
	return rt
}

func listCommand() plugin.CLICommand {
	return plugin.CLICommand{
		Path: []string{"vm", "list"}, Kind: plugin.CLIList,
		Method: http.MethodGet, Route: "/api/vms", Needs: "vm.view",
		Columns: []plugin.CLIColumn{{Header: "NAME", Path: "name"}},
	}
}

// TestCLIRootNamesCannotShadowACoreCommand is the rule that keeps `mcharbor
// container` meaning what it says whatever is installed.
func TestCLIRootNamesCannotShadowACoreCommand(t *testing.T) {
	for _, name := range []string{"container", "audit", "plugin", "support", "help"} {
		tracker := &fakeCLITracker{}
		reg := &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
		err := reg.Root(plugin.CLIRoot{Name: name, Short: "x"})
		if err == nil {
			t.Errorf("%q must be refused as a root name", name)
		}
		if len(tracker.roots) != 0 {
			t.Errorf("%q was refused but still recorded: %v", name, tracker.roots)
		}
	}

	// An alias is just as reachable as a name, so it is checked too.
	tracker := &fakeCLITracker{}
	reg := &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm", Aliases: []string{"stack"}}); err == nil {
		t.Error("an alias that shadows a core command must be refused")
	}

	// And a legal one is accepted and recorded.
	tracker = &fakeCLITracker{}
	reg = &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm", Short: "Virtual machines", Aliases: []string{"vms"}}); err != nil {
		t.Fatalf("kvm must be accepted: %v", err)
	}
	if len(tracker.roots) != 1 || tracker.roots[0] != "addon-kvm/kvm" {
		t.Errorf("root not recorded: %v", tracker.roots)
	}
}

// TestCLIRootNameShapeIsEnforced: a root that needed shell quoting, or that
// could be mistaken for a flag, is not usable at all.
func TestCLIRootNameShapeIsEnforced(t *testing.T) {
	for _, name := range []string{"", "K", "-kvm", "kvm_vm", "kvm vm", "1kvm", strings.Repeat("k", 40)} {
		reg := &cliRegistry{runtime: cliTestRuntime(t, &fakeCLITracker{}), origin: "addon-kvm", paths: map[string]bool{}}
		if err := reg.Root(plugin.CLIRoot{Name: name}); err == nil {
			t.Errorf("%q must be refused", name)
		}
	}
}

// TestCLIRootCannotBeClaimedTwice: two plugins sharing a root would make one of
// them silently unreachable.
func TestCLIRootCannotBeClaimedTwice(t *testing.T) {
	rt := cliTestRuntime(t, &fakeCLITracker{})
	first := &cliRegistry{runtime: rt, origin: "addon-kvm", paths: map[string]bool{}}
	if err := first.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatal(err)
	}
	second := &cliRegistry{runtime: rt, origin: "addon-other", paths: map[string]bool{}}
	err := second.Root(plugin.CLIRoot{Name: "kvm"})
	if err == nil || !strings.Contains(err.Error(), "addon-kvm") {
		t.Fatalf("the second claim must be refused and name the owner, got %v", err)
	}
	// The same plugin reclaiming its own root on a reload is legal.
	again := &cliRegistry{runtime: rt, origin: "addon-kvm", paths: map[string]bool{}}
	if err := again.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatalf("a reload must be able to reclaim its own root: %v", err)
	}
}

// TestCLICommandPermissionMustEqualItsRoute is the rule that makes a declared
// command incapable of advertising a weaker key than the middleware enforces —
// and the reason the six ungated routes had to be fixed before this could work.
func TestCLICommandPermissionMustEqualItsRoute(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*plugin.CLICommand)
		wantErr string
	}{
		{"a weaker key is refused", func(c *plugin.CLICommand) { c.Needs = "" }, "enforces"},
		{"a different key is refused", func(c *plugin.CLICommand) { c.Needs = "vm.manage" }, "enforces"},
		{"a route the plugin never declared is refused",
			func(c *plugin.CLICommand) { c.Route = "/api/vms/invented" }, "does not declare"},
		{"a route declared with no permission cannot satisfy the rule",
			func(c *plugin.CLICommand) { c.Route = "/api/vms/ungated"; c.Needs = "vm.view" }, "enforces"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &fakeCLITracker{}
			reg := &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
			if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
				t.Fatal(err)
			}
			cmd := listCommand()
			tc.mutate(&cmd)
			err := reg.Add(cmd)
			if err == nil {
				t.Fatalf("must be refused")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("the reason must say %q, got %q", tc.wantErr, err)
			}
			if len(tracker.commands) != 0 {
				t.Errorf("a refused command was recorded: %v", tracker.commands)
			}
		})
	}

	// The agreeing case records.
	tracker := &fakeCLITracker{}
	reg := &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(listCommand()); err != nil {
		t.Fatalf("an agreeing command must be accepted: %v", err)
	}
	if len(tracker.commands) != 1 || tracker.commands[0].Needs != "vm.view" {
		t.Fatalf("command not recorded: %+v", tracker.commands)
	}
	if tracker.commands[0].Path != "vm list" {
		t.Errorf("path must be stored space-joined, got %q", tracker.commands[0].Path)
	}
}

// TestCLIKindCoherence covers the per-kind requirements whose absence produces a
// command that cannot work rather than one that fails loudly.
func TestCLIKindCoherence(t *testing.T) {
	base := func() plugin.CLICommand {
		return plugin.CLICommand{
			Path: []string{"x"}, Method: http.MethodGet, Route: "/api/vms", Needs: "vm.view",
		}
	}
	cases := []struct {
		name    string
		cmd     plugin.CLICommand
		wantErr string
	}{
		{"a list with no columns has nothing to render", func() plugin.CLICommand {
			c := base()
			c.Kind = plugin.CLIList
			return c
		}(), "at least one column"},
		{"an attach with no guest kind cannot build its ticket", func() plugin.CLICommand {
			c := base()
			c.Kind = plugin.CLIAttach
			return c
		}(), "guest kind"},
		{"an upload with no file flag has nothing to send", func() plugin.CLICommand {
			c := base()
			c.Kind = plugin.CLIUpload
			return c
		}(), "exactly one file flag"},
		{"an upload with two file flags is ambiguous", func() plugin.CLICommand {
			c := base()
			c.Kind = plugin.CLIUpload
			c.Flags = []plugin.CLIFlag{
				{Name: "a", Type: plugin.FlagFile}, {Name: "b", Type: plugin.FlagFile},
			}
			return c
		}(), "exactly one file flag"},
		{"a download renders no columns", func() plugin.CLICommand {
			c := base()
			c.Kind = plugin.CLIDownload
			c.Columns = []plugin.CLIColumn{{Header: "X", Path: "x"}}
			return c
		}(), "renders no columns"},
		{"an unknown kind is refused rather than guessed", func() plugin.CLICommand {
			c := base()
			c.Kind = "telepathy"
			return c
		}(), "unknown interaction kind"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &cliRegistry{runtime: cliTestRuntime(t, &fakeCLITracker{}), origin: "addon-kvm", paths: map[string]bool{}}
			if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
				t.Fatal(err)
			}
			err := reg.Add(tc.cmd)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want an error saying %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestCLIDuplicatePathAndMissingRootAreRefused covers the two structural rules.
func TestCLIDuplicatePathAndMissingRootAreRefused(t *testing.T) {
	reg := &cliRegistry{runtime: cliTestRuntime(t, &fakeCLITracker{}), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Add(listCommand()); err == nil || !strings.Contains(err.Error(), "Root must be called") {
		t.Fatalf("Add before Root must be refused, got %v", err)
	}

	reg = &cliRegistry{runtime: cliTestRuntime(t, &fakeCLITracker{}), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(listCommand()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(listCommand()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("a duplicate path must be refused, got %v", err)
	}

	reg = &cliRegistry{runtime: cliTestRuntime(t, &fakeCLITracker{}), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatal(err)
	}
	empty := listCommand()
	empty.Path = nil
	if err := reg.Add(empty); err == nil || !strings.Contains(err.Error(), "empty path") {
		t.Fatalf("an empty path must be refused, got %v", err)
	}
}

// TestCLIRegistryStopsAfterTheFirstRejection: a plugin that ignores the error
// from one Add must not go on to record a half a command set, because a partial
// tree is worse than none.
func TestCLIRegistryStopsAfterTheFirstRejection(t *testing.T) {
	tracker := &fakeCLITracker{}
	reg := &cliRegistry{runtime: cliTestRuntime(t, tracker), origin: "addon-kvm", paths: map[string]bool{}}
	if err := reg.Root(plugin.CLIRoot{Name: "kvm"}); err != nil {
		t.Fatal(err)
	}
	bad := listCommand()
	bad.Needs = "vm.manage"
	if err := reg.Add(bad); err != nil {
		t.Logf("bad command refused at Add: %v", err)
	}

	good := listCommand()
	good.Path = []string{"vm", "get"}
	good.Kind = plugin.CLIGet
	if err := reg.Add(good); err == nil {
		t.Fatal("an Add after a rejection must keep returning the rejection")
	}
	if len(tracker.commands) != 0 {
		t.Fatalf("nothing may be recorded after a rejection: %+v", tracker.commands)
	}
}

// TestCLIRowFlattensNestedShapesAsJSON: the CLI reads these back whole, so they
// have to round-trip.
func TestCLIRowFlattensNestedShapesAsJSON(t *testing.T) {
	cmd := plugin.CLICommand{
		Path: []string{"vm", "create"}, Kind: plugin.CLIAction,
		Method: http.MethodPost, Route: "/api/vms/{id}/stop", Needs: "vm.manage",
		Args:  []string{"id"},
		Flags: []plugin.CLIFlag{{Name: "memory-mb", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "memoryMb"}},
	}
	row, err := cliRow("vm create", cmd)
	if err != nil {
		t.Fatal(err)
	}
	// An unset body and risk must default rather than store an empty string a
	// reader would have to interpret.
	if row.Body != string(plugin.BodyNone) || row.Risk != string(plugin.RiskSafe) {
		t.Errorf("body/risk must default, got %q/%q", row.Body, row.Risk)
	}
	var args []string
	if err := json.Unmarshal([]byte(row.ArgsJSON), &args); err != nil || len(args) != 1 || args[0] != "id" {
		t.Errorf("args did not round-trip: %q (%v)", row.ArgsJSON, err)
	}
	var flags []plugin.CLIFlag
	if err := json.Unmarshal([]byte(row.FlagsJSON), &flags); err != nil || len(flags) != 1 ||
		flags[0].Field != "memoryMb" || flags[0].Target != plugin.TargetBody {
		t.Errorf("flags did not round-trip: %q (%v)", row.FlagsJSON, err)
	}
	// Columns are absent, and the column must still be valid JSON.
	if row.ColumnsJSON != "null" && row.ColumnsJSON != "[]" {
		t.Errorf("columns must serialise to valid JSON, got %q", row.ColumnsJSON)
	}
}

// TestReservedCLINamesIsSorted keeps the exported list stable for the CLI-side
// contract test that compares it against the real tree.
func TestReservedCLINamesIsSorted(t *testing.T) {
	names := ReservedCLINames()
	if len(names) < 30 {
		t.Fatalf("the reserved list looks truncated: %d entries", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("not sorted at %d: %q then %q", i, names[i-1], names[i])
		}
	}
}
