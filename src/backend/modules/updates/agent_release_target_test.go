// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"testing"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

func TestAgentOutdatedComparesVersionThenBuild(t *testing.T) {
	cases := []struct {
		name                   string
		version, build, tv, tb string
		want                   bool
	}{
		{"older version", "2026.09.01", "202609281200", "2026.09.02", "", true},
		{"newer version is never downgraded", "2026.09.03", "", "2026.09.02", "202609281200", false},
		{"same version, older build", "2026.09.02", "202609271000", "2026.09.02", "202609281200", true},
		{"same version, same build", "2026.09.02", "202609281200", "2026.09.02", "202609281200", false},
		{"no target build to compare", "2026.09.02", "202609271000", "2026.09.02", "", false},
		{"agent reports no build", "2026.09.02", "", "2026.09.02", "202609281200", false},
	}
	for _, tc := range cases {
		if got := agentOutdated(tc.version, tc.build, tc.tv, tc.tb); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestUpdatePayloadOffersOnlyWhatMcHarborServes(t *testing.T) {
	none := updateArtifacts{osName: "linux", arch: "arm64"}
	if p := none.payload("https://mcharbor.example", "2026.09.02", "", true); p.BinaryURL != "" || p.ImageURL != "" {
		t.Fatalf("nothing stored must offer nothing, got %+v", p)
	}

	art := updateArtifacts{osName: "linux", arch: "amd64", binaryPath: "/x", binarySHA: "b1", imagePath: "/y", imageSHA: "i1", imageRef: "mcharbor-agent:2026.09.02"}
	legacy := art.payload("https://mcharbor.example/", "2026.09.02", "202609281200", false)
	if legacy.BinaryURL != "https://mcharbor.example/api/agent/binary/linux/amd64" || legacy.BinarySHA256 != "b1" {
		t.Fatalf("binary offer = %+v", legacy)
	}
	if legacy.ImageURL != "" || legacy.ImageRef != "" || legacy.Image != "" {
		t.Fatalf("an agent that cannot load images must get the old request unchanged, got %+v", legacy)
	}
	current := art.payload("https://mcharbor.example", "2026.09.02", "202609281200", true)
	if current.ImageURL != "https://mcharbor.example/api/agent/image/linux/amd64" || current.ImageSHA256 != "i1" || current.ImageRef != "mcharbor-agent:2026.09.02" {
		t.Fatalf("image offer = %+v", current)
	}
	for _, u := range []string{current.BinaryURL, current.ImageURL} {
		if u == "" || u[:len("https://mcharbor.example")] != "https://mcharbor.example" {
			t.Fatalf("offer points away from McHarbor: %q", u)
		}
	}
}

func TestImageLoadFloorUsesTheBuild(t *testing.T) {
	cases := []struct {
		version, build string
		want           bool
	}{
		{coreagent.AgentImageLoadMinVersion, coreagent.AgentImageLoadMinBuild, true},
		{coreagent.AgentImageLoadMinVersion, "202609270250", false},
		{coreagent.AgentImageLoadMinVersion, "dev", false},
		{coreagent.AgentImageLoadMinVersion, "", false},
		{"2026.09.02", "", true},
		{"3.8.0", "202612010000", false},
	}
	for _, tc := range cases {
		if got := coreagent.SupportsImageLoad(tc.version, tc.build); got != tc.want {
			t.Errorf("SupportsImageLoad(%q, %q) = %v, want %v", tc.version, tc.build, got, tc.want)
		}
	}
}
