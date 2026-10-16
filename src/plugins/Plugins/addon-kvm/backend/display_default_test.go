// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

// The agent no longer resolves Auto itself, so this table is what decides
// whether a guest boots with an adapter the console can resize.
func TestEffectiveDisplayResolvesAutoToAResizableAdapter(t *testing.T) {
	cases := []struct {
		stored, family, want string
	}{
		{"", OSFamilyLinux, "virtio-gpu"},
		{"", OSFamilyBSD, "virtio-gpu"},
		{"", OSFamilyOther, "virtio-gpu"},
		{"", "", "virtio-gpu"},
		{"", OSFamilyWindows, "qxl"},
		{"std", OSFamilyLinux, "std"},
		{"none", OSFamilyWindows, "none"},
	}
	for _, tc := range cases {
		if got := effectiveDisplay(tc.stored, tc.family); got != tc.want {
			t.Errorf("effectiveDisplay(%q, %q) = %q, want %q", tc.stored, tc.family, got, tc.want)
		}
	}
}
