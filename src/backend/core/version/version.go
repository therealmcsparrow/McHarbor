// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const fallbackVersion = "dev"

var (
	current string
	once    sync.Once
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

func versionFileCandidates() []string {
	candidates := []string{}
	if path := strings.TrimSpace(os.Getenv("MCHARBOR_VERSION_FILE")); path != "" {
		candidates = append(candidates, path)
	}

	for _, path := range []string{
		"VERSION",
		filepath.Join("..", "VERSION"),
		filepath.Join("..", "..", "VERSION"),
		filepath.Join("..", "..", "..", "VERSION"),
		filepath.Join("..", "..", "..", "..", "VERSION"),
	} {
		candidates = append(candidates, path)
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "VERSION"))
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "VERSION")))
	}

	return candidates
}
