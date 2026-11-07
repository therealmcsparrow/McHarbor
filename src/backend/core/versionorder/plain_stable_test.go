// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versionorder

import "testing"

// TestIsPlainStableFailsClosed: only a strictly parsed version without a "-" suffix is stable, so
// near-spellings of a prerelease that Parse refuses are not mistaken for one.
func TestIsPlainStableFailsClosed(t *testing.T) {
	for in, want := range map[string]bool{
		"2026.11.02":                true,
		"2026.11.02+build.5":        true,
		"2026.11.02-rc.1":           false,
		"2026.11.02-rc-1":           false,
		"2026.11.02-rc_1":           false,
		"V2026.11.02":               false,
		"v2026.11.02":               false,
		"2026.11.02-rc.1+build":     false,
		"latest":                    false,
		"":                          false,
		"2026.11":                   false,
		"2026.11.02@sha256:abc":     false,
		"2026.11.02-RC.1":           false,
		"2026.11.02-":               false,
		"2026.11.02-rc.1@sha256:ab": false,
	} {
		if got := IsPlainStable(in); got != want {
			t.Errorf("IsPlainStable(%q) = %v, want %v", in, got, want)
		}
	}
}
