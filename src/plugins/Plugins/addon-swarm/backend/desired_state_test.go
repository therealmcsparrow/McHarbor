// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

func desiredService(img string, replicas uint64) swarm.Service {
	var s swarm.Service
	s.ID = "svc1"
	s.Spec.Name = "web"
	s.Spec.Mode.Replicated = &swarm.ReplicatedService{Replicas: u64(replicas)}
	s.Spec.TaskTemplate.ContainerSpec = &swarm.ContainerSpec{Image: img}
	return s
}

func diffFields(diffs []desiredstate.Difference) map[string]desiredstate.Difference {
	out := map[string]desiredstate.Difference{}
	for _, d := range diffs {
		out[d.Field] = d
	}
	return out
}

func TestDesiredImageNormalisation(t *testing.T) {
	running := "docker.io/library/nginx:1.25@sha256:ABCDEF"
	for _, want := range []string{"nginx:1.25", "library/nginx:1.25", "docker.io/library/nginx:1.25", " NGINX:1.25 ", "nginx:1.25@sha256:abcdef", "nginx@sha256:abcdef"} {
		if d := serviceDrift(desiredService(running, 1), map[string]string{fieldImage: want}); len(d) != 0 {
			t.Errorf("pin %q against %q must not drift, got %v", want, running, d)
		}
	}
	if d := serviceDrift(desiredService("nginx", 1), map[string]string{fieldImage: "nginx:latest"}); len(d) != 0 {
		t.Errorf("an untagged image is :latest, got %v", d)
	}
	if d := serviceDrift(desiredService("registry.local:5000/app:2", 1), map[string]string{fieldImage: "registry.local:5000/app:2"}); len(d) != 0 {
		t.Errorf("a registry port is not a tag, got %v", d)
	}
	d := diffFields(serviceDrift(desiredService(running, 1), map[string]string{fieldImage: "nginx:1.26"}))
	if got := d[fieldImage]; got.Class != classImage || got.Expected != "nginx:1.26" || got.Actual != "nginx:1.25" {
		t.Fatalf("tag change must drift as image, got %+v", got)
	}
	if d := serviceDrift(desiredService(running, 1), map[string]string{fieldImage: "nginx:1.25@sha256:000000"}); len(d) != 1 {
		t.Fatalf("a pinned digest is compared, got %v", d)
	}
	if got := currentServiceSpec(desiredService(running, 3)); got[fieldImage] != "nginx:1.25" || got[fieldReplicas] != "3" || got[fieldUpdateState] != updateStateSettled {
		t.Fatalf("current spec = %v", got)
	}
}

func TestDesiredReplicasDrift(t *testing.T) {
	s := desiredService("nginx:1", 3)
	for _, want := range []string{"3", " 3 ", "3.0"} {
		if d := serviceDrift(s, map[string]string{fieldReplicas: want}); len(d) != 0 {
			t.Errorf("replicas %q against 3 must not drift, got %v", want, d)
		}
	}
	d := diffFields(serviceDrift(s, map[string]string{fieldReplicas: "5"}))
	if got := d[fieldReplicas]; got.Class != classState || got.Expected != "5" || got.Actual != "3" {
		t.Fatalf("replicas drift = %+v", got)
	}
	if d := serviceDrift(s, map[string]string{fieldReplicas: "2.5"}); len(d) != 1 {
		t.Fatalf("an invalid pin must be reported, got %v", d)
	}
	var global swarm.Service
	global.Spec.Mode.Global = &swarm.GlobalService{}
	if d := serviceDrift(global, map[string]string{fieldReplicas: "4"}); len(d) != 0 {
		t.Fatalf("replicas does not apply to a global service, got %v", d)
	}
}

func TestDesiredUpdateStateAndEmptySpec(t *testing.T) {
	s := desiredService("nginx:1", 1)
	if d := serviceDrift(s, map[string]string{}); len(d) != 0 {
		t.Fatalf("empty spec must not drift, got %v", d)
	}
	if d := serviceDrift(s, map[string]string{fieldImage: " ", fieldReplicas: ""}); len(d) != 0 {
		t.Fatalf("blank pins are not managed, got %v", d)
	}
	s.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStateUpdating}
	if d := serviceDrift(s, map[string]string{fieldUpdateState: "completed"}); len(d) != 0 {
		t.Fatalf("an update in progress is not drift, got %v", d)
	}
	s.UpdateStatus.State = swarm.UpdateStateRollbackPaused
	d := diffFields(serviceDrift(s, map[string]string{fieldUpdateState: "Completed"}))
	if got := d[fieldUpdateState]; got.Actual != "rollback_paused" || got.Expected != "completed" {
		t.Fatalf("paused rollback must drift, got %+v", got)
	}
}

func TestDesiredNodeDrift(t *testing.T) {
	var n swarm.Node
	n.Spec.Availability = swarm.NodeAvailabilityActive
	if d := nodeDrift(n, map[string]string{fieldAvailability: " Active "}); len(d) != 0 {
		t.Fatalf("case and whitespace must not drift, got %v", d)
	}
	if d := nodeDrift(n, nil); len(d) != 0 {
		t.Fatalf("empty spec must not drift, got %v", d)
	}
	d := nodeDrift(n, map[string]string{fieldAvailability: "drain"})
	if len(d) != 1 || d[0].Expected != "drain" || d[0].Actual != "active" {
		t.Fatalf("availability drift = %v", d)
	}
}

func TestDesiredPlanService(t *testing.T) {
	spec := map[string]string{fieldImage: "nginx:2", fieldReplicas: "2", fieldUpdateState: "completed"}
	diffs := []desiredstate.Difference{
		{Class: classState, Field: fieldReplicas, Expected: "2", Actual: "4"},
		{Class: classImage, Field: fieldImage, Expected: "nginx:2", Actual: "nginx:1"},
		{Class: classState, Field: fieldUpdateState, Expected: "completed", Actual: "paused"},
	}
	steps := planServiceSteps(spec, diffs)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %+v", steps)
	}
	if steps[0].Action != actionResumeUpdate || !steps[0].Manual {
		t.Fatalf("paused update comes first and is manual: %+v", steps[0])
	}
	if steps[1].Action != actionSetImage || steps[1].Manual || !steps[1].Disruptive {
		t.Fatalf("image change is automatic and disruptive: %+v", steps[1])
	}
	if steps[2].Action != actionSetReplicas || steps[2].Manual || !steps[2].Disruptive {
		t.Fatalf("scaling down is automatic and disruptive: %+v", steps[2])
	}
	up := planServiceSteps(map[string]string{fieldReplicas: "5"}, []desiredstate.Difference{{Field: fieldReplicas, Expected: "5", Actual: "4"}})
	if len(up) != 1 || up[0].Disruptive {
		t.Fatalf("scaling up is not disruptive: %+v", up)
	}
	bad := planServiceSteps(map[string]string{fieldReplicas: "x"}, []desiredstate.Difference{{Field: fieldReplicas, Expected: "x", Actual: "4"}})
	if len(bad) != 1 || !bad[0].Manual {
		t.Fatalf("an invalid pin plans a manual fix: %+v", bad)
	}
	gone := planServiceSteps(spec, append(diffs, existenceDiff()))
	if len(gone) != 1 || !gone[0].Manual || gone[0].Action != actionRecreate {
		t.Fatalf("existence drift is a single manual recreate: %+v", gone)
	}
}

func TestDesiredPlanNode(t *testing.T) {
	steps := planNodeSteps(map[string]string{fieldAvailability: "drain"}, []desiredstate.Difference{{Field: fieldAvailability, Expected: "drain", Actual: "active"}})
	if len(steps) != 1 || steps[0].Manual || !steps[0].Disruptive {
		t.Fatalf("drain is automatic and disruptive: %+v", steps)
	}
	steps = planNodeSteps(map[string]string{fieldAvailability: "active"}, []desiredstate.Difference{{Field: fieldAvailability, Expected: "active", Actual: "drain"}})
	if len(steps) != 1 || steps[0].Disruptive {
		t.Fatalf("activating is not disruptive: %+v", steps)
	}
	gone := planNodeSteps(nil, []desiredstate.Difference{existenceDiff()})
	if len(gone) != 1 || !gone[0].Manual {
		t.Fatalf("a removed node is manual: %+v", gone)
	}
}

type fakeApplier struct {
	calls []string
}

func (f *fakeApplier) ScaleService(_ context.Context, envID, id string, replicas uint64) error {
	f.calls = append(f.calls, "scale:"+envID+":"+id+":"+strconv.FormatUint(replicas, 10))
	return nil
}

func (f *fakeApplier) SetServiceImage(_ context.Context, envID, id, ref string) error {
	f.calls = append(f.calls, "image:"+envID+":"+id+":"+ref)
	return nil
}

func (f *fakeApplier) SetNodeAvailability(_ context.Context, envID, id, availability string) error {
	f.calls = append(f.calls, "avail:"+envID+":"+id+":"+availability)
	return nil
}

func TestDesiredApplyStepDispatch(t *testing.T) {
	ctx := context.Background()
	f := &fakeApplier{}
	spec := map[string]string{fieldReplicas: "3.0", fieldImage: " nginx:2 ", fieldAvailability: "Drain"}
	if _, err := applyDesiredStep(ctx, f, desiredKindService, "env", "svc1", spec, desiredstate.Step{Action: actionSetReplicas}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDesiredStep(ctx, f, desiredKindService, "env", "svc1", spec, desiredstate.Step{Action: actionSetImage}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDesiredStep(ctx, f, desiredKindNode, "env", "n1", spec, desiredstate.Step{Action: actionSetAvailability}); err != nil {
		t.Fatal(err)
	}
	want := []string{"scale:env:svc1:3", "image:env:svc1:nginx:2", "avail:env:n1:drain"}
	if len(f.calls) != len(want) {
		t.Fatalf("calls = %v", f.calls)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("call %d = %q, want %q", i, f.calls[i], want[i])
		}
	}
	for _, step := range []desiredstate.Step{
		{Action: actionResumeUpdate, Manual: true},
		{Action: actionSetAvailability},
		{Action: "delete"},
	} {
		if _, err := applyDesiredStep(ctx, f, desiredKindService, "env", "svc1", spec, step); err == nil {
			t.Errorf("step %+v on a service must be refused", step)
		}
	}
	if _, err := applyDesiredStep(ctx, f, desiredKindService, "env", "svc1", map[string]string{fieldReplicas: "5000"}, desiredstate.Step{Action: actionSetReplicas}); err == nil {
		t.Error("a replica count above the bound must be refused")
	}
	if len(f.calls) != len(want) {
		t.Fatalf("refused steps must not reach the service, calls = %v", f.calls)
	}
}
