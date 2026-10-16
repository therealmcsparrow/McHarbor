// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/desiredstatesources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

type fakeKinds struct {
	mu      sync.Mutex
	actual  map[string]string
	applied []string
	down    bool
}

func (f *fakeKinds) DesiredKinds() []desiredstate.Kind {
	return []desiredstate.Kind{{Kind: "t_vm", Label: "VM", SubjectLabel: "Virtual machine", Enforceable: true, Fields: []desiredstate.Field{
		{Key: "power", Type: desiredstate.FieldSelect, Options: []desiredstate.Option{{Value: "running"}, {Value: "stopped"}}},
		{Key: "memory_mib", Type: desiredstate.FieldNumber},
	}}}
}

func (f *fakeKinds) DesiredSubjects(context.Context, string, string) ([]desiredstate.Option, error) {
	return []desiredstate.Option{{Value: "vm-1", Label: "db-vm"}}, nil
}

func (f *fakeKinds) DesiredOptions(context.Context, string, string, string) ([]desiredstate.Option, error) {
	return nil, nil
}

func (f *fakeKinds) CurrentSpec(context.Context, string, string, string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]string{"power": f.actual["power"], "memory_mib": f.actual["memory_mib"]}, nil
}

func (f *fakeKinds) DetectDrift(_ context.Context, _, _, _ string, spec map[string]string) ([]desiredstate.Difference, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, context.DeadlineExceeded
	}
	var out []desiredstate.Difference
	for k, v := range spec {
		if f.actual[k] != v {
			out = append(out, desiredstate.Difference{Class: "state", Field: k, Expected: v, Actual: f.actual[k]})
		}
	}
	return out, nil
}

func (f *fakeKinds) PlanDrift(_ context.Context, _, _, _ string, spec map[string]string, diffs []desiredstate.Difference) ([]desiredstate.Step, error) {
	var out []desiredstate.Step
	for _, d := range diffs {
		switch d.Field {
		case "power":
			out = append(out, desiredstate.Step{Action: "set_power", Fields: []string{"power"}, Disruptive: spec["power"] == "stopped"})
		case "memory_mib":
			out = append(out, desiredstate.Step{Action: "set_memory", Fields: []string{"memory_mib"}, Manual: true, Reason: "needs a restart"})
		}
	}
	return out, nil
}

func (f *fakeKinds) ApplyStep(_ context.Context, _, _, _ string, spec map[string]string, step desiredstate.Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, step.Action)
	if step.Action == "set_power" {
		f.actual["power"] = spec["power"]
	}
	return nil
}

func (f *fakeKinds) appliedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.applied)
}

// TestPluginKindsAreValidatedDetectedAndEnforced guards the plugin path end to
// end: only declared fields are accepted, a scope that is not an environment is
// kept, drift comes from the plugin, a safe step is enforced, a disruptive or
// manual one only proposed, and an unreachable plugin is never drift.
func TestPluginKindsAreValidatedDetectedAndEnforced(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	_, act := withResponder(t, loop, database)
	fake := &fakeKinds{actual: map[string]string{"power": "stopped", "memory_mib": "2048"}}
	desiredstatesources.Register("t-kvm", fake)
	t.Cleanup(func() { desiredstatesources.Unregister("t-kvm") })
	ctx := context.Background()

	if err := ValidatePluginInput("t-kvm", "t_vm", "vm-1", map[string]string{"cpu": "4"}); err == nil {
		t.Fatal("an undeclared field was accepted")
	}
	if err := ValidatePluginInput("t-kvm", "t_vm", "vm-1", map[string]string{"power": "sleeping"}); err == nil {
		t.Fatal("a value outside the field's choices was accepted")
	}
	if err := ValidatePluginInput("t-kvm", "t_vm", "vm-1", map[string]string{"power": " "}); err == nil {
		t.Fatal("a spec that pins nothing was accepted")
	}

	ds, err := loop.service.Create(t.Context(), CreateDesiredStateInput{Name: "db", Kind: "t_vm", EnvID: "cluster-9", Mode: ModeEnforce,
		Source: "t-kvm", SubjectID: "vm-1", Spec: map[string]string{"power": "running"}})
	if err != nil {
		t.Fatalf("a plugin state scoped outside the environments table must save: %v", err)
	}
	if ds.EnvID != "cluster-9" || ds.Source != "t-kvm" || ds.Spec["power"] != "running" {
		t.Fatalf("stored %+v", ds)
	}
	setEnforcement(t, database, true, 5, 0, 3)

	for i := 0; i < 3 && fake.appliedCount() == 0; i++ {
		loop.pass(ctx)
	}
	if fake.appliedCount() != 1 || fake.applied[0] != "set_power" || act.count() != 0 {
		t.Fatalf("a safe plugin step must be applied through the plugin: plugin=%v docker=%v", fake.applied, act.calls)
	}

	if _, err := loop.service.Update(t.Context(), ds.ID, UpdateDesiredStateInput{Spec: &map[string]string{"power": "stopped"}}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.actual["power"] = "running"
	fake.mu.Unlock()
	for i := 0; i < 4; i++ {
		loop.pass(ctx)
	}
	if fake.appliedCount() != 1 || proposalsOf(t, database, ds.ID, ProposalPending) != 1 {
		t.Fatalf("a disruptive plugin step must be proposed, not enforced: applied=%v", fake.applied)
	}

	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()
	if report, err := loop.detector.Detect(ctx, nil, ds, nil); err == nil || report != nil {
		t.Fatalf("an unreachable plugin must be unknown, not drift: %+v %v", report, err)
	}
}
