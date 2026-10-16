// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	plugin "github.com/therealmcsparrow/mcharbor/core/plugin/contract"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	appstore "github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"

	_ "modernc.org/sqlite"
)

// newCLITestDB gives a migrated SQLite database, which is what proves migration
// 100 actually creates the two tables the spec is read out of.
func newCLITestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "cli.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// newCLITracker gives the appstore service used purely as the contribution
// tracker. The catalog resolver is nil because nothing here reads a catalog.
func newCLITracker(t *testing.T, database *sql.DB) *appstore.Service {
	t.Helper()
	return appstore.NewService(database, nil, t.TempDir())
}

// TestCLISpecRoundTrip is the contract's end-to-end assertion: a declared
// command set survives record → store → list, with its nested shapes intact.
//
// The nested fields travel as JSON text in the database, so a serialisation
// mistake there is invisible until an operator runs a command whose flags have
// silently vanished.
func TestCLISpecRoundTrip(t *testing.T) {
	database := newCLITestDB(t)
	svc := newCLITracker(t, database)
	ctx := context.Background()

	if err := svc.RecordCLIRoot(ctx, "addon-kvm", "kvm", "Virtual machines", `["vms"]`); err != nil {
		t.Fatalf("recording the root: %v", err)
	}
	row := pluginruntime.CLICommandRow{
		Path: "vm snapshot create", Kind: string(plugin.CLIAction),
		Method: "POST", Route: "/api/vms/{id}/snapshots",
		ArgsJSON:    `["id"]`,
		FlagsJSON:   `[{"Name":"memory-mb","Type":"int","Target":"body","Field":"spec.memoryMb","Required":false,"Default":"","Help":"guest memory"}]`,
		ColumnsJSON: `[{"Header":"NAME","Path":"name","Format":""}]`,
		Body:        string(plugin.BodyFlags), Needs: "vm.manage",
		Risk: string(plugin.RiskIrreversible), Short: "Create a snapshot",
		EnvScoped: true,
	}
	if err := svc.RecordCLICommand(ctx, "addon-kvm", row); err != nil {
		t.Fatalf("recording the command: %v", err)
	}

	spec, err := NewService(database, t.TempDir(), nil, nil).ListCLI(ctx)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if spec.SpecVersion != plugin.CLISpecVersion {
		t.Errorf("spec version = %d, want %d", spec.SpecVersion, plugin.CLISpecVersion)
	}
	if len(spec.Roots) != 1 || spec.Roots[0].Name != "kvm" {
		t.Fatalf("roots = %+v", spec.Roots)
	}
	if len(spec.Roots[0].Aliases) != 1 || spec.Roots[0].Aliases[0] != "vms" {
		t.Errorf("the aliases must survive as a list, got %v", spec.Roots[0].Aliases)
	}
	if len(spec.Commands) != 1 {
		t.Fatalf("commands = %+v", spec.Commands)
	}

	got := spec.Commands[0]
	// The path is stored space-joined and must come back as segments, or the
	// CLI hangs every command off the root instead of its group.
	if len(got.Path) != 3 || got.Path[0] != "vm" || got.Path[2] != "create" {
		t.Errorf("path = %v, want three segments", got.Path)
	}
	if got.Risk != string(plugin.RiskIrreversible) {
		t.Errorf("risk = %q; losing this turns a guarded delete into a footgun", got.Risk)
	}
	if !got.EnvScoped {
		t.Error("env scoping must survive the integer column")
	}
	if len(got.Args) != 1 || got.Args[0] != "id" {
		t.Errorf("args = %v", got.Args)
	}
	if len(got.Flags) != 1 {
		t.Fatalf("flags = %+v", got.Flags)
	}
	if got.Flags[0].Field != "spec.memoryMb" || got.Flags[0].Target != plugin.TargetBody {
		t.Errorf("the flag's target and dotted field must survive: %+v", got.Flags[0])
	}
	if len(got.Columns) != 1 || got.Columns[0].Header != "NAME" {
		t.Errorf("columns = %+v", got.Columns)
	}
}

// TestCLIRecordIsIdempotent: the loader records on every boot, so a restart must
// rewrite a command rather than accumulate duplicates of it.
func TestCLIRecordIsIdempotent(t *testing.T) {
	database := newCLITestDB(t)
	svc := newCLITracker(t, database)
	ctx := context.Background()

	row := pluginruntime.CLICommandRow{
		Path: "vm list", Kind: "list", Method: "GET", Route: "/api/vms",
		Needs: "vm.view", Risk: "safe",
		ColumnsJSON: `[{"Header":"ID","Path":"id"}]`,
	}
	for i := 0; i < 3; i++ {
		if err := svc.RecordCLIRoot(ctx, "addon-kvm", "kvm", "Virtual machines", `[]`); err != nil {
			t.Fatal(err)
		}
		if err := svc.RecordCLICommand(ctx, "addon-kvm", row); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := NewService(database, t.TempDir(), nil, nil).ListCLI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Roots) != 1 || len(spec.Commands) != 1 {
		t.Fatalf("three boots produced %d root(s) and %d command(s)", len(spec.Roots), len(spec.Commands))
	}

	// A rebuilt plugin that changes a command must overwrite it, not add a second.
	row.Needs = "vm.manage"
	row.Risk = "disruptive"
	if err := svc.RecordCLICommand(ctx, "addon-kvm", row); err != nil {
		t.Fatal(err)
	}
	spec, err = NewService(database, t.TempDir(), nil, nil).ListCLI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Commands) != 1 || spec.Commands[0].Needs != "vm.manage" || spec.Commands[0].Risk != "disruptive" {
		t.Fatalf("a changed command must replace the old row, got %+v", spec.Commands)
	}
}

// TestUninstallDropsTheCommandSet: an uninstalled plugin must leave no command
// rows, or its root stays in every operator's tree offering commands that 404.
func TestUninstallDropsTheCommandSet(t *testing.T) {
	database := newCLITestDB(t)
	svc := newCLITracker(t, database)
	ctx := context.Background()

	for _, slug := range []string{"addon-kvm", "addon-lxc"} {
		root := "kvm"
		if slug == "addon-lxc" {
			root = "lxc"
		}
		if err := svc.RecordCLIRoot(ctx, slug, root, root, `[]`); err != nil {
			t.Fatal(err)
		}
		if err := svc.RecordCLICommand(ctx, slug, pluginruntime.CLICommandRow{
			Path: "list", Kind: "list", Method: "GET", Route: "/api/" + root,
			Needs: root + ".view", Risk: "safe", ColumnsJSON: `[{"Header":"ID","Path":"id"}]`,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// ClearAll is what the uninstall path calls, so that is what is asserted
	// rather than DropCLI directly.
	if err := svc.ClearAll(ctx, "addon-lxc"); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}

	spec, err := NewService(database, t.TempDir(), nil, nil).ListCLI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range spec.Roots {
		if r.Slug == "addon-lxc" {
			t.Error("the uninstalled plugin's root survived")
		}
	}
	for _, c := range spec.Commands {
		if c.Slug == "addon-lxc" {
			t.Error("the uninstalled plugin's command survived")
		}
	}
	// The other plugin is untouched: an uninstall must not take the tree with it.
	if len(spec.Roots) != 1 || spec.Roots[0].Slug != "addon-kvm" || len(spec.Commands) != 1 {
		t.Fatalf("the surviving plugin's surface changed: %d root(s), %d command(s)",
			len(spec.Roots), len(spec.Commands))
	}

	// The root name is free again, so a reinstall under the same name works.
	if err := svc.RecordCLIRoot(ctx, "addon-lxc", "lxc", "lxc", `[]`); err != nil {
		t.Fatalf("reclaiming the root after an uninstall: %v", err)
	}
}

// TestCommandsWithoutARootAreNotServed: a command whose root was refused must
// not reach the tree, because it would be addressable at no path at all.
func TestCommandsWithoutARootAreNotServed(t *testing.T) {
	database := newCLITestDB(t)
	svc := newCLITracker(t, database)
	ctx := context.Background()

	if err := svc.RecordCLICommand(ctx, "addon-orphan", pluginruntime.CLICommandRow{
		Path: "list", Kind: "list", Method: "GET", Route: "/api/orphan",
		Needs: "orphan.view", Risk: "safe", ColumnsJSON: `[]`,
	}); err != nil {
		t.Fatal(err)
	}
	spec, err := NewService(database, t.TempDir(), nil, nil).ListCLI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Commands) != 0 {
		t.Fatalf("a command with no root was served: %+v", spec.Commands)
	}
}
