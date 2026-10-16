// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"slices"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

func inspected(name string, env []string, labels map[string]string) *container.InspectResponse {
	return &container.InspectResponse{
		Name: "/" + name, State: &container.State{Running: true, Status: "running"},
		HostConfig: &container.HostConfig{CapAdd: []string{"CAP_CHOWN"}},
		Config:     &container.Config{Image: "nginx:1", Env: env, Labels: labels},
	}
}

// TestRecreateKeepsTheEnvItWasNotAskedToChange guards the wholesale-replace
// trap: a recreate sets the whole env, so undeclared keys must be carried over.
func TestRecreateKeepsTheEnvItWasNotAskedToChange(t *testing.T) {
	ds := &DesiredState{Kind: KindContainer, ContainerName: "web", ImageRef: "nginx:2", Config: `{"env":{"MODE":"prod"},"capAdd":["NET_ADMIN"]}`}
	report := &DriftReport{Diffs: []DriftDiff{
		{Class: ClassConfiguration, Field: "image", Expected: "nginx:2", Actual: "nginx:1"},
		{Class: ClassConfiguration, Field: "env.MODE", Expected: "prod", Actual: "dev"},
		{Class: ClassConfiguration, Field: "capAdd.NET_ADMIN", Expected: "present", Actual: "missing"},
	}}
	steps := PlanFor(ds, report, inspected("web", []string{"PATH=/bin", "MODE=dev", "TOKEN=x"}, nil))
	if len(steps) != 1 || steps[0].Action != ActionRecreate || steps[0].Recreate == nil {
		t.Fatalf("steps = %+v", steps)
	}
	req := steps[0].Recreate
	if req.Image != "nginx:2" {
		t.Errorf("image = %q", req.Image)
	}
	if !slices.Equal(req.Env, []string{"PATH=/bin", "MODE=prod", "TOKEN=x"}) {
		t.Errorf("env = %v", req.Env)
	}
	if !slices.Equal(req.CapAdd, []string{"CHOWN", "NET_ADMIN"}) {
		t.Errorf("capAdd = %v", req.CapAdd)
	}
	if req.Labels != nil {
		t.Errorf("labels were replaced although no label drifted: %v", req.Labels)
	}
}

// TestPlanLeavesWhatItCannotDoToAPerson checks compose-managed containers,
// ports and missing containers become manual steps, never guesses.
func TestPlanLeavesWhatItCannotDoToAPerson(t *testing.T) {
	ds := &DesiredState{Kind: KindContainer, ContainerName: "web", ImageRef: "nginx:2"}
	image := DriftDiff{Class: ClassConfiguration, Field: "image", Expected: "nginx:2", Actual: "nginx:1"}
	composed := inspected("web", nil, map[string]string{"com.docker.compose.project": "shop"})
	steps := PlanFor(ds, &DriftReport{Diffs: []DriftDiff{image, {Class: ClassConfiguration, Field: "port.80/tcp"}}}, composed)
	if len(steps) != 2 || steps[0].Reason != ReasonComposeManaged || steps[0].Target != "shop" || steps[1].Reason != ReasonPorts {
		t.Fatalf("steps = %+v", steps)
	}
	if FullyApplicable(steps) {
		t.Fatalf("a plan with manual steps was called fully applicable")
	}
	missing := PlanFor(ds, &DriftReport{Diffs: []DriftDiff{{Class: ClassExistence, Field: "existence", Expected: "exists", Actual: "missing"}}}, nil)
	if len(missing) != 1 || missing[0].Reason != ReasonMissing {
		t.Fatalf("missing container plan = %+v", missing)
	}
}

// TestUndeclaredContainersAreStoppedNeverRemoved guards the environment plan.
func TestUndeclaredContainersAreStoppedNeverRemoved(t *testing.T) {
	ds := &DesiredState{Kind: KindEnvironment}
	steps := PlanFor(ds, &DriftReport{Diffs: []DriftDiff{
		{Class: ClassExistence, Field: "undeclared.miner", Expected: "not present", Actual: "xmrig"},
	}}, nil)
	if len(steps) != 1 || steps[0].Action != ActionStop || steps[0].Target != "miner" {
		t.Fatalf("steps = %+v", steps)
	}
}

// TestFingerprintIgnoresOrderAndTime checks two reports of the same drift
// match, and any changed value does not.
func TestFingerprintIgnoresOrderAndTime(t *testing.T) {
	a := DriftDiff{Class: ClassState, Field: "status", Expected: "running", Actual: "exited"}
	b := DriftDiff{Class: ClassConfiguration, Field: "image", Expected: "x:2", Actual: "x:1"}
	one := &DriftReport{Diffs: []DriftDiff{a, b}, CheckedAt: "2026-09-26T10:00:00Z"}
	two := &DriftReport{Diffs: []DriftDiff{b, a}, CheckedAt: "2026-09-26T11:00:00Z"}
	if Fingerprint(one) != Fingerprint(two) {
		t.Fatal("same drift, different fingerprint")
	}
	b.Actual = "x:3"
	if Fingerprint(one) == Fingerprint(&DriftReport{Diffs: []DriftDiff{a, b}}) {
		t.Fatal("different drift, same fingerprint")
	}
}

// TestDetectorComparesHealthcheckNetworksAndCapabilities covers the Phase 1
// leftovers: each of the three is reported, and a CAP_ prefix is not drift.
func TestDetectorComparesHealthcheckNetworksAndCapabilities(t *testing.T) {
	inspect := container.InspectResponse{
		State:      &container.State{Running: true},
		HostConfig: &container.HostConfig{CapAdd: []string{"CAP_NET_ADMIN"}},
		Config: &container.Config{Image: "nginx:1", Healthcheck: &container.HealthConfig{
			Test: []string{"CMD", "curl", "-f", "http://localhost"}, Interval: 30 * time.Second,
		}},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"front": {}}},
	}
	cfg, err := ParseConfig(`{"networks":["front","back"],"capAdd":["NET_ADMIN","SYS_TIME"],
		"healthcheck":{"test":["CMD","curl","-f","http://localhost"],"interval":"10s"}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	report := &DriftReport{}
	NewDetector(nil).compareConfig(inspect, &DesiredState{}, cfg, report)
	got := map[string]string{}
	for _, d := range report.Diffs {
		got[d.Field] = d.Actual
	}
	want := map[string]string{"network.back": "not attached", "capAdd.SYS_TIME": "missing", "healthcheck.interval": "30s"}
	if len(got) != len(want) {
		t.Fatalf("diffs = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if _, err := ParseConfig(`{"healthcheck":{"test":[]}}`, true); err == nil {
		t.Error("a healthcheck without a test was accepted")
	}
}
