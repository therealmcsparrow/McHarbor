// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"path/filepath"
	"testing"
)

func TestBackupScratchRootLivesUnderTheAgentDataDir(t *testing.T) {
	cases := map[string]string{
		"/srv/agent-data": filepath.Join("/srv/agent-data", "backups", "tmp"),
		"":                filepath.Join(defaultAgentDataDir, "backups", "tmp"),
		"   ":             filepath.Join(defaultAgentDataDir, "backups", "tmp"),
	}
	for dataDir, want := range cases {
		if got := backupScratchRoot(dataDir); got != want {
			t.Errorf("backupScratchRoot(%q) = %q, want %q", dataDir, got, want)
		}
	}
}
