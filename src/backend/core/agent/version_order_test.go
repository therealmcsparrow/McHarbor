// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

type floorCases struct {
	Meets []struct {
		Host  string `yaml:"host"`
		Floor string `yaml:"floor"`
		Want  bool   `yaml:"want"`
	} `yaml:"meets"`
}

// TestPrereleaseGates runs the shared floor cases through every agent comparator,
// so a prerelease agent meets a prerelease floor and not its stable base.
func TestPrereleaseGates(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/pluginmanifest/floor_cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases floorCases
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Meets) == 0 {
		t.Fatal("no meets cases loaded")
	}
	for _, c := range cases.Meets {
		if got := agentAtLeast(c.Host, c.Floor); got != c.Want {
			t.Errorf("agentAtLeast(%q, %q) = %v, want %v", c.Host, c.Floor, got, c.Want)
		}
		if got := AgentVersionAtLeast(c.Host, c.Floor); got != c.Want {
			t.Errorf("AgentVersionAtLeast(%q, %q) = %v, want %v", c.Host, c.Floor, got, c.Want)
		}
		if got := compareAgentVersions(c.Host, c.Floor) >= 0; got != c.Want {
			t.Errorf("compareAgentVersions(%q, %q) >= 0 = %v, want %v", c.Host, c.Floor, got, c.Want)
		}
	}
}

// TestPrereleaseGatesKeepHistoricalFloors pins the 3.x and dated gates against a prerelease agent.
func TestPrereleaseGatesKeepHistoricalFloors(t *testing.T) {
	for _, gate := range []string{ComputeMinVersion, ImageLibraryMinVersion, GuestNetworkingMinVersion, FreeVolumeMinVersion, HostUpdatesMinVersion, PackageInstallMinVersion} {
		if !AgentVersionAtLeast("2026.11.02-rc.1", gate) {
			t.Errorf("prerelease agent should clear gate %s", gate)
		}
		if AgentVersionAtLeast("1.0.0", gate) {
			t.Errorf("old agent should not clear gate %s", gate)
		}
	}
	if !agentSupportsRequestStreaming("2026.11.02-rc.1") {
		t.Error("prerelease agent should support request streaming")
	}
	if agentSupportsRequestStreaming("1.3.2") {
		t.Error("1.3.2 must not support request streaming")
	}
	if !SupportsCTBackup("2026.11.02-rc.1", "") {
		t.Error("a later prerelease clears CTBackup without a build")
	}
	if SupportsCTBackup("2026.09.01-rc.1", "202610010000") {
		t.Error("a prerelease of the gate version is below the gate")
	}
}

// TestAgentComparatorsIgnoreBuildMetadata: "+build.5" is metadata, so the legacy part reader that
// turned "02+build" into 0 must not decide it, while short legacy strings keep their reading.
func TestAgentComparatorsIgnoreBuildMetadata(t *testing.T) {
	for _, c := range []struct {
		host, floor string
		want        bool
	}{
		{"2026.11.02+build.5", "2026.11.02", true},
		{"2026.11.02", "2026.11.02+build.5", true},
		{"2026.11.03+build.5", "2026.11.02", true},
		{"2026.11.02+build.5", "2026.11.03", false},
		{"3.8", "3.8.0", true},
		{"3.7", "3.8.0", false},
	} {
		if got := agentAtLeast(c.host, c.floor); got != c.want {
			t.Errorf("agentAtLeast(%q, %q) = %v, want %v", c.host, c.floor, got, c.want)
		}
		if got := compareAgentVersions(c.host, c.floor) >= 0; got != c.want {
			t.Errorf("compareAgentVersions(%q, %q) >= 0 = %v, want %v", c.host, c.floor, got, c.want)
		}
	}
	if compareAgentVersions("2026.11.02+build.5", "2026.11.02") != 0 {
		t.Error("build metadata must not change the order")
	}
}
