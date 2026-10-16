// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package version reports the running release and build stamp.
package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const fallbackVersion = "dev"

// fallbackBuild is what a build stamp reads with no BUILD file present, which
// is the ordinary case for a `go run` from a source tree.
const fallbackBuild = "dev"

var (
	current string
	once    sync.Once

	build     string
	buildOnce sync.Once
)

// Current returns the McHarbor application version from the canonical VERSION file.
func Current() string {
	once.Do(func() {
		current = load()
	})
	return current
}

func load() string {
	if value := strings.TrimSpace(os.Getenv("MCHARBOR_VERSION")); value != "" {
		return value
	}

	for _, path := range versionFileCandidates() {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if value := strings.TrimSpace(string(content)); value != "" {
			return value
		}
	}

	return fallbackVersion
}

// Build returns the build stamp from the canonical BUILD file.
//
// The build moves on every artifact while the version stands still, so this is
// what tells two artifacts of one release apart. Format is yyyyMMddHHmm.
func Build() string {
	buildOnce.Do(func() {
		build = loadBuild()
	})
	return build
}

func loadBuild() string {
	if value := strings.TrimSpace(os.Getenv("MCHARBOR_BUILD")); value != "" {
		return value
	}
	for _, path := range stampCandidates("BUILD") {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if value := strings.TrimSpace(string(content)); value != "" {
			return value
		}
	}
	return fallbackBuild
}

func versionFileCandidates() []string {
	candidates := []string{}
	if path := strings.TrimSpace(os.Getenv("MCHARBOR_VERSION_FILE")); path != "" {
		candidates = append(candidates, path)
	}

	return append(candidates, stampCandidates("VERSION")...)
}

// stampCandidates lists where a single-line stamp file may sit: walking up
// from the working directory, then beside the binary, then beside this source.
func stampCandidates(name string) []string {
	candidates := []string{
		name,
		filepath.Join("..", name),
		filepath.Join("..", "..", name),
		filepath.Join("..", "..", "..", name),
		filepath.Join("..", "..", "..", "..", name),
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(
			filepath.Join(filepath.Dir(file), "..", "..", "..", "..", name)))
	}

	return candidates
}
