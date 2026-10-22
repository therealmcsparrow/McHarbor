// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"strings"
	"testing"
)

// TestSeedRunsAfterTheMySQLTimeColumnsAreKnown guards the boot order: the alert
// seed writes RFC3339 timestamps, which MySQL refuses until its DATETIME columns are loaded.
func TestSeedRunsAfterTheMySQLTimeColumnsAreKnown(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	load := strings.Index(src, "sqldialect.LoadMySQLTimeColumns(")
	for _, call := range []string{"alertsService.SeedDefaultRules(", "bootstrap.WireHostFSThresholds("} {
		at := strings.Index(src, call)
		if load < 0 || at < 0 {
			t.Fatalf("expected both %q and LoadMySQLTimeColumns in main.go", call)
		}
		if at < load {
			t.Errorf("%s runs before sqldialect.LoadMySQLTimeColumns", call)
		}
	}
}

// TestMainWiresMergeInvalidation fails when main.go stops registering the merge
// hook, which would leave alert state and host-fs readings stale after a merge.
func TestMainWiresMergeInvalidation(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "bootstrap.WireMergeInvalidation(environments.SetMergedHook)") {
		t.Fatal("main.go must call bootstrap.WireMergeInvalidation(environments.SetMergedHook)")
	}
}
