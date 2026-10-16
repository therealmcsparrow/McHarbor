// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// Step actions a plan can hold. Manual is a step McHarbor will not take itself;
// it stays in the plan so the reviewer sees the whole of what is wrong.
const (
	ActionStart    = "start"
	ActionStop     = "stop"
	ActionRemove   = "remove"
	ActionRecreate = "recreate"
	ActionStackUp  = "stack_up"
	ActionManual   = "manual"
	// ActionPlugin is a step a plugin kind carries out; PluginAction names it.
	ActionPlugin = "plugin"
)

// Reasons a step is manual. They are codes, translated by the UI.
const (
	ReasonMissing        = "missing"
	ReasonComposeManaged = "compose_managed"
	ReasonPorts          = "ports"
	ReasonMounts         = "mounts"
	ReasonNetworks       = "networks"
	ReasonProtected      = "protected"
	ReasonStackRemove    = "stack_remove"
	ReasonStackStop      = "stack_stop"
	ReasonGitCompose     = "git_compose"
)

// Step is one change a plan makes, or one it leaves to a person.
type Step struct {
	Action   string                  `json:"action"`
	Target   string                  `json:"target"`
	Fields   []string                `json:"fields,omitempty"`
	Reason   string                  `json:"reason,omitempty"`
	Recreate *docker.RecreateRequest `json:"recreate,omitempty"`
	// PluginAction, Detail and Disruptive come from a plugin kind: the plugin's own
	// verb, a sentence for the reviewer, and whether the step interrupts service.
	PluginAction string `json:"pluginAction,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Disruptive   bool   `json:"disruptive,omitempty"`
}

// Applicable reports whether McHarbor would take this step itself.
func (s Step) Applicable() bool { return s.Action != ActionManual }

// Fingerprint identifies a report's diffs independent of when it was taken, so
// an approval can tell whether reality moved since the plan was written.
func Fingerprint(report *DriftReport) string {
	if report == nil {
		return ""
	}
	parts := make([]string, 0, len(report.Diffs))
	for _, d := range report.Diffs {
		parts = append(parts, d.Class+"\x1f"+d.Field+"\x1f"+d.Expected+"\x1f"+d.Actual)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return hex.EncodeToString(sum[:])
}

// FullyApplicable reports whether every step can be taken without a person,
// the precondition for enforcing a plan rather than proposing it.
func FullyApplicable(steps []Step) bool {
	for _, s := range steps {
		if !s.Applicable() {
			return false
		}
	}
	return len(steps) > 0
}

// PlanFor turns a drift report into the steps that restore the desired state.
// current is the container as inspected for a container-kind state, nil when
// it is missing or the kind is not a container.
func PlanFor(ds *DesiredState, report *DriftReport, current *container.InspectResponse) []Step {
	if report == nil || len(report.Diffs) == 0 {
		return []Step{}
	}
	switch ds.Kind {
	case KindStack:
		return planStack(ds, report)
	case KindEnvironment:
		return planEnvironment(report)
	default:
		return planContainer(ds, report, current)
	}
}

func planContainer(ds *DesiredState, report *DriftReport, current *container.InspectResponse) []Step {
	target := strings.TrimPrefix(ds.ContainerName, "/")
	var steps []Step
	var recreateFields, ports, mounts, networks []string
	stateDrift := false
	for _, d := range report.Diffs {
		switch {
		case d.Field == "existence" && d.Expected == "absent":
			return []Step{{Action: ActionRemove, Target: target, Fields: []string{d.Field}}}
		case d.Field == "existence":
			return []Step{{Action: ActionManual, Target: target, Fields: []string{d.Field}, Reason: ReasonMissing}}
		case d.Field == "status" && d.Expected == "stopped":
			return []Step{{Action: ActionStop, Target: target, Fields: []string{d.Field}}}
		case d.Field == "status":
			stateDrift = true
		case strings.HasPrefix(d.Field, "port."):
			ports = append(ports, d.Field)
		case strings.HasPrefix(d.Field, "mount."):
			mounts = append(mounts, d.Field)
		case strings.HasPrefix(d.Field, "network."):
			networks = append(networks, d.Field)
		default:
			recreateFields = append(recreateFields, d.Field)
		}
	}

	if len(recreateFields) > 0 {
		switch {
		case current == nil:
			steps = append(steps, Step{Action: ActionManual, Target: target, Fields: recreateFields, Reason: ReasonMissing})
		case isProtected(current):
			steps = append(steps, Step{Action: ActionManual, Target: target, Fields: recreateFields, Reason: ReasonProtected})
		case composeProject(current) != "":
			steps = append(steps, Step{Action: ActionManual, Target: composeProject(current), Fields: recreateFields, Reason: ReasonComposeManaged})
		default:
			cfg, cfgErr := ParseConfig(ds.Config, false)
			if cfgErr != nil {
				cfg = DesiredConfig{}
			}
			req := recreateRequest(ds, cfg, report, current)
			steps = append(steps, Step{Action: ActionRecreate, Target: target, Fields: recreateFields, Recreate: &req})
		}
	}
	if stateDrift {
		steps = append(steps, Step{Action: ActionStart, Target: target, Fields: []string{"status"}})
	}
	for _, m := range []struct {
		fields []string
		reason string
	}{{ports, ReasonPorts}, {mounts, ReasonMounts}, {networks, ReasonNetworks}} {
		if len(m.fields) > 0 {
			steps = append(steps, Step{Action: ActionManual, Target: target, Fields: m.fields, Reason: m.reason})
		}
	}
	return steps
}

// recreateRequest builds the overrides for a recreate. Env and labels replace
// the container's whole set, so the desired keys are merged into what it has.
func recreateRequest(ds *DesiredState, cfg DesiredConfig, report *DriftReport, current *container.InspectResponse) docker.RecreateRequest {
	var req docker.RecreateRequest
	changed := map[string]bool{}
	for _, d := range report.Diffs {
		changed[d.Field] = true
		if d.Class == ClassImage {
			req.PullImage = true
		}
	}
	if changed["image"] || req.PullImage {
		req.Image = ds.ImageRef
	}
	if len(cfg.Env) > 0 && anyPrefix(changed, "env.") {
		env := map[string]string{}
		order := []string{}
		if current.Config != nil {
			for _, kv := range current.Config.Env {
				k, v, _ := strings.Cut(kv, "=")
				if _, seen := env[k]; !seen {
					order = append(order, k)
				}
				env[k] = v
			}
		}
		for _, k := range sortedKeys(cfg.Env) {
			if _, seen := env[k]; !seen {
				order = append(order, k)
			}
			env[k] = cfg.Env[k]
		}
		for _, k := range order {
			req.Env = append(req.Env, k+"="+env[k])
		}
	}
	if len(cfg.Labels) > 0 && anyPrefix(changed, "label.") {
		req.Labels = map[string]string{}
		if current.Config != nil {
			for k, v := range current.Config.Labels {
				req.Labels[k] = v
			}
		}
		for k, v := range cfg.Labels {
			req.Labels[k] = v
		}
	}
	if changed["restartPolicy"] && ds.RestartPolicy != "" {
		req.RestartPolicy = &container.RestartPolicy{Name: container.RestartPolicyMode(ds.RestartPolicy)}
	}
	if changed["memoryBytes"] {
		v := cfg.MemoryBytes
		req.Memory = &v
	}
	if changed["nanoCpus"] {
		v := cfg.NanoCPUs
		req.NanoCPUs = &v
	}
	if anyPrefix(changed, "capAdd.") && current.HostConfig != nil {
		req.CapAdd = unionCaps(current.HostConfig.CapAdd, cfg.CapAdd)
	}
	if anyPrefix(changed, "capDrop.") && current.HostConfig != nil {
		req.CapDrop = unionCaps(current.HostConfig.CapDrop, cfg.CapDrop)
	}
	if anyPrefix(changed, "healthcheck.") && cfg.Healthcheck != nil {
		req.Healthcheck = healthcheckFor(*cfg.Healthcheck, current)
	}
	return req
}

func healthcheckFor(want HealthcheckSpec, current *container.InspectResponse) *docker.HealthcheckSpec {
	spec := &docker.HealthcheckSpec{Test: want.Test}
	if current.Config != nil && current.Config.Healthcheck != nil {
		h := current.Config.Healthcheck
		spec.Interval, spec.Timeout, spec.Retries, spec.StartPeriod = int64(h.Interval), int64(h.Timeout), h.Retries, int64(h.StartPeriod)
	}
	if d, err := time.ParseDuration(want.Interval); err == nil && want.Interval != "" {
		spec.Interval = int64(d)
	}
	if d, err := time.ParseDuration(want.Timeout); err == nil && want.Timeout != "" {
		spec.Timeout = int64(d)
	}
	if want.Retries > 0 {
		spec.Retries = want.Retries
	}
	return spec
}

func unionCaps(have, want []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range append(append([]string{}, have...), want...) {
		n := normalizeCap(c)
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func anyPrefix(fields map[string]bool, prefix string) bool {
	for f := range fields {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

func composeProject(c *container.InspectResponse) string {
	if c == nil || c.Config == nil {
		return ""
	}
	return c.Config.Labels["com.docker.compose.project"]
}

func isProtected(c *container.InspectResponse) bool {
	if c == nil || c.Config == nil {
		return false
	}
	return docker.IsProtectedContainer([]string{c.Name}, c.Config.Image, c.Config.Labels)
}

func planStack(ds *DesiredState, report *DriftReport) []Step {
	var up []string
	var steps []Step
	for _, d := range report.Diffs {
		switch d.Expected {
		case "not declared":
			steps = append(steps, Step{Action: ActionStop, Target: d.Actual, Fields: []string{d.Field}})
		case "absent":
			steps = append(steps, Step{Action: ActionManual, Target: ds.StackName, Fields: []string{d.Field}, Reason: ReasonStackRemove})
		case "stopped":
			steps = append(steps, Step{Action: ActionManual, Target: ds.StackName, Fields: []string{d.Field}, Reason: ReasonStackStop})
		default:
			up = append(up, d.Field)
		}
	}
	if len(up) > 0 {
		if ds.SourceCompose != "" {
			steps = append([]Step{{Action: ActionManual, Target: ds.StackName, Fields: up, Reason: ReasonGitCompose}}, steps...)
		} else {
			steps = append([]Step{{Action: ActionStackUp, Target: ds.StackName, Fields: up}}, steps...)
		}
	}
	return steps
}

// planEnvironment stops undeclared containers and never removes them: the
// container may be the one thing keeping an incident contained, and a stop is
// the reversible half of a removal.
func planEnvironment(report *DriftReport) []Step {
	steps := []Step{}
	for _, d := range report.Diffs {
		if name, ok := strings.CutPrefix(d.Field, "undeclared."); ok {
			steps = append(steps, Step{Action: ActionStop, Target: name, Fields: []string{d.Field}})
		}
	}
	return steps
}

// redactSteps returns copies of steps fit to store, return or audit: a recreate keeps only
// the env and label keys the plan changes, and every env value becomes the marker. The full
// request is rebuilt from the live container at apply time (rehydrateSteps).
func redactSteps(steps []Step) []Step {
	if steps == nil {
		return nil
	}
	out := make([]Step, len(steps))
	for i, s := range steps {
		out[i] = s
		out[i].Fields = append([]string(nil), s.Fields...)
		if s.Recreate == nil {
			continue
		}
		req := *s.Recreate
		envKeys, labelKeys := map[string]bool{}, map[string]bool{}
		for _, f := range s.Fields {
			if k, ok := strings.CutPrefix(f, "env."); ok {
				envKeys[k] = true
			}
			if k, ok := strings.CutPrefix(f, "label."); ok {
				labelKeys[k] = true
			}
		}
		req.Env = nil
		for _, kv := range s.Recreate.Env {
			k, _, _ := strings.Cut(kv, "=")
			if envKeys[k] {
				req.Env = append(req.Env, k+"="+envredact.Marker)
			}
		}
		req.Labels = nil
		for k, v := range s.Recreate.Labels {
			if !labelKeys[k] {
				continue
			}
			if req.Labels == nil {
				req.Labels = map[string]string{}
			}
			if (envredact.Policy{}).Withhold(k, v) {
				v = envredact.Marker
			}
			req.Labels[k] = v
		}
		out[i].Recreate = &req
	}
	return out
}

// rehydrateSteps keeps the reviewed plan but takes each recreate's request from fresh, a plan
// just built from the live container, because the stored one carries no env values. A
// recreate with no fresh counterpart loses its request, so applying it fails rather than
// writing the marker.
func rehydrateSteps(stored, fresh []Step) []Step {
	out := make([]Step, len(stored))
	for i, s := range stored {
		out[i] = s
		if s.Action != ActionRecreate {
			continue
		}
		out[i].Recreate = nil
		for _, f := range fresh {
			if f.Action == ActionRecreate && f.Target == s.Target && f.Recreate != nil {
				req := *f.Recreate
				out[i].Recreate = &req
				break
			}
		}
	}
	return out
}
