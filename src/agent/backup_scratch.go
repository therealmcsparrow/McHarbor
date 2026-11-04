// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"path/filepath"
	"strings"
)

// backupScratchRoot is where VM and container backups stage artifacts and restores download chains:
// under the agent's data directory, the same backups/tmp shape McHarbor uses for its own backup temp.
func backupScratchRoot(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = defaultAgentDataDir
	}
	return filepath.Join(dataDir, "backups", "tmp")
}
