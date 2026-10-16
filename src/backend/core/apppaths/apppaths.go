// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package apppaths resolves the filesystem locations that differ between a
// container install and a native one, so no caller has to guess.
package apppaths

import (
	"os"
	"strings"
)

// DefaultDataDir is where the container has always kept its data, and stays
// the fallback so an unset DATA_DIR behaves exactly as it did before.
const DefaultDataDir = "/app/data"

// DataDir returns the data directory, preferring DATA_DIR. It exists because
// six call sites carried their own copy of this fallback and only agreed by
// coincidence.
func DataDir() string {
	if dir := strings.TrimSpace(os.Getenv("DATA_DIR")); dir != "" {
		return dir
	}
	return DefaultDataDir
}
