// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/therealmcsparrow/mcharbor-cli/run"
)

// cliArgs reports whether this process is the bundled CLI rather than the
// agent, and returns the CLI's arguments. It is the CLI when started through a
// link named mcharbor, or as `mcharbor-agent cli ...`.
func cliArgs(argv []string) ([]string, bool) {
	if len(argv) == 0 {
		return nil, false
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(argv[0])), ".exe")
	if name == "mcharbor" {
		return argv[1:], true
	}
	if len(argv) > 1 && argv[1] == "cli" {
		return argv[2:], true
	}
	return nil, false
}

// cliServerURL turns the agent's own MCHARBOR_URL, which may name the
// WebSocket endpoint, into the HTTP base URL the CLI expects under the same
// variable, so the CLI on an agent host targets that agent's McHarbor.
func cliServerURL(v string) string {
	switch {
	case strings.HasPrefix(v, "wss://"):
		v = "https://" + strings.TrimPrefix(v, "wss://")
	case strings.HasPrefix(v, "ws://"):
		v = "http://" + strings.TrimPrefix(v, "ws://")
	default:
		return v
	}
	return strings.TrimSuffix(strings.TrimRight(v, "/"), "/api/agent/ws")
}

// ensureCLILink makes the bundled CLI reachable as mcharbor beside exe. It runs
// after an update and at every native start, so a binary copied in by hand gets
// the link too; a mcharbor that is anything but this link is never touched.
func ensureCLILink(exe string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	link := filepath.Join(filepath.Dir(exe), "mcharbor")
	target := filepath.Base(exe)
	if _, err := os.Lstat(link); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("linking %s to %s: %w", link, target, err)
	}
	return nil
}

// runCLI runs the bundled CLI and exits with its status.
func runCLI(args []string) {
	if v := os.Getenv("MCHARBOR_URL"); v != "" {
		if err := os.Setenv("MCHARBOR_URL", cliServerURL(v)); err != nil {
			os.Exit(1)
		}
	}
	run.Stamp(agentVersion, agentBuild)
	os.Exit(run.Main(args, os.Stdout, os.Stderr))
}
