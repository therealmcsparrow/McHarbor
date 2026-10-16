// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import "testing"

func TestInstallImageKeepsChannelTags(t *testing.T) {
	latest := &LatestRelease{Version: "stable"}
	cases := []struct {
		name   string
		image  string
		latest *LatestRelease
		want   string
	}{
		{"channel tag survives", "ghcr.io/blakeblackshear/frigate:stable", latest, "ghcr.io/blakeblackshear/frigate:stable"},
		{"nightly survives", "example/app:nightly", latest, "example/app:nightly"},
		{"variant channel survives", "example/app:alpine", latest, "example/app:alpine"},
		{"release tag follows latest", "example/app:1.4.2", latest, "example/app:latest"},
		{"registry port with release tag", "registry.local:5000/app:2.0", latest, "registry.local:5000/app:latest"},
		{"registry port untagged", "registry.local:5000/app", latest, "registry.local:5000/app"},
		{"untagged", "example/app", latest, "example/app"},
		{"digest pinned", "example/app@sha256:abc", latest, "example/app@sha256:abc"},
		{"no latest block", "example/app:1.4.2", nil, "example/app:1.4.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := installImage(AppTemplate{Image: tc.image, Latest: tc.latest})
			if got != tc.want {
				t.Errorf("installImage(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}
