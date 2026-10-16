// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import "testing"

// TestRepairVersionFollowsAReleaseUpgrade guards the first boot after a release
// that bumped the plugins with the host: the catalog carries only the new
// version, and insisting on the installed one disabled every plugin.
func TestRepairVersionFollowsAReleaseUpgrade(t *testing.T) {
	catalog := []PluginVersion{{Version: "2026.10.01", BuiltFor: "2026.10.01", DownloadURL: "file:///x.tgz"}}

	got, ok := repairVersion(catalog, "2026.09.01", "2026.10.01", true)
	if !ok || got.Version != "2026.10.01" {
		t.Fatalf("release upgrade: repairVersion = %+v, %v; want 2026.10.01", got, ok)
	}

	if _, ok := repairVersion(catalog, "2026.09.01", "2026.10.01", false); ok {
		t.Fatal("same release: a missing installed version must not silently move the plugin")
	}

	stale := []PluginVersion{{Version: "2026.10.01", BuiltFor: "2026.09.01"}}
	if _, ok := repairVersion(stale, "2026.09.01", "2026.10.01", true); ok {
		t.Fatal("release upgrade: an artifact built for another release must not be picked")
	}
}

// TestRepairVersionPrefersTheInstalledVersion keeps a repair restoring what the
// operator accepted whenever the catalog still has it.
func TestRepairVersionPrefersTheInstalledVersion(t *testing.T) {
	catalog := []PluginVersion{
		{Version: "2026.10.02", BuiltFor: "2026.10.01"},
		{Version: "2026.10.01", BuiltFor: "2026.10.01"},
	}
	got, ok := repairVersion(catalog, "2026.10.01", "2026.10.01", true)
	if !ok || got.Version != "2026.10.01" {
		t.Fatalf("repairVersion = %+v, %v; want the installed 2026.10.01", got, ok)
	}
}
