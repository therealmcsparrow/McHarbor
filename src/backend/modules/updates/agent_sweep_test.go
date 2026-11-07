// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

func setChannels(t *testing.T, f *returnFixture, switchOn, optionOn bool) {
	t.Helper()
	p := updatechannel.Patch{AgentPrereleases: &switchOn, AgentAutoPrereleases: &optionOn}
	if _, _, err := updatechannel.Apply(context.Background(), f.db, p); err != nil {
		t.Fatal(err)
	}
}

func selectTarget(f *returnFixture, version string) {
	f.updater.selected = func(context.Context) (agentreleases.Release, bool) {
		return agentreleases.Release{Version: version, Assets: []agentreleases.Asset{{OS: "linux", Arch: "amd64"}}}, true
	}
}

func TestAutomaticAgentSweepNeedsSwitchAndOption(t *testing.T) {
	cases := []struct {
		name            string
		target          string
		switchOn, optOn bool
		autoAgents      bool
		wantRan         bool
		wantStatus      string
	}{
		{"prerelease target, switch off", "2026.11.02-alpha.2", false, false, true, true, "skipped"},
		{"prerelease target, switch on, option off", "2026.11.02-alpha.2", true, false, true, true, "skipped"},
		{"prerelease target, option on, switch off", "2026.11.02-alpha.2", false, true, true, true, "skipped"},
		{"prerelease target, switch and option on", "2026.11.02-alpha.2", true, true, true, true, "failed"},
		{"stable target needs neither", "2026.11.03", false, false, true, true, "failed"},
		{"auto agents off never sweeps", "2026.11.03", true, true, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReturnFixture(t, "native")
			setChannels(t, f, tc.switchOn, tc.optOn)
			selectTarget(f, tc.target)
			summary, ran := f.checker.sweepAgentsAuto(context.Background(), SelfUpdateSettings{AutoUpdateAgents: tc.autoAgents})
			if ran != tc.wantRan {
				t.Fatalf("ran = %v, want %v", ran, tc.wantRan)
			}
			var got string
			for _, r := range summary.Results {
				if r.EnvID == "env1" {
					got = r.Status
				}
			}
			if got != tc.wantStatus {
				t.Fatalf("env1 status = %q, want %q", got, tc.wantStatus)
			}
		})
	}
}
