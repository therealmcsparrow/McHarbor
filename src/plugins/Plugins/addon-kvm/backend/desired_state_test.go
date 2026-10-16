// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

func desiredTestVM(state string) VM {
	return VM{
		ID: "vm1", EnvironmentID: "env1", Name: "db", State: state, HostAvailable: true,
		VCPUs: 4, MemoryMB: 4096, Autostart: true, CPUModel: "host",
	}
}

func driftFields(diffs []desiredstate.Difference) []string {
	out := make([]string, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, d.Field)
	}
	return out
}

func requireStrings(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func stepActions(steps []desiredstate.Step) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Action)
	}
	return out
}

func TestDesiredEquivalentValuesAreNotDrift(t *testing.T) {
	vm := desiredTestVM(StateRunning)
	for _, spec := range []map[string]string{
		{desiredMemoryMiB: "4 GiB"}, {desiredMemoryMiB: "4096"}, {desiredMemoryMiB: "4096.0"},
		{desiredMemoryMiB: "4096 MiB"}, {desiredMemoryMiB: " 4g "},
		{desiredVCPUs: "4.0"}, {desiredVCPUs: " 4 "},
		{desiredAutostart: "yes"}, {desiredAutostart: "TRUE"}, {desiredAutostart: "1"},
		{desiredPower: "Running"}, {desiredPower: "on"},
		{desiredCPUModel: "Host"},
		{desiredPower: "   "},
	} {
		diffs, err := detectVMDrift(vm, spec)
		if err != nil || len(diffs) != 0 {
			t.Fatalf("spec %v reported %v, %v", spec, diffs, err)
		}
	}
}

func TestDesiredEmptySpecIsNoDrift(t *testing.T) {
	diffs, err := detectVMDrift(desiredTestVM(StateStopped), map[string]string{})
	if err != nil || len(diffs) != 0 {
		t.Fatalf("empty spec reported %v, %v", diffs, err)
	}
}

func TestDesiredDriftPerField(t *testing.T) {
	vm := desiredTestVM(StateRunning)
	vm.MemoryCurrentMB = 6144
	diffs, err := detectVMDrift(vm, map[string]string{
		desiredPower: "stopped", desiredVCPUs: "8", desiredMemoryMiB: "4096",
		desiredAutostart: "false", desiredCPUModel: "EPYC",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireStrings(t, driftFields(diffs), desiredPower, desiredVCPUs, desiredMemoryMiB, desiredCPUModel, desiredAutostart)
	if diffs[0].Class != driftState || diffs[1].Class != driftConfiguration {
		t.Fatalf("classes = %s, %s", diffs[0].Class, diffs[1].Class)
	}
	if diffs[2].Actual != "6144" {
		t.Fatalf("a running guest's memory is what it holds now, got %q", diffs[2].Actual)
	}
}

func TestDesiredCrashedReadsStopped(t *testing.T) {
	diffs, err := detectVMDrift(desiredTestVM(StateCrashed), map[string]string{desiredPower: "stopped"})
	if err != nil || len(diffs) != 0 {
		t.Fatalf("crashed pinned stopped reported %v, %v", diffs, err)
	}
}

func TestDesiredTransitionIsAnError(t *testing.T) {
	if _, err := detectVMDrift(desiredTestVM(StateStopping), map[string]string{desiredPower: "stopped"}); !errors.Is(err, errDesiredTransition) {
		t.Fatalf("err = %v, want a transition error", err)
	}
}

func TestDesiredPlanStopBeforeColdChange(t *testing.T) {
	vm := desiredTestVM(StateRunning)
	spec := map[string]string{desiredPower: "stopped", desiredMemoryMiB: "8192", desiredCPUModel: "EPYC"}
	diffs, _ := detectVMDrift(vm, spec)
	steps := planVMDrift(vm, spec, diffs)
	requireStrings(t, stepActions(steps), desiredActionStop, desiredActionMemory, desiredActionCPUModel)
	if !steps[0].Disruptive || steps[0].Manual {
		t.Fatalf("stop = %+v, want automatic and disruptive", steps[0])
	}
	for _, step := range steps[1:] {
		if step.Manual || step.Disruptive {
			t.Fatalf("%s after a stop is automatic and not disruptive: %+v", step.Action, step)
		}
	}
}

func TestDesiredPlanColdChangeBeforeStart(t *testing.T) {
	vm := desiredTestVM(StateStopped)
	spec := map[string]string{desiredPower: "running", desiredVCPUs: "16"}
	diffs, _ := detectVMDrift(vm, spec)
	steps := planVMDrift(vm, spec, diffs)
	requireStrings(t, stepActions(steps), desiredActionVCPUs, desiredActionStart)
	if steps[0].Manual || steps[1].Manual || steps[1].Disruptive {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestDesiredPlanLiveAndManual(t *testing.T) {
	vm := desiredTestVM(StateRunning)
	vm.VCPUsMax = 8

	within := map[string]string{desiredVCPUs: "6"}
	steps := planVMDrift(vm, within, mustDrift(t, vm, within))
	if len(steps) != 1 || steps[0].Manual || steps[0].Disruptive {
		t.Fatalf("hot-plug within the ceiling = %+v", steps)
	}

	beyond := map[string]string{desiredVCPUs: "12", desiredMemoryMiB: "8192", desiredCPUModel: "EPYC"}
	steps = planVMDrift(vm, beyond, mustDrift(t, vm, beyond))
	requireStrings(t, stepActions(steps), desiredActionVCPUs, desiredActionMemory, desiredActionCPUModel)
	for _, step := range steps {
		if !step.Manual || step.Reason == "" {
			t.Fatalf("%s on a running guest without a live path is manual with a reason: %+v", step.Action, step)
		}
	}

	vm.VirtioMem, vm.MemoryMaxMB = true, 16384
	resizable := map[string]string{desiredMemoryMiB: "8 GiB"}
	steps = planVMDrift(vm, resizable, mustDrift(t, vm, resizable))
	if len(steps) != 1 || steps[0].Manual {
		t.Fatalf("virtio-mem inside the ceiling = %+v", steps)
	}

	autostart := map[string]string{desiredAutostart: "false"}
	steps = planVMDrift(vm, autostart, mustDrift(t, vm, autostart))
	if len(steps) != 1 || steps[0].Manual || steps[0].Action != desiredActionAutostart {
		t.Fatalf("autostart = %+v", steps)
	}
}

func TestDesiredPlanPausedCases(t *testing.T) {
	vm := desiredTestVM(StatePaused)
	run := map[string]string{desiredPower: "running"}
	steps := planVMDrift(vm, run, mustDrift(t, vm, run))
	if len(steps) != 1 || steps[0].Action != desiredActionResume || steps[0].Manual {
		t.Fatalf("resume = %+v", steps)
	}
	vm.RunState = RunStateIOError
	steps = planVMDrift(vm, run, mustDrift(t, vm, run))
	if len(steps) != 1 || !steps[0].Manual {
		t.Fatalf("an io-error pause is not resumed automatically: %+v", steps)
	}
}

func TestDesiredPlanInvalidSpecIsManual(t *testing.T) {
	vm := desiredTestVM(StateRunning)
	spec := map[string]string{desiredMemoryMiB: "lots", desiredPower: "stopped"}
	steps := planVMDrift(vm, spec, mustDrift(t, vm, spec))
	requireStrings(t, stepActions(steps), desiredActionFixSpec, desiredActionStop)
	if !steps[0].Manual {
		t.Fatalf("an invalid pinned value is a manual fix: %+v", steps[0])
	}
}

func mustDrift(t *testing.T, vm VM, spec map[string]string) []desiredstate.Difference {
	t.Helper()
	diffs, err := detectVMDrift(vm, spec)
	if err != nil {
		t.Fatal(err)
	}
	return diffs
}

func desiredPlugin(t *testing.T) (*KVMPlugin, *fakeGuestRPC, *sql.DB) {
	t.Helper()
	svc, rpc, db := newIntentTestService(t)
	return &KVMPlugin{svc: svc, logger: quietLogger{}}, rpc, db
}

func TestDesiredExistenceAndUnreachable(t *testing.T) {
	p, rpc, db := desiredPlugin(t)
	ctx := context.Background()

	diffs, err := p.DetectDrift(ctx, desiredKindVM, "env1", "missing", map[string]string{desiredPower: "running"})
	if err != nil || len(diffs) != 1 || diffs[0].Class != driftExistence || diffs[0].Expected != "exists" || diffs[0].Actual != "missing" {
		t.Fatalf("missing guest = %+v, %v", diffs, err)
	}
	steps, err := p.PlanDrift(ctx, desiredKindVM, "env1", "missing", nil, diffs)
	if err != nil || len(steps) != 1 || !steps[0].Manual {
		t.Fatalf("existence plan = %+v, %v", steps, err)
	}

	insertIntentTestVM(t, db, "vm1", StateRunning)
	if diffs, err := p.DetectDrift(ctx, desiredKindVM, "other-env", "vm1", map[string]string{desiredPower: "running"}); err != nil || len(diffs) != 1 || diffs[0].Class != driftExistence {
		t.Fatalf("a guest on another environment is missing here: %+v, %v", diffs, err)
	}

	rpc.down = true
	if _, err := p.DetectDrift(ctx, desiredKindVM, "env1", "vm1", map[string]string{desiredPower: "stopped"}); !errors.Is(err, ErrHostUnavailable) {
		t.Fatalf("an unreachable host is an error, not drift: %v", err)
	}
}

func TestDesiredApplyStepReachesService(t *testing.T) {
	cases := []struct {
		name    string
		state   string
		spec    map[string]string
		action  string
		reply   string
		message string
		intent  string
	}{
		{"start", StateStopped, map[string]string{desiredPower: "running"}, desiredActionStart, "", msgVMStart, intentStart},
		{"stop", StateRunning, map[string]string{desiredPower: "stopped"}, desiredActionStop, "", msgVMShutdown, intentShutdown},
		{"resume", StatePaused, map[string]string{desiredPower: "running"}, desiredActionResume, "", msgVMResume, intentResume},
		{"live vcpus", StateRunning, map[string]string{desiredVCPUs: "1"}, desiredActionVCPUs, `{"resize":{"actualVcpus":1,"complied":true}}`, msgVMCPUSet, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, rpc, db := desiredPlugin(t)
			insertIntentTestVM(t, db, "vm1", tc.state)
			if tc.reply != "" {
				rpc.reply = []byte(tc.reply)
			}
			err := p.ApplyStep(context.Background(), desiredKindVM, "env1", "vm1", tc.spec, desiredstate.Step{Action: tc.action})
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if len(rpc.calls) == 0 || rpc.calls[len(rpc.calls)-1] != tc.message {
				t.Fatalf("agent calls = %v, want %s", rpc.calls, tc.message)
			}
			if tc.intent == "" {
				return
			}
			rows := intentRows(t, db, "vm1")
			want := intentRow{action: tc.intent, actor: desiredActor, source: desiredActor, outcome: intentOK}
			if len(rows) != 1 || rows[0] != want {
				t.Fatalf("intents = %+v, want %+v", rows, want)
			}
		})
	}
}

func TestDesiredApplyStepSettingsOnStoppedGuest(t *testing.T) {
	p, rpc, db := desiredPlugin(t)
	insertIntentTestVM(t, db, "vm1", StateStopped)
	ctx := context.Background()
	spec := map[string]string{desiredVCPUs: "3", desiredMemoryMiB: "3 GiB", desiredAutostart: "yes", desiredCPUModel: " host "}
	for _, action := range []string{desiredActionVCPUs, desiredActionMemory, desiredActionAutostart, desiredActionCPUModel} {
		if err := p.ApplyStep(ctx, desiredKindVM, "env1", "vm1", spec, desiredstate.Step{Action: action}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("settings on a stopped guest need no agent call, got %v", rpc.calls)
	}
	var vcpus, memory, autostart int
	var cpuModel string
	if err := db.QueryRow(`SELECT vcpus, memory_mb, autostart, cpu_model FROM plugin_addon_kvm_vms WHERE id = 'vm1'`).
		Scan(&vcpus, &memory, &autostart, &cpuModel); err != nil {
		t.Fatal(err)
	}
	if vcpus != 3 || memory != 3072 || autostart != 1 || cpuModel != "host" {
		t.Fatalf("stored vcpus=%d memory=%d autostart=%d cpu=%q", vcpus, memory, autostart, cpuModel)
	}
	after, err := p.DetectDrift(ctx, desiredKindVM, "env1", "vm1", spec)
	if err != nil || len(after) != 0 {
		t.Fatalf("drift after applying = %+v, %v", after, err)
	}
}

func TestDesiredApplyStepRefusals(t *testing.T) {
	p, rpc, db := desiredPlugin(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	ctx := context.Background()

	if err := p.ApplyStep(ctx, desiredKindVM, "env1", "vm1", nil, desiredstate.Step{Action: desiredActionStop, Manual: true}); err == nil {
		t.Fatal("a manual step must be refused")
	}
	if err := p.ApplyStep(ctx, desiredKindVM, "env1", "vm1", map[string]string{desiredMemoryMiB: "8192"}, desiredstate.Step{Action: desiredActionMemory}); !errors.Is(err, ErrNoResizableMemory) {
		t.Fatalf("live memory without virtio-mem = %v", err)
	}
	rpc.reply = []byte(`{"resize":{"actualVcpus":2,"complied":false}}`)
	if err := p.ApplyStep(ctx, desiredKindVM, "env1", "vm1", map[string]string{desiredVCPUs: "1"}, desiredstate.Step{Action: desiredActionVCPUs}); err == nil {
		t.Fatal("a guest that declined the unplug must be an error")
	}
	if len(intentRows(t, db, "vm1")) != 0 {
		t.Fatal("refused steps must not record lifecycle actions")
	}
}
