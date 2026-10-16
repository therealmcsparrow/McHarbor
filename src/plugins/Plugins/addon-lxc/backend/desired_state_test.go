// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"reflect"
	"testing"

	desiredspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

func desiredFixture() Container {
	return Container{
		ID: "ct1", EnvironmentID: "env1", Name: "web", State: StateRunning,
		Autostart: true,
		Limits: Limits{
			CPUMax: "150000 100000", CPUWeight: 100, PidsMax: 4096,
			MemoryMaxBytes: 512 * mebibyte, MemoryHighBytes: 512 * mebibyte / 10 * 9,
		},
	}
}

func TestDesiredNormalisationNoFalseDrift(t *testing.T) {
	current := currentValues(desiredFixture(), powerRunning)
	equivalent := []map[string]string{
		{fieldPower: " Running "},
		{fieldCPUMax: "150000"},
		{fieldCPUMax: "150000   100000"},
		{fieldMemoryMax: "512"},
		{fieldMemoryMax: "512.0"},
		{fieldMemoryMax: " 512.000 "},
		{fieldAutostart: "TRUE"},
		{fieldAutostart: "yes"},
		{fieldAutostart: "1"},
	}
	for _, spec := range equivalent {
		diffs, err := diffSpec(current, spec)
		if err != nil {
			t.Fatalf("%v: %v", spec, err)
		}
		if len(diffs) != 0 {
			t.Errorf("%v reported drift: %+v", spec, diffs)
		}
	}

	unlimited := desiredFixture()
	unlimited.Limits.CPUMax = ""
	unlimited.Limits.MemoryMaxBytes = 0
	for _, spec := range []map[string]string{
		{fieldCPUMax: "max"}, {fieldCPUMax: "MAX 100000"},
		{fieldMemoryMax: "0"}, {fieldMemoryMax: "unlimited"},
	} {
		diffs, err := diffSpec(currentValues(unlimited, ""), spec)
		if err != nil || len(diffs) != 0 {
			t.Errorf("%v: diffs=%+v err=%v", spec, diffs, err)
		}
	}
}

func TestDesiredDriftPerField(t *testing.T) {
	current := currentValues(desiredFixture(), powerRunning)
	cases := []struct {
		spec map[string]string
		want desiredspec.Difference
	}{
		{map[string]string{fieldPower: "stopped"}, desiredspec.Difference{Class: classState, Field: fieldPower, Expected: "stopped", Actual: "running"}},
		{map[string]string{fieldCPUMax: "200000"}, desiredspec.Difference{Class: classConfiguration, Field: fieldCPUMax, Expected: "200000 100000", Actual: "150000 100000"}},
		{map[string]string{fieldMemoryMax: "1024"}, desiredspec.Difference{Class: classConfiguration, Field: fieldMemoryMax, Expected: "1024", Actual: "512"}},
		{map[string]string{fieldMemoryMax: "512.5"}, desiredspec.Difference{Class: classConfiguration, Field: fieldMemoryMax, Expected: "512.5", Actual: "512"}},
		{map[string]string{fieldAutostart: "false"}, desiredspec.Difference{Class: classConfiguration, Field: fieldAutostart, Expected: "false", Actual: "true"}},
	}
	for _, tc := range cases {
		diffs, err := diffSpec(current, tc.spec)
		if err != nil {
			t.Fatalf("%v: %v", tc.spec, err)
		}
		if len(diffs) != 1 || diffs[0] != tc.want {
			t.Errorf("%v: got %+v, want %+v", tc.spec, diffs, tc.want)
		}
	}

	frozen, err := diffSpec(currentValues(desiredFixture(), powerFrozen), map[string]string{fieldPower: "running"})
	if err != nil || len(frozen) != 1 || frozen[0].Actual != powerFrozen {
		t.Errorf("frozen is not running: %+v %v", frozen, err)
	}
}

func TestDesiredEmptySpecNoDrift(t *testing.T) {
	current := currentValues(desiredFixture(), powerStopped)
	for _, spec := range []map[string]string{nil, {}, {fieldPower: "", fieldCPUMax: "  ", fieldMemoryMax: "", fieldAutostart: ""}} {
		diffs, err := diffSpec(current, spec)
		if err != nil || len(diffs) != 0 {
			t.Errorf("%v: diffs=%+v err=%v", spec, diffs, err)
		}
	}
}

func TestDesiredInvalidValueIsError(t *testing.T) {
	current := currentValues(desiredFixture(), powerRunning)
	for _, spec := range []map[string]string{
		{fieldPower: "paused"}, {fieldCPUMax: "1.5"}, {fieldCPUMax: "100 0"},
		{fieldMemoryMax: "-1"}, {fieldMemoryMax: "lots"}, {fieldAutostart: "maybe"},
	} {
		if _, err := diffSpec(current, spec); err == nil {
			t.Errorf("%v: expected an error", spec)
		}
	}
}

func TestDesiredSubjectGoneIsExistence(t *testing.T) {
	d := existenceDifference()
	if d.Class != "existence" || d.Field != "existence" || d.Expected != "exists" || d.Actual != "missing" {
		t.Fatalf("existence difference = %+v", d)
	}
	steps := planSteps(false, []desiredspec.Difference{
		{Class: classState, Field: fieldPower, Expected: "running", Actual: "stopped"}, d,
	})
	if len(steps) != 1 || !steps[0].Manual || steps[0].Action != actionRecreate || steps[0].Reason == "" {
		t.Fatalf("existence plan = %+v", steps)
	}
}

func TestDesiredPowerOf(t *testing.T) {
	for state, want := range map[string]string{
		StateRunning: powerRunning, StateFrozen: powerFrozen, StateStopped: powerStopped,
		StateDefined: powerStopped, StateCrashed: powerStopped,
	} {
		if got, ok := powerOf(state); !ok || got != want {
			t.Errorf("powerOf(%s) = %q, %t", state, got, ok)
		}
	}
	if _, ok := powerOf(StateUnknown); ok {
		t.Error("an unknown state must not become a power value")
	}
}

func TestDesiredPlanOrderAndFlags(t *testing.T) {
	diffs := []desiredspec.Difference{
		{Class: classState, Field: fieldPower, Expected: "stopped", Actual: "running"},
		{Class: classConfiguration, Field: fieldAutostart, Expected: "false", Actual: "true"},
		{Class: classConfiguration, Field: fieldMemoryMax, Expected: "256", Actual: "512"},
		{Class: classConfiguration, Field: fieldCPUMax, Expected: "max", Actual: "150000 100000"},
	}
	steps := planSteps(false, diffs)
	var actions []string
	for _, s := range steps {
		actions = append(actions, s.Action)
		if s.Manual {
			t.Errorf("%s should be automatic", s.Action)
		}
	}
	want := []string{actionSetCPUMax, actionSetMemoryMax, actionSetAutostart, actionStop}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("order = %v, want %v", actions, want)
	}
	disruptive := map[string]bool{actionSetCPUMax: false, actionSetMemoryMax: true, actionSetAutostart: false, actionStop: true}
	for _, s := range steps {
		if s.Disruptive != disruptive[s.Action] {
			t.Errorf("%s disruptive = %t", s.Action, s.Disruptive)
		}
	}

	raise := planSteps(false, []desiredspec.Difference{{Class: classConfiguration, Field: fieldMemoryMax, Expected: "1024", Actual: "512"}})
	if raise[0].Disruptive {
		t.Error("raising memory.max is not disruptive")
	}
	start := planSteps(false, []desiredspec.Difference{{Class: classState, Field: fieldPower, Expected: "running", Actual: "stopped"}})
	if start[0].Action != actionStart || start[0].Disruptive || start[0].Manual {
		t.Errorf("start step = %+v", start[0])
	}
	thaw := planSteps(false, []desiredspec.Difference{{Class: classState, Field: fieldPower, Expected: "running", Actual: "frozen"}})
	if thaw[0].Action != actionUnfreeze {
		t.Errorf("frozen → running should thaw, got %+v", thaw[0])
	}
	template := planSteps(true, []desiredspec.Difference{{Class: classState, Field: fieldPower, Expected: "running", Actual: "stopped"}})
	if !template[0].Manual || template[0].Reason == "" {
		t.Errorf("starting a template must be manual: %+v", template[0])
	}
}

type fakeDesiredActions struct {
	calls  []string
	limits Limits
	update UpdateRequest
}

func (f *fakeDesiredActions) syncLiveState(context.Context, Container) error {
	f.calls = append(f.calls, "sync")
	return nil
}
func (f *fakeDesiredActions) Start(context.Context, string) error {
	f.calls = append(f.calls, "Start")
	return nil
}
func (f *fakeDesiredActions) Shutdown(context.Context, string, int) (bool, error) {
	f.calls = append(f.calls, "Shutdown")
	return false, nil
}
func (f *fakeDesiredActions) Unfreeze(context.Context, string) error {
	f.calls = append(f.calls, "Unfreeze")
	return nil
}
func (f *fakeDesiredActions) UpdateLimits(_ context.Context, _ string, l Limits) (LimitsResponse, error) {
	f.calls = append(f.calls, "UpdateLimits")
	f.limits = l
	return LimitsResponse{}, nil
}
func (f *fakeDesiredActions) Update(_ context.Context, _ string, req UpdateRequest) (Container, error) {
	f.calls = append(f.calls, "Update")
	f.update = req
	return Container{}, nil
}

func TestDesiredApplyStepDispatch(t *testing.T) {
	ctx := context.Background()
	spec := map[string]string{fieldPower: "running", fieldCPUMax: "max", fieldMemoryMax: "256", fieldAutostart: "no"}
	cases := map[string][]string{
		actionStart:        {"sync", "Start"},
		actionStop:         {"sync", "Shutdown"},
		actionUnfreeze:     {"sync", "Unfreeze"},
		actionSetCPUMax:    {"UpdateLimits"},
		actionSetMemoryMax: {"UpdateLimits"},
		actionSetAutostart: {"Update"},
	}
	for action, want := range cases {
		fake := &fakeDesiredActions{}
		if err := applyDesiredStep(ctx, fake, desiredFixture(), spec, desiredspec.Step{Action: action}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !reflect.DeepEqual(fake.calls, want) {
			t.Errorf("%s called %v, want %v", action, fake.calls, want)
		}
	}

	fake := &fakeDesiredActions{}
	if err := applyDesiredStep(ctx, fake, desiredFixture(), spec, desiredspec.Step{Action: actionSetMemoryMax}); err != nil {
		t.Fatal(err)
	}
	if fake.limits.MemoryMaxBytes != 256*mebibyte || fake.limits.MemoryHighBytes != 0 ||
		fake.limits.CPUMax != "150000 100000" || fake.limits.PidsMax != 4096 {
		t.Errorf("memory step moved the wrong limits: %+v", fake.limits)
	}

	fake = &fakeDesiredActions{}
	if err := applyDesiredStep(ctx, fake, desiredFixture(), spec, desiredspec.Step{Action: actionSetCPUMax}); err != nil {
		t.Fatal(err)
	}
	if fake.limits.CPUMax != "" || fake.limits.MemoryMaxBytes != 512*mebibyte {
		t.Errorf("cpu step moved the wrong limits: %+v", fake.limits)
	}

	fake = &fakeDesiredActions{}
	if err := applyDesiredStep(ctx, fake, desiredFixture(), spec, desiredspec.Step{Action: actionSetAutostart}); err != nil {
		t.Fatal(err)
	}
	if fake.update.Autostart == nil || *fake.update.Autostart || fake.update.Name != nil {
		t.Errorf("autostart step sent %+v", fake.update)
	}

	for _, bad := range []desiredspec.Step{{Action: "delete"}, {Action: actionSetCPUMax}} {
		fake = &fakeDesiredActions{}
		if err := applyDesiredStep(ctx, fake, desiredFixture(), map[string]string{}, bad); err == nil || len(fake.calls) != 0 {
			t.Errorf("%s with an empty spec: err=%v calls=%v", bad.Action, err, fake.calls)
		}
	}
}

func TestDesiredWithMemoryMaxKeepsCustomHigh(t *testing.T) {
	limits := Limits{MemoryMaxBytes: 1024 * mebibyte, MemoryHighBytes: 700 * mebibyte}
	if got := withMemoryMax(limits, 2048*mebibyte); got.MemoryHighBytes != 700*mebibyte {
		t.Errorf("a custom memory.high below the new limit must stay: %+v", got)
	}
	if got := withMemoryMax(limits, 512*mebibyte); got.MemoryHighBytes != 0 {
		t.Errorf("memory.high above the new limit must reset: %+v", got)
	}
}
