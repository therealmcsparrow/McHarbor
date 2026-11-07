// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import "testing"

// TestRangeAllowsPrereleaseOrder pins that a prerelease target sits below its own final, so a
// floor at the final refuses it, while stable results stay as they were.
func TestRangeAllowsPrereleaseOrder(t *testing.T) {
	cases := []struct {
		rng, v         string
		allowed, known bool
	}{
		{">=2026.11.01", "2026.11.01-beta.1", false, true},
		{">=2026.11.01-beta.1", "2026.11.01-beta.2", true, true},
		{">=2026.11.01-beta.1", "2026.11.01", true, true},
		{"<2026.11.01", "2026.11.01-rc.1", true, true},
		{"^2026.9.0", "2026.12.01-rc.1", true, true},
		{"~2026.11.00", "2026.12.01-rc.1", false, true},
		{"~2026.11.00", "2026.11.20-rc.1", true, true},
		{">=2026.09.01", "v2026.09.02", true, true},
		{">=2026.09.01", "2026.09", false, false},
	}
	for _, c := range cases {
		allowed, known := RangeAllows(c.rng, c.v)
		if allowed != c.allowed || known != c.known {
			t.Errorf("RangeAllows(%q, %q) = (%v, %v), want (%v, %v)", c.rng, c.v, allowed, known, c.allowed, c.known)
		}
	}
}
