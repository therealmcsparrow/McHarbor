// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

func TestFloorOf(t *testing.T) {
	for expr, want := range map[string]string{
		">=2026.11.01":   "2026.11.01",
		">= 2026.11.01":  "2026.11.01",
		"2026.11.01":     "2026.11.01",
		">=2.1.0 <3.0.0": "2.1.0",
		"^1.2.3":         "1.2.3",
		"<3.0.0":         "",
		"":               "",
	} {
		if got := floorOf(expr); got != want {
			t.Errorf("floorOf(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestReleaseAtLeast(t *testing.T) {
	for _, tc := range []struct {
		current, floor string
		want           bool
	}{
		{"2026.11.01", "2026.11.01", true},
		{"2026.12.01", "2026.11.01", true},
		{"2027.01.01", "2026.12.09", true},
		{"2026.11.01", "2026.11.02", false},
		{"2026.10.09", "2026.11.01", false},
		// Numeric, not lexical: this is why a string compare would be wrong.
		{"2026.10.01", "2026.9.01", true},
		{"2026.9.1", "2026.09.01", true},
		{"dev", "2026.11.01", true},
		{"2026.11.01", "unknown", true},
	} {
		if got := releaseAtLeast(tc.current, tc.floor); got != tc.want {
			t.Errorf("releaseAtLeast(%q, %q) = %v, want %v", tc.current, tc.floor, got, tc.want)
		}
	}
}

func TestFloorKeepsPrereleaseSuffix(t *testing.T) {
	for expr, want := range map[string]string{
		">=2026.11.02-rc.1":       "2026.11.02-rc.1",
		"2026.11.02-rc.1 <3.0.0":  "2026.11.02-rc.1",
		">=2026.11.02-":           "",
		">=2026.11.02-../x":       "",
		">=2026.11.02-rc..1":      "",
		">=2026.11.02/x":          "",
		">=2026.11.02-rc.1+build": "",
	} {
		if got := floorOf(expr); got != want {
			t.Errorf("floorOf(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestReleaseAtLeastOrdersPrereleases(t *testing.T) {
	for _, tc := range []struct {
		current, floor string
		want           bool
	}{
		{"2026.11.02-rc.1", "2026.11.02-rc.1", true},
		{"2026.11.02-rc.1", "2026.11.02-rc.2", false},
		{"2026.11.02-rc.10", "2026.11.02-rc.2", true},
		{"2026.11.02", "2026.11.02-rc.1", true},
		{"2026.11.02-rc.1", "2026.11.02", false},
		{"2026.11.01", "2026.11.02-rc.1", false},
		{"2026.11.03-rc.1", "2026.11.02", true},
	} {
		if got := releaseAtLeast(tc.current, tc.floor); got != tc.want {
			t.Errorf("releaseAtLeast(%q, %q) = %v, want %v", tc.current, tc.floor, got, tc.want)
		}
	}
}

// TestUnreadableFloorRefuses: a floor that starts like a version but is malformed must
// not read as "no floor", or a hostile catalog entry would lift its own gate.
func TestUnreadableFloorRefuses(t *testing.T) {
	for _, min := range []string{"2026.11.02-", "2026.11.02-../x", "9999.01.01-"} {
		if err := rejectBelowMinimum(min); !errors.Is(err, ErrHostBelowMinimum) {
			t.Errorf("rejectBelowMinimum(%q) = %v, want a refusal", min, err)
		}
	}
	for _, min := range []string{"", "<3.0.0", ">=2026.09.01"} {
		if err := rejectBelowMinimum(min); err != nil {
			t.Errorf("rejectBelowMinimum(%q) = %v, want nil", min, err)
		}
	}
}

// TestInstallRefusesAnArtifactAboveTheHost covers both places a floor can come from:
// the catalog entry, refused before any download, and the tarball's own manifest,
// which is all an uploaded plugin has.
func TestInstallRefusesAnArtifactAboveTheHost(t *testing.T) {
	inst, _, _ := unsignedInstaller(t, nil)
	tarball := tarballWith(t, "2026.10.01", "", nil)

	if _, err := inst.Install(t.Context(), Source{Tarball: tarball, AllowUnsigned: true, MinMcharbor: "9999.01.01"}, nil); !errors.Is(err, ErrHostBelowMinimum) {
		t.Fatalf("catalog floor above the host: %v", err)
	}

	own := &plugin.Manifest{Slug: upgradeSlug, Name: "Up", Version: "2026.10.01", Mcharbor: ">=9999.01.01"}
	if err := validateManifest(own); !errors.Is(err, ErrHostBelowMinimum) {
		t.Fatalf("manifest floor above the host: %v", err)
	}

	if _, err := inst.Install(t.Context(), Source{Tarball: tarball, AllowUnsigned: true, MinMcharbor: "2026.09.01"}, nil); err != nil {
		t.Fatalf("a floor the host meets must install: %v", err)
	}
}
