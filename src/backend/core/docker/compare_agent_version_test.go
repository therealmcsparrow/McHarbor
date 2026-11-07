// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCompareAgentVersionAgreesWithTheSharedOrder holds the local comparator copy
// to the shared cases, so a prerelease agent is never ordered differently here
// than by core/versionorder, build metadata and oversized components included.
func TestCompareAgentVersionAgreesWithTheSharedOrder(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "versionorder", "cases.yaml"))
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	var tbl struct {
		Compare []struct {
			A    string `yaml:"a"`
			B    string `yaml:"b"`
			Want int    `yaml:"want"`
		} `yaml:"compare"`
	}
	if err := yaml.Unmarshal(raw, &tbl); err != nil {
		t.Fatalf("parse cases: %v", err)
	}
	if len(tbl.Compare) == 0 {
		t.Fatal("case table has no compare rows")
	}
	for _, c := range tbl.Compare {
		if got := compareAgentVersion(c.A, c.B); got != c.Want {
			t.Errorf("compareAgentVersion(%q, %q) = %d, want %d", c.A, c.B, got, c.Want)
		}
		if got := compareAgentVersion(c.B, c.A); got != -c.Want {
			t.Errorf("compareAgentVersion(%q, %q) = %d, want %d (antisymmetry)", c.B, c.A, got, -c.Want)
		}
	}
}

func TestAgentAtLeastTreatsAPrereleaseAsOlderThanItsRelease(t *testing.T) {
	if compareAgentVersion("2026.11.02-rc.1", "2026.11.02") >= 0 {
		t.Fatal("a prerelease must order before its release")
	}
	if compareAgentVersion("2026.11.02-rc.1", "3.8.0") <= 0 {
		t.Fatal("a calendar prerelease must clear a historical floor")
	}
}

func TestCompareAgentVersionIgnoresBuildMetadata(t *testing.T) {
	if got := compareAgentVersion("2026.11.02+build.5", "2026.11.02"); got != 0 {
		t.Fatalf("metadata must not change the order, got %d", got)
	}
	if got := compareAgentVersion("2026.11.02+build.5", "2026.11.01"); got <= 0 {
		t.Fatalf("a newer release with metadata must order above an older one, got %d", got)
	}
	if got := compareAgentVersion("3.8", "3.8.0"); got != 0 {
		t.Fatalf("a short legacy version keeps its old reading, got %d", got)
	}
}
