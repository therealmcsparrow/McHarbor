// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versionorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type caseTable struct {
	Compare []struct {
		A    string `yaml:"a"`
		B    string `yaml:"b"`
		Want int    `yaml:"want"`
	} `yaml:"compare"`
	Parse []struct {
		Input string `yaml:"input"`
		Valid bool   `yaml:"valid"`
	} `yaml:"parse"`
	Prerelease []struct {
		Input string `yaml:"input"`
		Want  bool   `yaml:"want"`
	} `yaml:"prerelease"`
}

func loadCases(t *testing.T) caseTable {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "versionorder", "cases.yaml"))
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	var tbl caseTable
	if err := yaml.Unmarshal(raw, &tbl); err != nil {
		t.Fatalf("parse cases: %v", err)
	}
	if len(tbl.Compare) == 0 || len(tbl.Parse) == 0 || len(tbl.Prerelease) == 0 {
		t.Fatal("case table has an empty section")
	}
	return tbl
}

// TestSharedOrder runs the contract table that the CLI and the frontend also run.
func TestSharedOrder(t *testing.T) {
	tbl := loadCases(t)
	for _, c := range tbl.Compare {
		if got := Compare(c.A, c.B); got != c.Want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.A, c.B, got, c.Want)
		}
		if got := Compare(c.B, c.A); got != -c.Want {
			t.Errorf("Compare(%q, %q) = %d, want %d (antisymmetry)", c.B, c.A, got, -c.Want)
		}
	}
	for _, c := range tbl.Parse {
		_, err := Parse(c.Input)
		if (err == nil) != c.Valid {
			t.Errorf("Parse(%q) err = %v, valid want %v", c.Input, err, c.Valid)
		}
	}
	for _, c := range tbl.Prerelease {
		if got := IsPrerelease(c.Input); got != c.Want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", c.Input, got, c.Want)
		}
	}
}

func TestHostileInputBounded(t *testing.T) {
	huge := strings.Repeat("9", 1<<20)
	long := "1.0.0-" + strings.Repeat("a.", 1<<17) + "a"
	for _, in := range []string{huge, huge + ".0.0", long, strings.Repeat("-", 5000), "\x00", "1.0.0-\x00"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse of hostile input (len %d) succeeded", len(in))
		}
		if got := Compare(in, "0.0.0"); got != 0 {
			t.Errorf("Compare of unparsable input (len %d) = %d, want 0", len(in), got)
		}
		if IsPrerelease(in) {
			t.Errorf("IsPrerelease of unparsable input (len %d) = true", len(in))
		}
	}
}

func TestLeadingZeroNumericEqualsStripped(t *testing.T) {
	if Compare("1.0.0-rc.007", "1.0.0-rc.7") != 0 {
		t.Error("numeric prerelease identifiers with leading zeros must compare numerically")
	}
}

func TestTransitiveOrder(t *testing.T) {
	ordered := []string{"2026.11.01", "2026.11.02-rc.1", "2026.11.02-rc.2", "2026.11.02-rc.10", "2026.11.02-rc.x", "2026.11.02"}
	for i := range ordered {
		for j := range ordered {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(ordered[i], ordered[j]); got != want {
				t.Errorf("Compare(%q, %q) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}
