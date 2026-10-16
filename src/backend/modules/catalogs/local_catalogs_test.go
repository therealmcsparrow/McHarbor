// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import "testing"

// TestLocalCatalogPathsDefaultToContainerLayout pins the container's paths, so
// making these configurable cannot silently move them for an existing install.
func TestLocalCatalogPathsDefaultToContainerLayout(t *testing.T) {
	paths := localCatalogPaths("", "")

	want := map[string]Kind{
		"/mnt/app-catalog":    KindDockerApp,
		"/mnt/themes-catalog": KindTheme,
		"/mnt/plugins":        KindPlugin,
		"/mnt/plugin-catalog": KindPlugin,
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for _, entry := range paths {
		kind, ok := want[entry.Path]
		if !ok {
			t.Errorf("unexpected path %q", entry.Path)
			continue
		}
		if entry.Kind != kind {
			t.Errorf("%s kind = %q, want %q", entry.Path, entry.Kind, kind)
		}
	}
}

// TestLocalCatalogPathsHonourOverrides covers the native install, where the
// plugin and theme catalogs live under /opt/mcharbor instead.
func TestLocalCatalogPathsHonourOverrides(t *testing.T) {
	paths := localCatalogPaths("/opt/mcharbor/ai-plugin-catalog", "/opt/mcharbor/themes-catalog")

	var foundPlugin, foundTheme bool
	for _, entry := range paths {
		if entry.Path == "/opt/mcharbor/ai-plugin-catalog" && entry.Kind == KindPlugin {
			foundPlugin = true
		}
		if entry.Path == "/opt/mcharbor/themes-catalog" && entry.Kind == KindTheme {
			foundTheme = true
		}
		if entry.Path == "/app/ai-plugin-catalog" || entry.Path == "/mnt/themes-catalog" {
			t.Errorf("override ignored: container path %q still present", entry.Path)
		}
	}
	if !foundPlugin {
		t.Error("overridden plugin catalog root missing")
	}
	if !foundTheme {
		t.Error("overridden theme catalog root missing")
	}
}

// TestLocalCatalogPathsKeepDevMounts guards the /mnt development mounts, which
// have no native equivalent and must survive an override untouched.
func TestLocalCatalogPathsKeepDevMounts(t *testing.T) {
	paths := localCatalogPaths("/opt/mcharbor/ai-plugin-catalog", "/opt/mcharbor/themes-catalog")

	for _, want := range []string{"/mnt/app-catalog", "/mnt/plugins", "/mnt/plugin-catalog"} {
		var found bool
		for _, entry := range paths {
			if entry.Path == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("development mount %q was dropped", want)
		}
	}
}

// TestSeededLocalMountFollowsOverriddenRoots defends against a native
// install's first-party plugin catalog being flagged as unvetted because
// the provenance check compared against the container's hardcoded roots
// instead of the ones actually configured for this install.
func TestSeededLocalMountFollowsOverriddenRoots(t *testing.T) {
	const pluginRoot = "/opt/mcharbor/ai-plugin-catalog"
	const themeRoot = "/opt/mcharbor/themes-catalog"

	if !isSeededLocalMount(pluginRoot, KindPlugin, pluginRoot, themeRoot) {
		t.Error("overridden plugin root not recognised as a seeded local mount")
	}
	if isSeededLocalMount("/app/ai-plugin-catalog", KindPlugin, pluginRoot, themeRoot) {
		t.Error("container default recognised as seeded while an override is in force")
	}
}
