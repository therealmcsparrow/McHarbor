// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"os"
	"path/filepath"
)

// demoModeFile is the marker that offers demo mode in Settings. It sits in
// McHarbor's root beside VERSION; the image copies it there when the build
// context has one, so a release built without it never shows the section.
const demoModeFile = "demo_mode"

// demoModeAvailable reports whether the marker is present.
func demoModeAvailable() bool {
	return rootFileExists(demoModeFile, rootCandidates())
}

// rootCandidates lists McHarbor's root the way core/version finds VERSION:
// walking up from the working directory (a checkout run with `go run`), then
// beside the binary (the image's /app). It is a copy rather than a call so that
// no package plugins link against changes.
func rootCandidates() []string {
	dirs := []string{".", "..", filepath.Join("..", ".."), filepath.Join("..", "..", "..")}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}

// rootFileExists reports whether name is a regular file in any of dirs.
func rootFileExists(name string, dirs []string) bool {
	for _, dir := range dirs {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}
