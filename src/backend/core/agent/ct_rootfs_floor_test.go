// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import "testing"

func TestSupportsCTRootfs(t *testing.T) {
	cases := []struct {
		version, build string
		want           bool
	}{
		{CTRootfsMinVersion, CTRootfsMinBuild, true},
		{CTRootfsMinVersion, "209912312359", true},
		{CTRootfsMinVersion, "202609010000", false},
		{CTRootfsMinVersion, "dev", false},
		{CTRootfsMinVersion, "", false},
		{"3.8.0", "209912312359", false},
		{"2026.10.01", "", true},
	}
	for _, tc := range cases {
		if got := SupportsCTRootfs(tc.version, tc.build); got != tc.want {
			t.Errorf("SupportsCTRootfs(%q, %q) = %v, want %v", tc.version, tc.build, got, tc.want)
		}
	}
}
