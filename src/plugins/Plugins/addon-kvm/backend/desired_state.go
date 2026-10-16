// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

// desiredKindVM is the one kind this plugin holds to a desired state.
const desiredKindVM = "virtual_machine"

// Fields a virtual machine's desired state can pin.
const (
	desiredPower     = "power"
	desiredVCPUs     = "vcpus"
	desiredMemoryMiB = "memory_mib"
	desiredAutostart = "autostart"
	desiredCPUModel  = "cpu_model"
)

// Power values a spec may pin.
const (
	powerRunning = "running"
	powerStopped = "stopped"
	powerPaused  = "paused"
)

// Plan actions, handed back to ApplyStep unchanged.
const (
	desiredActionStart     = "start"
	desiredActionStop      = "stop"
	desiredActionResume    = "resume"
	desiredActionVCPUs     = "set_vcpus"
	desiredActionMemory    = "set_memory"
	desiredActionAutostart = "set_autostart"
	desiredActionCPUModel  = "set_cpu_model"
	desiredActionRecreate  = "recreate"
	desiredActionFixSpec   = "fix_spec"
)

// Difference classes the reconciler reports.
const (
	driftState         = "state"
	driftConfiguration = "configuration"
	driftExistence     = "existence"
)

// desiredActor is who the lifecycle-intent log names for reconciler actions.
const desiredActor = "reconciler"

// desiredShutdownGrace is the ACPI window a reconciler stop gives the guest,
// the same default the workflow node uses before escalating.
const desiredShutdownGrace = 90

// desiredFieldOrder fixes the order differences and steps come out in.
var desiredFieldOrder = []string{desiredPower, desiredVCPUs, desiredMemoryMiB, desiredCPUModel, desiredAutostart}

// errDesiredTransition means the guest is between states; the host keeps its
// last report rather than calling a shutdown in progress drift.
var errDesiredTransition = errors.New("the virtual machine is changing state")

// DesiredKinds declares virtual machines. Machine type is left out on purpose:
// changing it is the upgrade the plugin deliberately never makes silently.
func (p *KVMPlugin) DesiredKinds() []desiredstate.Kind {
	return []desiredstate.Kind{{
		Kind:         desiredKindVM,
		Label:        "Virtual machine",
		Description:  "Power state, processors, memory, CPU model and autostart of a KVM guest.",
		SubjectLabel: "Virtual machine",
		Enforceable:  true,
		Fields: []desiredstate.Field{
			{
				Key: desiredPower, Label: "Power", Type: desiredstate.FieldSelect,
				Description: "Whether the guest should be running.",
				Options: []desiredstate.Option{
					{Value: powerRunning, Label: "Running"},
					{Value: powerStopped, Label: "Stopped"},
				},
			},
			{
				Key: desiredVCPUs, Label: "Virtual CPUs", Type: desiredstate.FieldNumber, Min: 1, Max: 512,
				Description: "Changed live up to the guest's hot-plug ceiling; beyond it the guest must be restarted.",
			},
			{
				Key: desiredMemoryMiB, Label: "Memory", Type: desiredstate.FieldNumber, Unit: "MiB", Min: 128,
				Description: "Changed live only on a guest with resizable (virtio-mem) memory, between its boot size and ceiling.",
			},
			{
				Key: desiredCPUModel, Label: "CPU model", Type: desiredstate.FieldText,
				Description: "Applied only while the guest is stopped.",
			},
			{
				Key: desiredAutostart, Label: "Start with the host", Type: desiredstate.FieldBool,
			},
		},
	}}
}

// DesiredSubjects lists the non-template guests on one environment.
func (p *KVMPlugin) DesiredSubjects(ctx context.Context, kind, environmentID string) ([]desiredstate.Option, error) {
	if p.svc == nil || kind != desiredKindVM {
		return nil, nil
	}
	guests, err := p.svc.List(ctx, environmentID, false)
	if err != nil {
		return nil, fmt.Errorf("kvm: listing guests for desired state: %w", err)
	}
	out := make([]desiredstate.Option, 0, len(guests))
	for _, vm := range guests {
		out = append(out, desiredstate.Option{
			Value:  vm.ID,
			Label:  vm.Name,
			Detail: fmt.Sprintf("%s · %d vCPU · %d MiB", vm.State, vm.VCPUs, currentMemoryMiB(vm)),
		})
	}
	return out, nil
}

// DesiredOptions has nothing to offer: no virtual machine field is dynamic.
func (p *KVMPlugin) DesiredOptions(context.Context, string, string, string) ([]desiredstate.Option, error) {
	return nil, nil
}

// CurrentSpec reports the guest as a spec, formatted exactly as DetectDrift
// compares; a transitional power state is left out rather than pinned.
func (p *KVMPlugin) CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error) {
	vm, err := p.desiredVM(ctx, kind, environmentID, subjectID)
	if err != nil {
		return nil, err
	}
	return currentDesiredSpec(vm), nil
}

// DetectDrift compares the pinned fields with the guest. A missing guest is
// existence drift; an unreachable host or a guest mid-transition is an error.
func (p *KVMPlugin) DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]desiredstate.Difference, error) {
	vm, err := p.desiredVM(ctx, kind, environmentID, subjectID)
	if errors.Is(err, ErrNotFound) {
		return []desiredstate.Difference{existenceDifference()}, nil
	}
	if err != nil {
		return nil, err
	}
	return detectVMDrift(vm, spec)
}

// PlanDrift orders the steps that would fix the differences against the guest
// as it is now; ApplyStep re-checks every one against the guest as it is then.
func (p *KVMPlugin) PlanDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, diffs []desiredstate.Difference) ([]desiredstate.Step, error) {
	for _, diff := range diffs {
		if diff.Class == driftExistence {
			return []desiredstate.Step{recreateStep()}, nil
		}
	}
	vm, err := p.desiredVM(ctx, kind, environmentID, subjectID)
	if errors.Is(err, ErrNotFound) {
		return []desiredstate.Step{recreateStep()}, nil
	}
	if err != nil {
		return nil, err
	}
	return planVMDrift(vm, spec, diffs), nil
}

// ApplyStep runs one approved step through the service, so validation, the
// lifecycle-intent log (actor "reconciler") and the audit trail all apply.
func (p *KVMPlugin) ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step desiredstate.Step) error {
	if step.Manual {
		return fmt.Errorf("kvm: %q is a manual step and is never applied automatically", step.Action)
	}
	vm, err := p.desiredVM(ctx, kind, environmentID, subjectID)
	if err != nil {
		return err
	}
	ctx = withIntentOrigin(ctx, desiredActor, desiredActor)
	if err := p.svc.applyDesiredStep(ctx, vm, spec, step.Action); err != nil {
		return fmt.Errorf("kvm: %s on %s: %w", step.Action, vm.Name, err)
	}
	p.svc.recordAudit("plugin.kvm.vm.desired_state_applied", map[string]any{
		"vmId": vm.ID, "name": vm.Name, "environmentId": vm.EnvironmentID,
		"action": step.Action, "source": desiredActor,
	})
	return nil
}

// desiredVM reads one guest with a single row lookup, and fails when its host is
// unreachable so the host never reads an agent outage as drift.
func (p *KVMPlugin) desiredVM(ctx context.Context, kind, environmentID, subjectID string) (VM, error) {
	if p.svc == nil {
		return VM{}, ErrHostUnavailable
	}
	if kind != desiredKindVM {
		return VM{}, fmt.Errorf("kvm: %q is not a kind this plugin manages", kind)
	}
	vm, err := p.svc.lightVM(ctx, subjectID)
	if err != nil {
		return VM{}, err
	}
	if vm.IsTemplate || (environmentID != "" && vm.EnvironmentID != environmentID) {
		return VM{}, ErrNotFound
	}
	if !vm.HostAvailable {
		return VM{}, fmt.Errorf("%w: %s", ErrHostUnavailable, vm.UnavailableReason)
	}
	return vm, nil
}

// lightVM reads a guest's row without its disks, interfaces and snapshots,
// which is all drift detection needs once a minute.
func (s *Service) lightVM(ctx context.Context, id string) (VM, error) {
	rows, err := s.db.Query(ctx, `SELECT `+vmColumns+` FROM plugin_addon_kvm_vms WHERE id = ?`, id)
	if err != nil {
		return VM{}, fmt.Errorf("reading virtual machine: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return VM{}, err
		}
		return VM{}, ErrNotFound
	}
	vm, err := scanVM(rows)
	if err != nil {
		return VM{}, err
	}
	s.applyAvailability(ctx, &vm)
	return vm, nil
}

// applyDesiredStep dispatches one action to the service method that owns it,
// choosing the live or the stopped path from the guest's state right now.
func (s *Service) applyDesiredStep(ctx context.Context, vm VM, spec map[string]string, action string) error {
	switch action {
	case desiredActionStart:
		return s.Start(ctx, vm.ID)
	case desiredActionStop:
		_, err := s.Shutdown(ctx, vm.ID, desiredShutdownGrace)
		return err
	case desiredActionResume:
		return s.Resume(ctx, vm.ID)
	case desiredActionAutostart:
		value, ok := parseDesiredBool(spec[desiredAutostart])
		if !ok {
			return fmt.Errorf("%q is not a yes/no value", spec[desiredAutostart])
		}
		_, err := s.Update(ctx, vm.ID, UpdateRequest{Autostart: &value})
		return err
	case desiredActionCPUModel:
		value := strings.TrimSpace(spec[desiredCPUModel])
		if vm.State != StateStopped {
			return ErrRunning
		}
		_, err := s.Update(ctx, vm.ID, UpdateRequest{CPUModel: &value})
		return err
	case desiredActionVCPUs:
		target, ok := parseDesiredCount(spec[desiredVCPUs])
		if !ok {
			return fmt.Errorf("%q is not a processor count", spec[desiredVCPUs])
		}
		if vm.State == StateStopped {
			value := int(target)
			_, err := s.Update(ctx, vm.ID, UpdateRequest{VCPUs: &value})
			return err
		}
		result, err := s.SetVCPUs(ctx, vm.ID, int(target))
		if err != nil {
			return err
		}
		if result == nil || int64(result.ActualVCPUs) != target {
			return fmt.Errorf("the guest holds %d processors after asking for %d", actualVCPUs(result), target)
		}
		return nil
	case desiredActionMemory:
		target, ok := parseMemoryMiB(spec[desiredMemoryMiB])
		if !ok {
			return fmt.Errorf("%q is not a memory size", spec[desiredMemoryMiB])
		}
		if vm.State == StateStopped {
			_, err := s.Update(ctx, vm.ID, UpdateRequest{MemoryMB: &target})
			return err
		}
		result, err := s.ResizeMemory(ctx, vm.ID, target)
		if err != nil {
			return err
		}
		if result == nil || result.ActualMemoryMB != target {
			return fmt.Errorf("the guest holds %d MiB after asking for %d MiB", actualMemory(result), target)
		}
		return nil
	}
	return fmt.Errorf("%q is not an action this plugin applies", action)
}

func actualVCPUs(result *ResizeResult) int {
	if result == nil {
		return 0
	}
	return result.ActualVCPUs
}

func actualMemory(result *ResizeResult) int64 {
	if result == nil {
		return 0
	}
	return result.ActualMemoryMB
}

// currentDesiredSpec formats a guest's current values as DetectDrift compares
// them. A guest mid-transition has no power value to offer.
func currentDesiredSpec(vm VM) map[string]string {
	spec := map[string]string{
		desiredVCPUs:     strconv.Itoa(vm.VCPUs),
		desiredMemoryMiB: strconv.FormatInt(currentMemoryMiB(vm), 10),
		desiredAutostart: strconv.FormatBool(vm.Autostart),
		desiredCPUModel:  normalizeCPUModel(vm.CPUModel),
	}
	if power, ok := powerOf(vm); ok {
		spec[desiredPower] = power
	}
	return spec
}

// currentMemoryMiB is what the guest holds: after a live virtio-mem resize the
// running guest differs from its boot size, and the boot size is what a stop restores.
func currentMemoryMiB(vm VM) int64 {
	if vm.State == StateRunning && vm.MemoryCurrentMB > 0 {
		return vm.MemoryCurrentMB
	}
	return vm.MemoryMB
}

// powerOf maps a stored state onto a spec power value; a crashed or defined guest
// is not running, so it reads stopped. Transitional states have no value.
func powerOf(vm VM) (string, bool) {
	switch vm.State {
	case StateRunning:
		return powerRunning, true
	case StatePaused:
		return powerPaused, true
	case StateStopped, StateDefined, StateCrashed:
		return powerStopped, true
	}
	return "", false
}

// detectVMDrift compares every pinned field with the guest. An unparseable
// pinned value is reported as drift so the plan can say it needs fixing.
func detectVMDrift(vm VM, spec map[string]string) ([]desiredstate.Difference, error) {
	var out []desiredstate.Difference
	for _, field := range desiredFieldOrder {
		raw := strings.TrimSpace(spec[field])
		if raw == "" {
			continue
		}
		diff, drifted, err := compareDesiredField(vm, field, raw)
		if err != nil {
			return nil, err
		}
		if drifted {
			out = append(out, diff)
		}
	}
	return out, nil
}

// compareDesiredField normalises one pinned value and the guest's, and reports
// a difference only when they genuinely disagree.
func compareDesiredField(vm VM, field, raw string) (desiredstate.Difference, bool, error) {
	current := currentDesiredSpec(vm)
	switch field {
	case desiredPower:
		actual, ok := powerOf(vm)
		if !ok {
			return desiredstate.Difference{}, false, fmt.Errorf("%w: it is %s", errDesiredTransition, vm.State)
		}
		expected, valid := normalizePower(raw)
		if !valid || expected != actual {
			return desiredstate.Difference{Class: driftState, Field: field, Expected: raw, Actual: actual}, true, nil
		}
	case desiredVCPUs:
		expected, valid := parseDesiredCount(raw)
		if !valid || expected != int64(vm.VCPUs) {
			return configDiff(field, raw, current[field]), true, nil
		}
	case desiredMemoryMiB:
		expected, valid := parseMemoryMiB(raw)
		if !valid || expected != currentMemoryMiB(vm) {
			return configDiff(field, raw, current[field]), true, nil
		}
	case desiredAutostart:
		expected, valid := parseDesiredBool(raw)
		if !valid || expected != vm.Autostart {
			return configDiff(field, raw, current[field]), true, nil
		}
	case desiredCPUModel:
		if normalizeCPUModel(raw) != normalizeCPUModel(vm.CPUModel) {
			return configDiff(field, raw, current[field]), true, nil
		}
	}
	return desiredstate.Difference{}, false, nil
}

func configDiff(field, expected, actual string) desiredstate.Difference {
	return desiredstate.Difference{Class: driftConfiguration, Field: field, Expected: expected, Actual: actual}
}

func existenceDifference() desiredstate.Difference {
	return desiredstate.Difference{Class: driftExistence, Field: "existence", Expected: "exists", Actual: "missing"}
}

func recreateStep() desiredstate.Step {
	return desiredstate.Step{
		Action: desiredActionRecreate, Description: "Recreate the virtual machine",
		Fields: []string{"existence"}, Manual: true,
		Reason: "The virtual machine no longer exists; recreate it, then point this desired state at the new one.",
	}
}

// planVMDrift orders the fix: a stop comes before settings that need the guest
// stopped, and settings a stopped guest needs are applied before it starts.
func planVMDrift(vm VM, spec map[string]string, diffs []desiredstate.Difference) []desiredstate.Step {
	drifted := map[string]bool{}
	for _, diff := range diffs {
		drifted[diff.Field] = true
	}

	var invalid []desiredstate.Step
	for _, field := range desiredFieldOrder {
		if drifted[field] && !validDesiredValue(field, spec[field]) {
			invalid = append(invalid, desiredstate.Step{
				Action: desiredActionFixSpec, Description: "Correct the pinned " + field,
				Fields: []string{field}, Manual: true,
				Reason: fmt.Sprintf("%q is not a valid value for %s.", spec[field], field),
			})
			drifted[field] = false
		}
	}

	var before, after []desiredstate.Step
	applyState := vm.State
	if drifted[desiredPower] {
		expected, _ := normalizePower(spec[desiredPower])
		actual, _ := powerOf(vm)
		switch {
		case expected == powerStopped && actual == powerRunning:
			before = append(before, desiredstate.Step{
				Action: desiredActionStop, Description: "Shut the guest down", Fields: []string{desiredPower},
				Disruptive: true,
			})
			applyState = StateStopped
		case expected == powerStopped && actual == powerPaused:
			before = append(before, desiredstate.Step{
				Action: desiredActionStop, Description: "Stop the paused guest", Fields: []string{desiredPower},
				Manual: true, Disruptive: true,
				Reason: "A paused guest cannot shut down cleanly; resume it or force it off yourself.",
			})
		case expected == powerRunning && actual == powerPaused && vm.RunState == RunStateIOError:
			before = append(before, desiredstate.Step{
				Action: desiredActionResume, Description: "Resume the guest", Fields: []string{desiredPower},
				Manual: true,
				Reason: "QEMU paused this guest on a disk I/O error; free space on its storage first, or a resume pauses it again.",
			})
		case expected == powerRunning && actual == powerPaused:
			before = append(before, desiredstate.Step{
				Action: desiredActionResume, Description: "Resume the guest", Fields: []string{desiredPower},
			})
			applyState = StateRunning
		case expected == powerRunning && actual == powerStopped:
			after = append(after, desiredstate.Step{
				Action: desiredActionStart, Description: "Start the guest", Fields: []string{desiredPower},
			})
		}
	}

	var config []desiredstate.Step
	for _, field := range desiredFieldOrder[1:] {
		if drifted[field] {
			config = append(config, configStep(vm, applyState, field, spec[field]))
		}
	}
	steps := append(append(append(invalid, before...), config...), after...)
	return steps
}

// configStep is the step for one setting, automatic when the guest will be in a
// state the service accepts the change in, manual with the reason otherwise.
func configStep(vm VM, state, field, raw string) desiredstate.Step {
	step := desiredstate.Step{Fields: []string{field}}
	switch field {
	case desiredAutostart:
		step.Action, step.Description = desiredActionAutostart, "Set start-with-host to "+raw
		return step
	case desiredCPUModel:
		step.Action, step.Description = desiredActionCPUModel, "Set the CPU model to "+strings.TrimSpace(raw)
		if state != StateStopped {
			step.Manual = true
			step.Reason = "The CPU model is read when the guest boots; stop it, or schedule a restart, and the change applies."
		}
		return step
	case desiredVCPUs:
		target, _ := parseDesiredCount(raw)
		step.Action, step.Description = desiredActionVCPUs, fmt.Sprintf("Set %d virtual CPUs", target)
		ceiling := vm.VCPUsMax
		if ceiling <= 0 {
			ceiling = vm.VCPUs
		}
		switch {
		case state == StateStopped:
		case state == StateRunning && target <= int64(ceiling):
			step.Description += " live"
		case state == StateRunning:
			step.Manual = true
			step.Reason = fmt.Sprintf("The guest was started with room for %d processors; more needs a restart.", ceiling)
		default:
			step.Manual = true
			step.Reason = "The guest is " + vm.State + "; processors change only while it runs or is stopped."
		}
		return step
	}

	target, _ := parseMemoryMiB(raw)
	step.Action, step.Description = desiredActionMemory, fmt.Sprintf("Set memory to %d MiB", target)
	switch {
	case state == StateStopped:
	case state == StateRunning && vm.VirtioMem && target >= vm.MemoryMB &&
		(vm.MemoryMaxMB == 0 || target <= vm.MemoryMaxMB):
		step.Description += " live"
	case state == StateRunning:
		step.Manual = true
		step.Reason = "Memory changes live only on a guest with resizable memory, between its boot size and its ceiling; this change needs a restart."
	default:
		step.Manual = true
		step.Reason = "The guest is " + vm.State + "; memory changes only while it runs or is stopped."
	}
	return step
}

// validDesiredValue reports whether a pinned value parses for its field.
func validDesiredValue(field, raw string) bool {
	switch field {
	case desiredPower:
		_, ok := normalizePower(raw)
		return ok
	case desiredVCPUs:
		_, ok := parseDesiredCount(raw)
		return ok
	case desiredMemoryMiB:
		_, ok := parseMemoryMiB(raw)
		return ok
	case desiredAutostart:
		_, ok := parseDesiredBool(raw)
		return ok
	}
	return strings.TrimSpace(raw) != ""
}

// normalizePower accepts the spellings a person types for on and off.
func normalizePower(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running", "run", "on", "up", "started", "start":
		return powerRunning, true
	case "stopped", "stop", "off", "down", "shutoff", "shut off", "halted":
		return powerStopped, true
	}
	return "", false
}

// parseDesiredCount reads a whole positive number, tolerating "4.0".
func parseDesiredCount(raw string) (int64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 1 || value != math.Trunc(value) || value > math.MaxInt32 {
		return 0, false
	}
	return int64(value), true
}

// parseMemoryMiB reads a memory size in MiB, accepting a unit suffix so "4 GiB",
// "4096", "4096 MiB" and "4096.0" all mean the same guest.
func parseMemoryMiB(raw string) (int64, bool) {
	text := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""))
	multiplier := 1.0
	for _, unit := range []struct {
		suffix string
		factor float64
	}{{"gib", 1024}, {"gb", 1024}, {"g", 1024}, {"mib", 1}, {"mb", 1}, {"m", 1}} {
		if strings.HasSuffix(text, unit.suffix) {
			text, multiplier = strings.TrimSuffix(text, unit.suffix), unit.factor
			break
		}
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	mib := value * multiplier
	if mib < 1 || mib != math.Trunc(mib) || mib > math.MaxInt32 {
		return 0, false
	}
	return int64(mib), true
}

// parseDesiredBool reads the yes/no spellings a form or a person produces.
func parseDesiredBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "on", "1", "enabled":
		return true, true
	case "false", "no", "off", "0", "disabled":
		return false, true
	}
	return false, false
}

// normalizeCPUModel is the comparison form of a CPU model: case-insensitive, so
// "Host" pinned against "host" is not drift; the pinned spelling is what is applied.
func normalizeCPUModel(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

var _ desiredstate.Provider = (*KVMPlugin)(nil)
