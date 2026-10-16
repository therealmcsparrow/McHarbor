// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package run is the CLI's public entry point, so a binary in another module,
// the remote agent, can carry the whole command tree without reaching into internal/.
package run

import (
	"io"

	"github.com/therealmcsparrow/mcharbor-cli/internal/cli"
)

// Main runs the command tree on args, without the program name, and returns
// the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	return cli.Execute(args, stdout, stderr)
}

// Stamp sets the version the CLI reports when its host binary was not linked
// with the CLI's own -X flags; core, agent and CLI share one VERSION.
func Stamp(version, build string) {
	if cli.Version == "dev" && version != "" {
		cli.Version = version
	}
	if cli.Build == "dev" && build != "" {
		cli.Build = build
	}
}
