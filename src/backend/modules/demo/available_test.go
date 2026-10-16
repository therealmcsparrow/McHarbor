// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRootFileExistsNeedsTheMarker guards the switch that hides Settings → Demo
// mode: absent marker, hidden; a directory of the same name does not count.
func TestRootFileExistsNeedsTheMarker(t *testing.T) {
	empty, withDir, withFile := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(withDir, demoModeFile), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withFile, demoModeFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if rootFileExists(demoModeFile, []string{empty}) {
		t.Error("no marker: want unavailable")
	}
	if rootFileExists(demoModeFile, []string{withDir}) {
		t.Error("a directory named demo_mode: want unavailable")
	}
	if !rootFileExists(demoModeFile, []string{empty, withFile}) {
		t.Error("marker in a later candidate: want available")
	}
}
