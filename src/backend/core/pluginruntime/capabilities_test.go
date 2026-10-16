// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const capsTestSlug = "addon-capstest"

// noopRouterDeps satisfies RouterDeps for a test that only unloads; any other call
// panics on the nil embedded interface, which would be a test bug worth seeing.
type noopRouterDeps struct{ RouterDeps }

func (noopRouterDeps) SetPluginRoutes(string, []router.PluginRoute) {}

func writeCapsManifest(t *testing.T, dir, domain string) {
	t.Helper()
	body := "slug: " + capsTestSlug + "\ncontributes:\n  capabilities:\n    - kind: runbook-provider\n      domain: " + domain + "\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLoadedPluginCapabilitiesFollowItsLifecycle: a load publishes the manifest's
// capabilities, a bad declaration withdraws them without failing, and UnloadOne drops them.
func TestLoadedPluginCapabilitiesFollowItsLifecycle(t *testing.T) {
	t.Cleanup(func() { plugincaps.Default.Remove(capsTestSlug) })
	r := &Runtime{
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		adapters:      newAdapterRegistry(),
		loaded:        map[string]*Loaded{},
		manifests:     map[string]plugin.Manifest{},
		nodeExecutors: map[string]plugin.NodeExecFunc{},
		deps:          noopRouterDeps{},
	}
	dir := t.TempDir()

	writeCapsManifest(t, dir, "capstest")
	r.recordCapabilities(capsTestSlug, dir)
	if c, ok := plugincaps.Default.Provider(plugincaps.KindRunbookProvider, "capstest"); !ok || c.Plugin != capsTestSlug {
		t.Fatalf("a loaded plugin's capability must be published, got %+v %v", c, ok)
	}

	writeCapsManifest(t, dir, "Not A Token")
	r.recordCapabilities(capsTestSlug, dir)
	if _, ok := plugincaps.Default.Provider(plugincaps.KindRunbookProvider, "capstest"); ok {
		t.Fatal("a refused declaration must withdraw the plugin's earlier capabilities")
	}

	writeCapsManifest(t, dir, "capstest")
	r.recordCapabilities(capsTestSlug, dir)
	if err := r.UnloadOne(context.Background(), capsTestSlug); err != nil {
		t.Fatal(err)
	}
	if _, ok := plugincaps.Default.Provider(plugincaps.KindRunbookProvider, "capstest"); ok {
		t.Fatal("an unloaded plugin must leave no capability behind")
	}
}
