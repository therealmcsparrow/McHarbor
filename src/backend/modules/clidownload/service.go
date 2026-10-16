// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package clidownload serves the mcharbor command-line client archives this
// instance was built with, so an operator installs a client that matches it.
package clidownload

import (
	"bufio"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrNotFound means no archive exists for the requested platform.
var ErrNotFound = errors.New("clidownload: no archive for that platform")

// archivePattern matches what build-cli.sh writes, and bounds every part so a
// request can never name a file outside it.
var archivePattern = regexp.MustCompile(`^mcharbor_([0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.]+)?)_(linux|darwin|windows)_(amd64|arm64)\.(tar\.gz|zip)$`)

// minArchiveSize refuses a truncated or placeholder file; a real archive is
// several megabytes.
const minArchiveSize = 1 << 20

// Archive is one downloadable client build.
type Archive struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	File    string `json:"file"`
	Version string `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256,omitempty"`
	path    string
}

// Service finds archives in its directories, first match per platform winning,
// so a local rebuild shadows what the image shipped.
type Service struct {
	dirs []string
}

// NewService returns a Service over dirs in precedence order.
func NewService(dirs ...string) *Service {
	return &Service{dirs: dirs}
}

// List returns one archive per platform, read from disk on every call because
// the local folder is rebuilt while McHarbor runs.
func (s *Service) List() []Archive {
	seen := map[string]bool{}
	var out []Archive
	for _, dir := range s.dirs {
		sums := readChecksums(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			m := archivePattern.FindStringSubmatch(entry.Name())
			if m == nil || entry.IsDir() {
				continue
			}
			key := m[2] + "/" + m[3]
			if seen[key] {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.Size() < minArchiveSize {
				continue
			}
			seen[key] = true
			out = append(out, Archive{
				OS: m[2], Arch: m[3], File: entry.Name(), Version: m[1],
				Size: info.Size(), SHA256: sums[entry.Name()],
				path: filepath.Join(dir, entry.Name()),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OS != out[j].OS {
			return out[i].OS < out[j].OS
		}
		return out[i].Arch < out[j].Arch
	})
	return out
}

// Find returns the archive for one platform.
func (s *Service) Find(osName, arch string) (Archive, error) {
	for _, a := range s.List() {
		if a.OS == osName && a.Arch == arch {
			return a, nil
		}
	}
	return Archive{}, ErrNotFound
}

// Path is where an archive's bytes are on disk.
func (a Archive) Path() string { return a.path }

// readChecksums parses the `<sha256> [*]<file>` lines build-cli.sh writes.
func readChecksums(dir string) map[string]string {
	sums := map[string]string{}
	matches, err := filepath.Glob(filepath.Join(dir, "mcharbor_*_checksums.txt"))
	if err != nil {
		slog.Warn("clidownload: checksum glob failed", "error", err, "dir", dir)
	}
	for _, file := range matches {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 && len(fields[0]) == 64 {
				sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
			}
		}
		if err := f.Close(); err != nil {
			continue
		}
	}
	return sums
}
