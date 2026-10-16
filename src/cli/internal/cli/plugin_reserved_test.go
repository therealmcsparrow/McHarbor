// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

// hostReservedCLINames mirrors core/pluginruntime/cli_reserved.go.
//
// It is duplicated rather than imported because the CLI is a separate Go module
// and importing the backend would pull its whole dependency set into this
// binary. The test below is what keeps the copy honest: a core command added
// without updating the host list would let a plugin claim its name.
var hostReservedCLINames = []string{
	"agent", "ai", "api-key", "api-keys", "app", "apps",
	"audit", "backup", "backups", "capacity", "catalog", "catalogs", "cluster",
	"compliance", "completion", "container", "containers", "context", "ctr", "docs",
	"doctor", "elevation", "elevations", "env", "environment", "gitops",
	"group", "groups", "help", "host", "identity", "image", "images",
	"login", "logout", "network", "networks", "plugin", "plugins",
	"policies", "policy", "runbooks", "runbook", "restore", "role", "roles", "scim",
	"settings", "stack", "stacks", "status", "support", "tui", "ui",
	"user", "users", "version", "volume", "volumes", "whoami", "widgets",
}

// TestEveryCoreCommandIsReservedAgainstPlugins: a core command the host does not
// know about is a name a plugin can claim, and then `mcharbor <name>` quietly
// means something else.
func TestEveryCoreCommandIsReservedAgainstPlugins(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_PLUGIN_CLI", "1")
	root, _ := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})

	reserved := map[string]bool{}
	for _, name := range hostReservedCLINames {
		reserved[name] = true
	}

	var missing []string
	for _, c := range root.Commands() {
		for _, name := range append([]string{c.Name()}, c.Aliases...) {
			if !reserved[name] {
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("these core command names are not reserved against plugin roots — add them to "+
			"core/pluginruntime/cli_reserved.go and to hostReservedCLINames here: %s",
			strings.Join(missing, ", "))
	}
}

// TestReservedNamesAreNotAlreadyPluginRoots keeps the list from drifting the
// other way: a name reserved here but absent from the tree is dead weight that
// blocks a plugin for no reason.
func TestReservedNamesAreNotAlreadyPluginRoots(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_PLUGIN_CLI", "1")
	root, _ := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})

	present := map[string]bool{"help": true, "completion": true}
	for _, c := range root.Commands() {
		present[c.Name()] = true
		for _, a := range c.Aliases {
			present[a] = true
		}
	}
	var stale []string
	for _, name := range hostReservedCLINames {
		if !present[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("these names are reserved but are not core commands, so they block a plugin "+
			"for no reason: %s", strings.Join(stale, ", "))
	}
}
