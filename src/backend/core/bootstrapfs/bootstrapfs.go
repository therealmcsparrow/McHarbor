// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package bootstrapfs performs the filesystem setup that used to live in
// docker-entrypoint.sh, so a native install gets it without a second copy of
// that script.
package bootstrapfs

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDirs creates the data directory and the TLS subdirectory certgen
// writes into, before anything that assumes they exist runs.
func EnsureDirs(dataDir string) error {
	for _, dir := range []string{dataDir, filepath.Join(dataDir, "tls")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}
