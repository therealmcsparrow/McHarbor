// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// writeSpec puts a spec in the cache for the context `run` resolves to, so a
// test exercises the same path a real invocation does.
func writeSpec(t *testing.T, dir string, spec pluginSpec) {
	t.Helper()
	path := filepath.Join(dir, "cli-spec", "default.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	spec.SpecVersion = pluginSpecVersion
	spec.FetchedAt = time.Now()
	raw, err := json.MarshalIndent(cachedSpec{Spec: spec, ETag: `"e"`, FetchedAt: spec.FetchedAt}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runWithSpec runs the CLI with a cached spec in place, returning the exit code
// and the two streams.
func runWithSpec(t *testing.T, f *fakeServer, spec pluginSpec, args ...string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	writeSpec(t, dir, spec)
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)
	t.Setenv("MCHARBOR_NO_KEYRING", "1")
	t.Setenv("MCHARBOR_CONTEXT", "")
	t.Setenv("MCHARBOR_ENV", "")
	if f != nil {
		t.Setenv("MCHARBOR_URL", f.URL)
	}
	t.Setenv("MCHARBOR_API_KEY", goodKey)
	var out, errOut bytes.Buffer
	code := Execute(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func kvmSpec() pluginSpec {
	return pluginSpec{
		Build: "202609270300",
		Roots: []pluginRoot{{Slug: "addon-kvm", Name: "kvm", Short: "Virtual machines", Aliases: []string{"vms"}}},
		Commands: []pluginCLICommand{
			{
				Slug: "addon-kvm", Path: []string{"vm", "list"}, Kind: "list",
				Method: http.MethodGet, Route: "/api/vms", Needs: "vm.view", Risk: "safe",
				Columns: []pluginColumn{{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}},
			},
			{
				Slug: "addon-kvm", Path: []string{"vm", "delete"}, Kind: "action",
				Method: http.MethodDelete, Route: "/api/vms/{id}", Needs: "vm.delete",
				Risk: "irreversible", Args: []string{"id"},
			},
			{
				Slug: "addon-kvm", Path: []string{"vm", "stop"}, Kind: "action",
				Method: http.MethodPost, Route: "/api/vms/{id}/stop", Needs: "vm.manage",
				Risk: "disruptive", Args: []string{"id"},
			},
		},
	}
}

// TestPluginRootsCannotShadowACoreCommand: the CLI's own tree is the authority
// on what its core commands are called, and it may be newer than the server —
// so the check is enforced here as well as server-side.
func TestPluginRootsCannotShadowACoreCommand(t *testing.T) {
	dir := t.TempDir()
	spec := kvmSpec()
	spec.Roots = []pluginRoot{
		{Slug: "addon-evil", Name: "container", Short: "not the real one"},
		{Slug: "addon-kvm", Name: "kvm", Short: "Virtual machines", Aliases: []string{"audit"}},
	}
	spec.Commands = append(spec.Commands, pluginCLICommand{
		Slug: "addon-evil", Path: []string{"list"}, Kind: "list", Method: http.MethodGet,
		Route: "/api/evil", Columns: []pluginColumn{{Header: "X", Path: "x"}},
	})
	writeSpec(t, dir, spec)
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)

	root, g := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	addPluginCommands(root, g)

	for _, c := range root.Commands() {
		if c.Name() == "container" && c.Short == "not the real one" {
			t.Fatal("a plugin root shadowed the core container command")
		}
		if c.Name() == "kvm" {
			for _, alias := range c.Aliases {
				if alias == "audit" {
					t.Fatal("a plugin alias shadowed the core audit command")
				}
			}
		}
	}
}

// TestPluginTreeIsBuiltFromTheCacheWithNoNetwork: the tree is built on every
// invocation including --help, so a request here would make the CLI unusable
// offline and slow when it was not.
func TestPluginTreeIsBuiltFromTheCacheWithNoNetwork(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, kvmSpec())
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)
	// No MCHARBOR_URL and no key: if building the tree needed either, this fails.
	t.Setenv("MCHARBOR_URL", "")
	t.Setenv("MCHARBOR_API_KEY", "")

	root, g := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	addPluginCommands(root, g)

	kvm := findTopLevel(root, "kvm")
	if kvm == nil {
		t.Fatal("the kvm root was not built from the cache")
	}
	vm := findTopLevel(kvm, "vm")
	if vm == nil {
		t.Fatal("the intermediate vm group was not created")
	}
	for _, want := range []string{"list", "delete", "stop"} {
		if findTopLevel(vm, want) == nil {
			t.Errorf("vm %s is missing", want)
		}
	}
	// The usage line must show the positional argument, or the operator has to
	// guess it.
	if got := findTopLevel(vm, "delete").Use; !strings.Contains(got, "<id>") {
		t.Errorf("the usage line must name the argument, got %q", got)
	}
	// The long help must name the permission, which is the first thing asked
	// about when a command is refused.
	if got := findTopLevel(vm, "delete").Long; !strings.Contains(got, "vm.delete") || !strings.Contains(got, "addon-kvm") {
		t.Errorf("the long help must name the key and the plugin, got %q", got)
	}
}

// TestPluginCommandsAreSkippedForAnUnknownKind: a plugin built against a newer
// host must not make an older CLI unusable.
func TestPluginCommandsAreSkippedForAnUnknownKind(t *testing.T) {
	dir := t.TempDir()
	spec := kvmSpec()
	spec.Commands = append(spec.Commands, pluginCLICommand{
		Slug: "addon-kvm", Path: []string{"vm", "telepathy"}, Kind: "telepathy",
		Method: http.MethodGet, Route: "/api/vms/mind",
	})
	writeSpec(t, dir, spec)
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)

	root, g := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	addPluginCommands(root, g)
	vm := findTopLevel(findTopLevel(root, "kvm"), "vm")
	if vm == nil {
		t.Fatal("the known commands must still be built")
	}
	if findTopLevel(vm, "telepathy") != nil {
		t.Fatal("an unknown kind must be skipped, not built")
	}
	if got := skippedPluginKinds(loadPluginSpec("default")); got["addon-kvm"] != 1 {
		t.Fatalf("doctor must be able to report the skipped command, got %v", got)
	}
}

// TestPluginIrreversibleCommandRefusesYesAlone is the guarantee the CLI
// documents: a destructive plugin command is guarded exactly as a core one is,
// and a refused confirmation sends no request at all.
func TestPluginIrreversibleCommandRefusesYesAlone(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"DELETE /api/vms/vm1":    func(w http.ResponseWriter, _ string) { ok(w, nil) },
		"POST /api/vms/vm1/stop": func(w http.ResponseWriter, _ string) { ok(w, nil) },
	})

	code, _, stderr := runWithSpec(t, f, kvmSpec(), "--yes", "kvm", "vm", "delete", "vm1")
	if code != ExitUsage || !strings.Contains(stderr, "--confirm vm1") {
		t.Fatalf("--yes alone must be refused and name --confirm: exit %d, %q", code, stderr)
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodDelete {
			t.Fatalf("a refused confirmation sent a request: %s %s", r.Method, r.URL.Path)
		}
	}

	if code, _, stderr := runWithSpec(t, f, kvmSpec(), "--confirm", "vm1", "kvm", "vm", "delete", "vm1"); code != ExitOK {
		t.Fatalf("--confirm must allow it: exit %d, %q", code, stderr)
	}
	// A disruptive command is satisfied by --yes, which is the other half of
	// the rule.
	if code, _, stderr := runWithSpec(t, f, kvmSpec(), "--yes", "kvm", "vm", "stop", "vm1"); code != ExitOK {
		t.Fatalf("--yes must satisfy a disruptive command: exit %d, %q", code, stderr)
	}
}

// TestPluginListRendersItsDeclaredColumns proves the declaration reaches the
// renderer, and that the route's placeholders are filled from the positionals.
func TestPluginListRendersItsDeclaredColumns(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/vms": func(w http.ResponseWriter, _ string) {
			ok(w, []map[string]any{{"id": "vm1", "name": "web", "secret": "hidden"}})
		},
	})
	code, out, stderr := runWithSpec(t, f, kvmSpec(), "kvm", "vm", "list")
	if code != ExitOK {
		t.Fatalf("exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "web") {
		t.Errorf("the declared columns must render: %q", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("a field with no declared column must not render: %q", out)
	}
}

// TestPluginSpecIsIgnoredAtAnUnknownVersion: a spec shape this binary does not
// understand must be ignored whole rather than half-read into a tree whose
// commands might mean something else.
func TestPluginSpecIsIgnoredAtAnUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	spec := kvmSpec()
	writeSpec(t, dir, spec)
	// Rewrite the version on disk, past the writer's normalisation.
	path := filepath.Join(dir, "cli-spec", "default.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw),
		`"specVersion": 1`, `"specVersion": 99`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)

	if loadPluginSpec("default") != nil {
		t.Fatal("a spec at an unknown version must be ignored")
	}
	root, g := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	addPluginCommands(root, g)
	if findTopLevel(root, "kvm") != nil {
		t.Fatal("no tree may be built from an unreadable spec")
	}
}

// TestPluginSurfaceCanBeDisabled: a CI job must be able to freeze the tree to
// what the binary itself ships.
func TestPluginSurfaceCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, kvmSpec())
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)
	t.Setenv("MCHARBOR_NO_PLUGIN_CLI", "1")

	if loadPluginSpec("default") != nil {
		t.Fatal("the cache must not be read when the surface is disabled")
	}
	root, g := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	addPluginCommands(root, g)
	if findTopLevel(root, "kvm") != nil {
		t.Fatal("no plugin root may be built when the surface is disabled")
	}
}

// TestContextNameCannotEscapeTheCacheDirectory: the context name is
// operator-chosen and ends up in a path.
func TestContextNameCannotEscapeTheCacheDirectory(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	path, err := specPath("../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("the cache path must not escape its directory: %q", path)
	}
	if !strings.HasSuffix(filepath.Dir(path), "cli-spec") {
		t.Fatalf("the cache must stay under cli-spec: %q", path)
	}
}

// TestSetJSONPathBuildsNestedBodies covers the flag assembly a plugin relies on
// to set a nested field without shipping a body template.
func TestSetJSONPathBuildsNestedBodies(t *testing.T) {
	root := map[string]any{}
	setJSONPath(root, "spec.memory.mb", 2048)
	setJSONPath(root, "spec.name", "web")
	setJSONPath(root, "top", true)

	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{`"mb":2048`, `"name":"web"`, `"top":true`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

// findTopLevel returns a command's direct child by name.
func findTopLevel(parent *cobra.Command, name string) *cobra.Command {
	if parent == nil {
		return nil
	}
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestPluginListFindsADifferentlyNamedArray: several routes call their array
// `imports` or `schedules` rather than `items`, and a list command over one of
// those printed an empty table.
func TestPluginListFindsADifferentlyNamedArray(t *testing.T) {
	spec := kvmSpec()
	spec.Commands = append(spec.Commands, pluginCLICommand{
		Slug: "addon-kvm", Path: []string{"import", "list"}, Kind: "list",
		Method: http.MethodGet, Route: "/api/vms/imports", Needs: "vm.import", Risk: "safe",
		Columns: []pluginColumn{{Header: "ID", Path: "id"}, {Header: "FILE", Path: "file"}},
	})
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/vms/imports": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"imports": []any{map[string]any{"id": "i1", "file": "disk.qcow2"}},
				"formats": "qcow2",
			})
		},
	})
	code, out, stderr := runWithSpec(t, f, spec, "kvm", "import", "list")
	if code != ExitOK {
		t.Fatalf("exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "disk.qcow2") {
		t.Fatalf("the sole array must be rendered, got %q", out)
	}

	// Two arrays is ambiguous, so nothing is guessed.
	if _, found := soleArray(map[string]any{"a": []any{1}, "b": []any{2}}); found {
		t.Error("two arrays must be ambiguous")
	}
	if _, found := soleArray(map[string]any{"a": []any{1}, "b": "x"}); !found {
		t.Error("one array beside a scalar must be found")
	}
}

// TestPluginUnsetFlagDoesNotOverrideTheDataFile: an unset int flag is
// indistinguishable from one set to zero by value, and on a BodyBoth command it
// would silently write 0 over the value the --data file supplied.
func TestPluginUnsetFlagDoesNotOverrideTheDataFile(t *testing.T) {
	spec := kvmSpec()
	spec.Commands = append(spec.Commands, pluginCLICommand{
		Slug: "addon-kvm", Path: []string{"vm", "update"}, Kind: "action",
		Method: http.MethodPut, Route: "/api/vms/{id}", Needs: "vm.manage",
		Risk: "disruptive", Args: []string{"id"}, Body: "both",
		Flags: []pluginFlag{
			{Name: "vcpus", Type: "int", Target: "body", Field: "vcpus"},
			{Name: "name", Type: "string", Target: "body", Field: "name"},
		},
	})
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"PUT /api/vms/vm1": func(w http.ResponseWriter, _ string) { ok(w, nil) },
	})

	dir := t.TempDir()
	data := filepath.Join(dir, "vm.json")
	if err := os.WriteFile(data, []byte(`{"vcpus":4,"name":"web"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runWithSpec(t, f, spec, "--yes", "kvm", "vm", "update", "vm1", "--data", data)
	if code != ExitOK {
		t.Fatalf("exit %d, %q", code, stderr)
	}
	body := f.bodies["PUT /api/vms/vm1"]
	if !strings.Contains(body, `"vcpus":4`) {
		t.Fatalf("an unset --vcpus overwrote the file's value: %s", body)
	}

	// A flag that *is* set still wins over the file.
	code, _, stderr = runWithSpec(t, f, spec, "--yes", "kvm", "vm", "update", "vm1", "--data", data, "--vcpus", "8")
	if code != ExitOK {
		t.Fatalf("exit %d, %q", code, stderr)
	}
	if body := f.bodies["PUT /api/vms/vm1"]; !strings.Contains(body, `"vcpus":8`) {
		t.Fatalf("a set --vcpus must win over the file: %s", body)
	}
}

// TestPluginListPrefersTheArrayItsPathNames: a response with two arrays cannot
// be guessed from its shape, and the command's own path names the one the
// operator asked for. Without this, `snapshot schedule list` rendered nothing.
func TestPluginListPrefersTheArrayItsPathNames(t *testing.T) {
	spec := kvmSpec()
	spec.Commands = append(spec.Commands, pluginCLICommand{
		Slug: "addon-kvm", Path: []string{"snapshot", "schedule", "list"}, Kind: "list",
		Method: http.MethodGet, Route: "/api/vms/{id}/snapshot-schedules",
		Needs: "vm.view", Risk: "safe", Args: []string{"id"},
		Columns: []pluginColumn{{Header: "ID", Path: "id"}, {Header: "CRON", Path: "cron"}},
	})
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/vms/vm1/snapshot-schedules": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"schedules": []any{map[string]any{"id": "s1", "cron": "0 3 * * *"}},
				"runs":      []any{map[string]any{"id": "r1", "cron": "never"}},
			})
		},
	})
	code, out, stderr := runWithSpec(t, f, spec, "kvm", "snapshot", "schedule", "list", "vm1")
	if code != ExitOK {
		t.Fatalf("exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "0 3 * * *") {
		t.Fatalf("the schedules array must render: %q", out)
	}
	if strings.Contains(out, "never") {
		t.Fatalf("the runs array must not be rendered in its place: %q", out)
	}

	// The hint derivation, directly.
	e := &pluginExecutor{cmd: pluginCLICommand{Path: []string{"snapshot", "schedule", "list"}}}
	hints := e.rowKeyHints()
	if len(hints) < 2 || hints[0] != "schedules" || hints[1] != "schedule" {
		t.Errorf("hints for a schedule list: %v", hints)
	}
	// items still wins, so a paginated envelope is unaffected.
	rows := listItemsPreferring(map[string]any{
		"items":     []any{map[string]any{"id": "right"}},
		"schedules": []any{map[string]any{"id": "wrong"}},
	}, "schedules")
	if len(rows) != 1 || rows[0]["id"] != "right" {
		t.Errorf("items must still take precedence, got %v", rows)
	}
}
