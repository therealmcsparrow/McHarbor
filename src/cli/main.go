// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Command mcharbor is the McHarbor command-line client.
package main

import (
	"os"

	"github.com/therealmcsparrow/mcharbor-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
