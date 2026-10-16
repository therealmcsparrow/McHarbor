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

	desiredspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

const (
	desiredKind = "system_container"

	fieldPower     = "power"
	fieldCPUMax    = "cpu_max"
	fieldMemoryMax = "memory_max"
	fieldAutostart = "autostart"
	fieldExistence = "existence"

	classState         = "state"
	classConfiguration = "configuration"
	classExistence     = "existence"

	powerRunning = "running"
	powerStopped = "stopped"
	powerFrozen  = "frozen"

	actionStart        = "start"
	actionStop         = "stop"
	actionUnfreeze     = "unfreeze"
	actionSetCPUMax    = "set_cpu_max"
	actionSetMemoryMax = "set_memory_max"
	actionSetAutostart = "set_autostart"
	actionRecreate     = "recreate"

	// defaultCPUPeriod is cgroup v2's own period, applied when cpu.max names only a
	// quota, so "50000" and "50000 100000" compare equal.
	defaultCPUPeriod = 100000

	mebibyte = int64(1024 * 1024)

	reconcilerActor = "reconciler"
)

// desiredFieldOrder fixes the order differences are reported in, so a report is
// stable from one scan to the next.
var desiredFieldOrder = []string{fieldPower, fieldCPUMax, fieldMemoryMax, fieldAutostart}

// desiredActions is the slice of Service a step runs through; Service satisfies it,
// and it keeps ApplyStep's dispatch testable without an agent.
type desiredActions interface {
	syncLiveState(ctx context.Context, ct Container) error
	Start(ctx context.Context, id string) error
	Shutdown(ctx context.Context, id string, graceSeconds int) (bool, error)
	Unfreeze(ctx context.Context, id string) error
	UpdateLimits(ctx context.Context, id string, requested Limits) (LimitsResponse, error)
	Update(ctx context.Context, id string, req UpdateRequest) (Container, error)
}

// DesiredKinds declares system containers with the four properties the plugin can both
// read and set; cpu_max is cgroup v2's own "quota period" text because that is what is stored.
func (p *LXCPlugin) DesiredKinds() []desiredspec.Kind {
	return []desiredspec.Kind{{
		Kind:         desiredKind,
		Label:        "System container",
		Description:  "Power state, CPU and memory limits, and autostart of an LXC system container.",
		SubjectLabel: "System container",
		Enforceable:  true,
		Fields: []desiredspec.Field{
			{
				Key: fieldPower, Label: "Power", Type: desiredspec.FieldSelect,
				Description: "Whether the container should be running or stopped.",
				Options: []desiredspec.Option{
					{Value: powerRunning, Label: "Running"},
					{Value: powerStopped, Label: "Stopped"},
				},
			},
			{
				Key: fieldCPUMax, Label: "CPU limit", Type: desiredspec.FieldText,
				Description: `cgroup v2 cpu.max: "max" for no limit, or quota and period in ` +
					`microseconds ("150000 100000" is 1.5 CPUs); a bare quota uses a 100000 period.`,
			},
			{
				Key: fieldMemoryMax, Label: "Memory limit", Type: desiredspec.FieldNumber,
				Description: "Hard memory limit (memory.max); 0 means no limit.",
				Unit:        "MiB", Min: 0,
			},
			{
				Key: fieldAutostart, Label: "Autostart", Type: desiredspec.FieldBool,
				Description: "Start the container when its host's agent starts.",
			},
		},
	}}
}

// DesiredSubjects lists the environment's containers from the plugin's own rows, so
// the picker works while a host is offline; templates are excluded because they never run.
func (p *LXCPlugin) DesiredSubjects(ctx context.Context, kind, environmentID string) ([]desiredspec.Option, error) {
	if err := checkDesiredKind(kind); err != nil {
		return nil, err
	}
	if p.svc == nil {
		return nil, nil
	}
	containers, err := p.svc.List(ctx, environmentID, false)
	if err != nil {
		return nil, err
	}
	out := make([]desiredspec.Option, 0, len(containers))
	for _, ct := range containers {
		detail := ct.State
		if ct.Limits.MemoryMaxBytes > 0 {
			detail += " · " + formatMiB(ct.Limits.MemoryMaxBytes) + " MiB"
		}
		out = append(out, desiredspec.Option{Value: ct.ID, Label: ct.Name, Detail: detail})
	}
	return out, nil
}

// DesiredOptions returns nothing: no system container field is a dynamic select.
func (p *LXCPlugin) DesiredOptions(_ context.Context, kind, _, _ string) ([]desiredspec.Option, error) {
	return nil, checkDesiredKind(kind)
}

// CurrentSpec returns every field in the form DetectDrift compares against. Power comes
// from the agent, because the stored state is only refreshed by McHarbor's own actions.
func (p *LXCPlugin) CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error) {
	if err := checkDesiredKind(kind); err != nil {
		return nil, err
	}
	ct, found, err := p.desiredSubject(ctx, environmentID, subjectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	power, err := p.livePower(ctx, ct)
	if err != nil {
		return nil, err
	}
	return currentValues(ct, power), nil
}

// DetectDrift compares only the pinned fields. It asks the agent only when power is
// pinned, and an unreachable host is an error rather than drift.
func (p *LXCPlugin) DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]desiredspec.Difference, error) {
	if err := checkDesiredKind(kind); err != nil {
		return nil, err
	}
	ct, found, err := p.desiredSubject(ctx, environmentID, subjectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return []desiredspec.Difference{existenceDifference()}, nil
	}

	power := ""
	if pinned(spec, fieldPower) {
		if power, err = p.livePower(ctx, ct); err != nil {
			return nil, err
		}
	} else if available, reason := p.svc.availability(ctx, ct.EnvironmentID); !available {
		return nil, fmt.Errorf("%w: %s", ErrHostUnavailable, reason)
	}
	return diffSpec(currentValues(ct, power), spec)
}

// PlanDrift turns differences into ordered steps: limits, then autostart, then power,
// so a container that is started comes up with its new limits already stored.
func (p *LXCPlugin) PlanDrift(ctx context.Context, kind, environmentID, subjectID string, _ map[string]string, diffs []desiredspec.Difference) ([]desiredspec.Step, error) {
	if err := checkDesiredKind(kind); err != nil {
		return nil, err
	}
	isTemplate := false
	if p.svc != nil {
		ct, found, err := p.desiredSubject(ctx, environmentID, subjectID)
		if err != nil {
			return nil, err
		}
		isTemplate = found && ct.IsTemplate
	}
	return planSteps(isTemplate, diffs), nil
}

// ApplyStep runs one approved step through the same service methods the plugin's
// routes use, then audits it with the reconciler as the actor.
func (p *LXCPlugin) ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step desiredspec.Step) error {
	if err := checkDesiredKind(kind); err != nil {
		return err
	}
	if step.Manual {
		return errors.New("a manual step cannot be applied automatically")
	}
	ct, found, err := p.desiredSubject(ctx, environmentID, subjectID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	if err := applyDesiredStep(ctx, p.svc, ct, spec, step); err != nil {
		return fmt.Errorf("%s on system container %s: %w", step.Action, ct.Name, err)
	}
	p.auditDesiredStep(ct, step)
	return nil
}

var _ desiredspec.Provider = (*LXCPlugin)(nil)

func checkDesiredKind(kind string) error {
	if kind != desiredKind {
		return fmt.Errorf("unknown desired-state kind %q", kind)
	}
	return nil
}

func (p *LXCPlugin) desiredSubject(ctx context.Context, environmentID, subjectID string) (Container, bool, error) {
	if p.svc == nil {
		return Container{}, false, errors.New("system containers are not initialised")
	}
	ct, found, err := p.svc.storedContainer(ctx, subjectID)
	if err != nil || !found || ct.EnvironmentID != environmentID {
		return Container{}, false, err
	}
	return ct, true, nil
}

// livePower asks the agent once for this container's state. Unknown is an error,
// never a guess, so an agent that cannot read the state produces no drift.
func (p *LXCPlugin) livePower(ctx context.Context, ct Container) (string, error) {
	states, err := p.svc.queryStates(ctx, ct.EnvironmentID, []string{ct.ID})
	if err != nil {
		return "", err
	}
	power, ok := powerOf(states[ct.ID])
	if !ok {
		return "", fmt.Errorf("%w: the host could not read the container's state", ErrHostUnavailable)
	}
	return power, nil
}

// powerOf folds an observed state into the power field. Frozen stays distinct: it
// holds memory but runs nothing, so it matches neither running nor stopped.
func powerOf(state string) (string, bool) {
	switch state {
	case StateRunning:
		return powerRunning, true
	case StateFrozen:
		return powerFrozen, true
	case StateStopped, StateDefined, StateCrashed:
		return powerStopped, true
	}
	return "", false
}

func currentValues(ct Container, power string) map[string]string {
	cpu, err := normalizeCPUMax(ct.Limits.CPUMax)
	if err != nil {
		cpu = ct.Limits.CPUMax
	}
	out := map[string]string{
		fieldCPUMax:    cpu,
		fieldMemoryMax: formatMiB(ct.Limits.MemoryMaxBytes),
		fieldAutostart: strconv.FormatBool(ct.Autostart),
	}
	if power != "" {
		out[fieldPower] = power
	}
	return out
}

func pinned(spec map[string]string, key string) bool {
	return strings.TrimSpace(spec[key]) != ""
}

func existenceDifference() desiredspec.Difference {
	return desiredspec.Difference{Class: classExistence, Field: fieldExistence, Expected: "exists", Actual: "missing"}
}

// diffSpec reports one difference per pinned field whose normalised value differs.
// A pinned value that cannot be parsed is an error, not drift.
func diffSpec(current, spec map[string]string) ([]desiredspec.Difference, error) {
	var out []desiredspec.Difference
	for _, key := range desiredFieldOrder {
		if !pinned(spec, key) {
			continue
		}
		want, err := normalizeField(key, spec[key])
		if err != nil {
			return nil, err
		}
		if got := current[key]; got != want {
			class := classConfiguration
			if key == fieldPower {
				class = classState
			}
			out = append(out, desiredspec.Difference{Class: class, Field: key, Expected: want, Actual: got})
		}
	}
	return out, nil
}

func normalizeField(key, value string) (string, error) {
	switch key {
	case fieldPower:
		switch v := strings.ToLower(strings.TrimSpace(value)); v {
		case powerRunning, powerStopped:
			return v, nil
		}
		return "", fmt.Errorf("power must be running or stopped, not %q", value)
	case fieldCPUMax:
		return normalizeCPUMax(value)
	case fieldMemoryMax:
		bytes, err := parseMiB(value)
		if err != nil {
			return "", err
		}
		return formatMiB(bytes), nil
	case fieldAutostart:
		b, err := parseBool(value)
		if err != nil {
			return "", err
		}
		return strconv.FormatBool(b), nil
	}
	return "", fmt.Errorf("unknown field %q", key)
}

// normalizeCPUMax puts cpu.max in canonical "quota period" form; empty (the plugin's
// stored "no limit") and any "max" collapse to "max".
func normalizeCPUMax(value string) (string, error) {
	parts := strings.Fields(strings.ToLower(value))
	if len(parts) == 0 {
		return "max", nil
	}
	if len(parts) > 2 {
		return "", fmt.Errorf(`cpu_max must be "max" or a quota and period, not %q`, value)
	}
	period := uint64(defaultCPUPeriod)
	if len(parts) == 2 {
		parsed, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || parsed == 0 {
			return "", fmt.Errorf("cpu_max period must be a positive number, not %q", parts[1])
		}
		period = parsed
	}
	if parts[0] == "max" {
		return "max", nil
	}
	quota, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || quota == 0 {
		return "", fmt.Errorf(`cpu_max quota must be "max" or a positive number, not %q`, parts[0])
	}
	return fmt.Sprintf("%d %d", quota, period), nil
}

// parseMiB reads a MiB count, fractions allowed, into bytes; "max" and "unlimited"
// mean no limit, which the plugin stores as zero.
func parseMiB(value string) (int64, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "max" || v == "unlimited" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("memory_max must be a non-negative number of MiB, not %q", value)
	}
	return int64(math.Round(f * float64(mebibyte))), nil
}

func formatMiB(bytes int64) string {
	if bytes <= 0 {
		return "0"
	}
	if bytes%mebibyte == 0 {
		return strconv.FormatInt(bytes/mebibyte, 10)
	}
	return strconv.FormatFloat(float64(bytes)/float64(mebibyte), 'f', -1, 64)
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("autostart must be true or false, not %q", value)
}

// planSteps orders the fixes. Existence drift replaces the plan with one manual step,
// and a template is never started because the service refuses to run one.
func planSteps(isTemplate bool, diffs []desiredspec.Difference) []desiredspec.Step {
	byField := map[string]desiredspec.Difference{}
	for _, d := range diffs {
		if d.Class == classExistence {
			return []desiredspec.Step{{
				Action:      actionRecreate,
				Description: "Recreate the system container, then point this desired state at it.",
				Fields:      []string{fieldExistence},
				Manual:      true,
				Reason:      "The container no longer exists; recreating it needs its image, pool and network chosen by a person.",
			}}
		}
		byField[d.Field] = d
	}

	var steps []desiredspec.Step
	if d, ok := byField[fieldCPUMax]; ok {
		steps = append(steps, desiredspec.Step{
			Action:      actionSetCPUMax,
			Description: fmt.Sprintf("Set cpu.max from %s to %s.", d.Actual, d.Expected),
			Fields:      []string{fieldCPUMax},
		})
	}
	if d, ok := byField[fieldMemoryMax]; ok {
		steps = append(steps, desiredspec.Step{
			Action:      actionSetMemoryMax,
			Description: fmt.Sprintf("Set memory.max from %s MiB to %s MiB.", d.Actual, d.Expected),
			Fields:      []string{fieldMemoryMax},
			// Lowering memory.max under a live workload makes the kernel reclaim and,
			// failing that, OOM-kill inside the container.
			Disruptive: lowersMemory(d.Actual, d.Expected),
		})
	}
	if d, ok := byField[fieldAutostart]; ok {
		steps = append(steps, desiredspec.Step{
			Action:      actionSetAutostart,
			Description: fmt.Sprintf("Set autostart to %s.", d.Expected),
			Fields:      []string{fieldAutostart},
		})
	}
	if d, ok := byField[fieldPower]; ok {
		if step, ok := powerStep(isTemplate, d); ok {
			steps = append(steps, step)
		}
	}
	return steps
}

func powerStep(isTemplate bool, d desiredspec.Difference) (desiredspec.Step, bool) {
	fields := []string{fieldPower}
	switch {
	case d.Expected == powerRunning && isTemplate:
		return desiredspec.Step{
			Action: actionStart, Description: "Start the container.", Fields: fields, Manual: true,
			Reason: "This container is a template, and a template cannot be started.",
		}, true
	case d.Expected == powerRunning && d.Actual == powerFrozen:
		return desiredspec.Step{Action: actionUnfreeze, Description: "Thaw the frozen container.", Fields: fields}, true
	case d.Expected == powerRunning:
		return desiredspec.Step{Action: actionStart, Description: "Start the container.", Fields: fields}, true
	case d.Expected == powerStopped:
		return desiredspec.Step{
			Action: actionStop, Description: "Shut the container down gracefully.",
			Fields: fields, Disruptive: true,
		}, true
	}
	return desiredspec.Step{}, false
}

// lowersMemory treats zero as unlimited, so any finite limit replacing it lowers it.
func lowersMemory(actual, expected string) bool {
	from, errFrom := parseMiB(actual)
	to, errTo := parseMiB(expected)
	if errFrom != nil || errTo != nil {
		return true
	}
	if to == 0 {
		return false
	}
	return from == 0 || to < from
}

// applyDesiredStep dispatches one step. Limits go through UpdateLimits with the
// container's other limits unchanged, so only the pinned value moves.
func applyDesiredStep(ctx context.Context, act desiredActions, ct Container, spec map[string]string, step desiredspec.Step) error {
	switch step.Action {
	case actionSetCPUMax:
		value, err := pinnedValue(spec, fieldCPUMax)
		if err != nil {
			return err
		}
		limits := ct.Limits
		limits.CPUMax = value
		if value == "max" {
			limits.CPUMax = ""
		}
		return updateLimits(ctx, act, ct.ID, limits)

	case actionSetMemoryMax:
		if _, err := pinnedValue(spec, fieldMemoryMax); err != nil {
			return err
		}
		bytes, err := parseMiB(spec[fieldMemoryMax])
		if err != nil {
			return err
		}
		return updateLimits(ctx, act, ct.ID, withMemoryMax(ct.Limits, bytes))

	case actionSetAutostart:
		value, err := pinnedValue(spec, fieldAutostart)
		if err != nil {
			return err
		}
		autostart := value == "true"
		_, err = act.Update(ctx, ct.ID, UpdateRequest{Autostart: &autostart})
		return err

	case actionStart, actionStop, actionUnfreeze:
		if err := act.syncLiveState(ctx, ct); err != nil {
			return err
		}
		switch step.Action {
		case actionStart:
			return act.Start(ctx, ct.ID)
		case actionStop:
			_, err := act.Shutdown(ctx, ct.ID, 0)
			return err
		default:
			return act.Unfreeze(ctx, ct.ID)
		}
	}
	return fmt.Errorf("unknown step %q", step.Action)
}

func pinnedValue(spec map[string]string, key string) (string, error) {
	if !pinned(spec, key) {
		return "", fmt.Errorf("%s is not pinned in this desired state", key)
	}
	return normalizeField(key, spec[key])
}

func updateLimits(ctx context.Context, act desiredActions, id string, limits Limits) error {
	if err := validateLimits(limits); err != nil {
		return err
	}
	_, err := act.UpdateLimits(ctx, id, limits)
	return err
}

// withMemoryMax sets memory.max and drops memory.high back to the plugin's 90% default
// when it was that default or would sit above the new limit, which validation refuses.
func withMemoryMax(limits Limits, bytes int64) Limits {
	oldDefault := limits.MemoryMaxBytes / 10 * 9
	if limits.MemoryHighBytes == oldDefault || (bytes > 0 && limits.MemoryHighBytes > bytes) {
		limits.MemoryHighBytes = 0
	}
	limits.MemoryMaxBytes = bytes
	return limits
}

// syncLiveState records the agent's current state for ct when it differs from the row,
// because Start and Shutdown guard on the stored state and would otherwise no-op.
func (s *Service) syncLiveState(ctx context.Context, ct Container) error {
	states, err := s.queryStates(ctx, ct.EnvironmentID, []string{ct.ID})
	if err != nil {
		return err
	}
	observed := states[ct.ID]
	if observed == "" || observed == StateUnknown || observed == ct.State {
		return nil
	}
	if ct.State == StateCrashed && observed == StateStopped {
		return nil
	}
	return s.setState(ctx, ct.ID, observed, ct.LastError)
}

func (p *LXCPlugin) auditDesiredStep(ct Container, step desiredspec.Step) {
	if p.handler == nil {
		return
	}
	event := map[string]string{
		actionStart:        "plugin.lxc.ct.started",
		actionStop:         "plugin.lxc.ct.shutdown",
		actionUnfreeze:     "plugin.lxc.ct.unfrozen",
		actionSetCPUMax:    "plugin.lxc.ct.limits_changed",
		actionSetMemoryMax: "plugin.lxc.ct.limits_changed",
		actionSetAutostart: "plugin.lxc.ct.updated",
	}[step.Action]
	if event == "" {
		return
	}
	p.handler.emit(event, map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"userId":        "",
		"actor":         reconcilerActor,
		"source":        reconcilerActor,
		"action":        step.Action,
		"fields":        step.Fields,
	})
}
