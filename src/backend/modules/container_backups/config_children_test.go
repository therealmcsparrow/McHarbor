// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"strings"
	"testing"
)

// TestEnumerateConfigChildrenAt_EnvLeaves exercises the lazy-load
// path the operator hits when they drill into the Environment folder
// of the per-item config picker. Without this the picker shows
// "This mount is empty." even though Env vars are present.
func TestEnumerateConfigChildrenAt_EnvLeaves(t *testing.T) {
	inspect := []byte(`{"Config":{"Env":["FOO=1","BAR=2","EMPTY=","=leadingeq","BAD"],"Cmd":["nginx"],"WorkingDir":"/app","User":"nginx","Image":"nginx:1.27","Labels":{"team":"ops"}}}`)
	for _, parentKey := range []string{"", "config", "config:Config.Env", "config:Config.Cmd", "config:Config.Labels"} {
		items, _, err := enumerateConfigChildrenAt(inspect, parentKey)
		if err != nil {
			t.Fatalf("enumerateConfigChildrenAt(%q) err: %v", parentKey, err)
		}
		t.Logf("parentKey=%q items=%d", parentKey, len(items))
		for _, it := range items {
			t.Logf("  - key=%q type=%q label=%q", it.Key, it.Type, it.Label)
		}
		switch parentKey {
		case "config:Config.Env":
			if len(items) != 2 {
				t.Errorf("Env parent expected 2 leaves (FOO, BAR), got %d", len(items))
			}
			labels := map[string]bool{}
			for _, it := range items {
				labels[it.Label] = true
			}
			for _, want := range []string{"FOO", "BAR"} {
				if !labels[want] {
					t.Errorf("Env parent missing leaf for %q", want)
				}
			}
		case "config:Config.Cmd":
			if len(items) != 1 {
				t.Errorf("Cmd parent expected 1 leaf, got %d", len(items))
			}
		case "config:Config.Labels":
			if len(items) != 1 {
				t.Errorf("Labels parent expected 1 leaf, got %d", len(items))
			}
		}
	}
}

// TestEnumerateConfigChildrenAt_RootReturnsFoldersAndScalars verifies
// that the root call (parentKey == "" or "config") emits BOTH the
// scalar leaves AND the folder rows. If this returns only folders the
// frontend misses the scalars (WorkingDir/User/Image) entirely.
func TestEnumerateConfigChildrenAt_RootReturnsFoldersAndScalars(t *testing.T) {
	inspect := []byte(`{"Config":{"Env":["FOO=1"],"Cmd":["nginx"],"WorkingDir":"/app","User":"nginx","Image":"nginx:1.27","Labels":{}}}`)
	items, _, err := enumerateConfigChildrenAt(inspect, "")
	if err != nil {
		t.Fatalf("enumerateConfigChildrenAt: %v", err)
	}
	if len(items) < 10 {
		t.Fatalf("root expected >=10 children (3 scalars + 7 folders), got %d", len(items))
	}
	var folders, files int
	for _, it := range items {
		switch it.Type {
		case "folder":
			folders++
		case "file":
			files++
		}
	}
	if folders != 7 {
		t.Errorf("root expected 7 folders, got %d", folders)
	}
	if files != 3 {
		t.Errorf("root expected 3 scalars, got %d", files)
	}
}

// TestEnumerateConfigChildrenAt_UnknownKeyErrors guards against a
// silently empty response — if the frontend sends an unexpected
// parentKey the backend must surface a 500 instead of returning []
// (which renders "This mount is empty" in the UI).
func TestEnumerateConfigChildrenAt_UnknownKeyErrors(t *testing.T) {
	inspect := []byte(`{"Config":{"Env":["FOO=1"]}}`)
	_, _, err := enumerateConfigChildrenAt(inspect, "config:Config.NotARealField")
	if err == nil {
		t.Fatalf("expected error for unknown parentKey, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config parent key") {
		t.Errorf("expected unknown-key error, got %v", err)
	}
}
