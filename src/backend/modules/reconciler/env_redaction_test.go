// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// TestDriftReportWithholdsLiveCredentials keeps a container's live password out of a drift
// report, which is stored, written to the audit trail and shown to reconciler viewers.
func TestDriftReportWithholdsLiveCredentials(t *testing.T) {
	inspect := container.InspectResponse{
		State:  &container.State{Running: true},
		Config: &container.Config{Image: "nginx:1", Env: []string{"DB_PASSWORD=live-password-1", "MODE=blue"}},
	}
	cfg, err := ParseConfig(`{"env":{"DB_PASSWORD":"desired-password-2","MODE":"green"}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	report := &DriftReport{}
	NewDetector(nil).compareConfig(inspect, &DesiredState{}, cfg, report)
	got := map[string]DriftDiff{}
	for _, d := range report.Diffs {
		got[d.Field] = d
	}
	if d := got["env.DB_PASSWORD"]; d.Actual != envredact.Marker || d.Expected != envredact.Marker {
		t.Errorf("credential drift = %+v, want both sides withheld", d)
	}
	if d := got["env.MODE"]; d.Actual != "blue" || d.Expected != "green" {
		t.Errorf("ordinary drift = %+v, want the real values", d)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("config:\n  env:\n    DB_PASSWORD: desired-password-2\n"), &doc); err != nil {
		t.Fatal(err)
	}
	if applyDiffToDoc(&doc, got["env.DB_PASSWORD"]) {
		t.Error("accepting a withheld value would write the marker into the source")
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), envredact.Marker) {
		t.Errorf("the source document gained the marker:\n%s", out)
	}
}
