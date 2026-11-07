// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import "testing"

func TestAgentOutdatedOrdersPrereleases(t *testing.T) {
	cases := []struct {
		name                   string
		version, build, tv, tb string
		want                   bool
	}{
		{"prerelease agent moves to a later prerelease", "2026.11.02-alpha.1", "", "2026.11.02-alpha.2", "", true},
		{"alpha.10 is later than alpha.9", "2026.11.02-alpha.9", "", "2026.11.02-alpha.10", "", true},
		{"stable agent is never moved down to a prerelease", "2026.11.02", "", "2026.11.02-rc.1", "", false},
		{"prerelease agent is never moved down to the prior stable", "2026.11.02-alpha.1", "", "2026.11.01", "", false},
		{"prerelease agent moves up to its own stable", "2026.11.02-rc.1", "", "2026.11.02", "", true},
		{"stable agent moves to a later core prerelease", "2026.11.01", "", "2026.11.02-alpha.1", "", true},
		{"same prerelease, older build", "2026.11.02-alpha.1", "202611070000", "2026.11.02-alpha.1", "202611071200", true},
		{"no target at all offers nothing", "2026.11.02-alpha.1", "", "", "", false},
	}
	for _, tc := range cases {
		if got := agentOutdated(tc.version, tc.build, tc.tv, tc.tb); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
