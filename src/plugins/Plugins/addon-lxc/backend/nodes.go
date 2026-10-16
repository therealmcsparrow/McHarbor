// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Container workflow nodes.
//
// Five nodes covering what a container workflow actually does: change a
// container's state, change its limits, snapshot it, clone it, or read it and
// branch. Anything about pools, networks or images belongs to addon-compute,
// which owns those objects.
//
// The limits node has no equivalent in addon-kvm, and that is the point: a
// container's CPU, memory and pid limits apply while it runs. A workflow can
// therefore give a container more memory before a nightly batch job and take it
// back afterwards, with no restart and no downtime — which is not something a
// VM workflow can offer.
//
// Every node routes a failure down the error port rather than returning an
// error. A workflow that stops a container before a backup wants to handle "it
// was already stopped" itself, not have the run die.

const (
	nodeLifecycle = "ct-lifecycle"
	nodeLimits    = "ct-limits"
	nodeSnapshot  = "ct-snapshot"
	nodeClone     = "ct-clone"
	nodeState     = "ct-state"
)

// registerNodes attaches the five descriptors and their executors.
func (p *LXCPlugin) registerNodes(reg plugin.NodeRegistry) error {
	// Every node needs a container, and the picker is the same each time.
	ctField := map[string]any{
		"key":         "containerId",
		"label":       "Container",
		"type":        "plugin-select",
		"required":    true,
		"allowCustom": true,
		"source":      map[string]any{"plugin": "addon-lxc", "target": "ct"},
		"help":        "The container's id, from its detail page. Leave it empty to supply it at run time by setting msg.containerId.",
	}
	envField := map[string]any{
		"key":      "environment",
		"label":    "Host",
		"type":     "environment",
		"required": false,
	}

	nodes := []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{
			key: nodeLifecycle,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Container: lifecycle",
				Description: "Start, shut down, stop, restart, freeze or thaw a system container.",
				Icon:        "IconPlayerPlay",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "ct.manage",
				Fields: []map[string]any{
					envField, ctField,
					{
						"key":      "action",
						"label":    "Action",
						"type":     "select",
						"required": true,
						"default":  "start",
						"options": []map[string]any{
							{"value": "start", "label": "Start"},
							{"value": "shutdown", "label": "Shut down (asks init)"},
							{"value": "stop", "label": "Stop (immediate)"},
							{"value": "restart", "label": "Restart"},
							{"value": "freeze", "label": "Freeze (suspend every process)"},
							{"value": "unfreeze", "label": "Thaw"},
						},
					},
					{
						"key":      "graceSeconds",
						"label":    "Shutdown grace period",
						"type":     "number",
						"default":  defaultGraceSeconds,
						"help":     "How long init gets to exit before the container's cgroup is killed. Only used by the shutdown action.",
						"showWhen": map[string]any{"action": "shutdown"},
					},
				},
			},
		},
		{
			key: nodeLimits,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Container: resource limits",
				Description: "Change a container's memory, CPU and process limits. These apply to a running container immediately — no restart.",
				Icon:        "IconAdjustments",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "ct.manage",
				Fields: []map[string]any{
					envField, ctField,
					{
						"key":   "memoryMaxMb",
						"label": "Memory limit (MB)",
						"type":  "number",
						"help":  "Leave empty to keep the current limit. The kernel may round this to a page multiple; the node's output reports what was actually applied.",
					},
					{
						"key":   "cpuMax",
						"label": "CPU limit",
						"type":  "text",
						"help":  `cgroup v2 cpu.max: "max" for unlimited, or a quota and a period in microseconds — "200000 100000" is two cores. Leave empty to keep the current limit.`,
					},
					{
						"key":   "pidsMax",
						"label": "Process limit",
						"type":  "number",
						"help":  "The maximum number of processes. Leave empty to keep the current limit.",
					},
					{
						// Worth a switch of its own, because it is the whole
						// reason this node is useful in a schedule: raise the
						// limits for a batch window, then put them back.
						"key":     "failOnClamp",
						"label":   "Fail if the kernel changes a value",
						"type":    "boolean",
						"default": false,
						"help":    "Route to the error port when the kernel did not accept a limit exactly as asked — for example when the host has less memory free than requested.",
					},
				},
			},
		},
		{
			key: nodeSnapshot,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Container: snapshot",
				Description: "Take a snapshot. A running container is frozen for the moment it takes, unless you turn that off.",
				Icon:        "IconCameraPlus",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "ct.manage",
				Fields: []map[string]any{
					envField, ctField,
					{
						"key":      "name",
						"label":    "Snapshot name",
						"type":     "text",
						"required": true,
						"help":     "Supports {{msg}} substitution, so a scheduled workflow can stamp the date into the name.",
					},
					{
						"key":     "freeze",
						"label":   "Freeze the container first",
						"type":    "boolean",
						"default": true,
						"help":    "Costs milliseconds on a copy-on-write pool and is what makes the snapshot consistent. Without it the snapshot is crash-consistent — the equivalent of pulling the power cord.",
					},
				},
			},
		},
		{
			key: nodeClone,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Container: clone",
				Description: "Clone a stopped container or a template. On a copy-on-write pool this is a metadata operation.",
				Icon:        "IconCopy",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "ct.manage",
				Fields: []map[string]any{
					envField, ctField,
					{
						"key":      "name",
						"label":    "Name for the clone",
						"type":     "text",
						"required": true,
					},
					{
						"key":     "start",
						"label":   "Start the clone",
						"type":    "boolean",
						"default": false,
						"help":    "Off by default: a clone that starts before its network configuration has been looked at is already on the LAN unconfigured.",
					},
				},
			},
		},
		{
			key: nodeState,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Container: read state",
				Description: "Read a container's state and resource usage, and optionally branch on it.",
				Icon:        "IconInfoCircle",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "ct.view",
				Fields: []map[string]any{
					envField, ctField,
					{
						"key":     "expectState",
						"label":   "Expect state",
						"type":    "select",
						"default": "any",
						"options": []map[string]any{
							{"value": "any", "label": "Any"},
							{"value": "running", "label": "Running"},
							{"value": "stopped", "label": "Stopped"},
							{"value": "frozen", "label": "Frozen"},
						},
						"help": "Route to the error port when the container is in a different state.",
					},
				},
			},
		},
	}

	for _, node := range nodes {
		desc := node.desc
		desc.Execute = p.execNode
		if err := reg.Add(node.key, desc); err != nil {
			return fmt.Errorf("lxc: register node %s: %w", node.key, err)
		}
	}
	return nil
}

// execNode is the one executor behind all five nodes.
func (p *LXCPlugin) execNode(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil {
		return errorPort(in, "system containers are not available on this installation"), nil
	}

	containerID := resolveContainerID(in)
	if containerID == "" {
		return errorPort(in, "this node needs a container"), nil
	}

	ct, err := p.svc.Get(ctx, containerID)
	if err != nil {
		return errorPort(in, err.Error()), nil
	}
	if !ct.HostAvailable {
		// Compute commands are never queued, so a workflow that fires while a
		// host is offline gets told rather than silently deferred.
		return errorPort(in, "this container's host cannot be reached"), nil
	}

	switch in.Key {
	case nodeLifecycle:
		return p.execLifecycle(ctx, in, ct)
	case nodeLimits:
		return p.execLimits(ctx, in, ct)
	case nodeSnapshot:
		return p.execSnapshot(ctx, in, ct)
	case nodeClone:
		return p.execClone(ctx, in, ct)
	case nodeState:
		return p.execState(in, ct)
	default:
		return plugin.NodeExecOutput{}, fmt.Errorf("lxc: unknown node %q", in.Key)
	}
}

// resolveContainerID takes the container from the node's config, falling back
// to the message.
//
// The message fallback is what lets one workflow act on many containers: a list
// node upstream sets msg.containerId per branch and this node acts on each.
func resolveContainerID(in plugin.NodeExecInput) string {
	if configured, _ := in.Config["containerId"].(string); configured != "" {
		return configured
	}
	if fromMsg, _ := in.Msg["containerId"].(string); fromMsg != "" {
		return fromMsg
	}
	// A payload from an upstream container node carries the id too.
	if payload, ok := in.Msg["payload"].(map[string]any); ok {
		if id, _ := payload["containerId"].(string); id != "" {
			return id
		}
	}
	return ""
}

func (p *LXCPlugin) execLifecycle(
	ctx context.Context,
	in plugin.NodeExecInput,
	ct Container,
) (plugin.NodeExecOutput, error) {
	action, _ := in.Config["action"].(string)

	var err error
	escalated := false

	switch action {
	case "start":
		err = p.svc.Start(ctx, ct.ID)
	case "shutdown":
		escalated, err = p.svc.Shutdown(ctx, ct.ID,
			configInt(in.Config, "graceSeconds", defaultGraceSeconds))
	case "stop":
		err = p.svc.Stop(ctx, ct.ID)
	case "restart":
		err = p.svc.Restart(ctx, ct.ID)
	case "freeze":
		err = p.svc.Freeze(ctx, ct.ID)
	case "unfreeze":
		err = p.svc.Unfreeze(ctx, ct.ID)
	default:
		return errorPort(in, fmt.Sprintf("%q is not a lifecycle action", action)), nil
	}

	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	return outputPort(in, map[string]any{
		"containerId": ct.ID, "name": ct.Name, "action": action,
		// Carried so a workflow can notice a container whose init ignored its
		// stop signal and, say, raise a ticket about it.
		"escalated": escalated,
	}), nil
}

// execLimits changes a container's limits live.
//
// The node reports what the kernel actually applied, not what was asked for,
// and can be set to route to the error port when the two differ. That option
// exists because "give this container 32 GB for the nightly job" quietly
// becoming "the kernel gave it 4 GB" is exactly the kind of failure a scheduled
// workflow should be able to notice on its own.
func (p *LXCPlugin) execLimits(
	ctx context.Context,
	in plugin.NodeExecInput,
	ct Container,
) (plugin.NodeExecOutput, error) {
	// Start from what the container has, so a node that sets only memory does
	// not silently clear the CPU and pid limits.
	requested := ct.Limits

	if memoryMB := configInt(in.Config, "memoryMaxMb", 0); memoryMB > 0 {
		requested.MemoryMaxBytes = int64(memoryMB) * 1024 * 1024
		// The soft throttle follows the hard limit rather than being left at a
		// value that is now above it, which the kernel would refuse.
		requested.MemoryHighBytes = 0
	}
	if cpuMax, _ := in.Config["cpuMax"].(string); cpuMax != "" {
		requested.CPUMax = cpuMax
	}
	if pidsMax := configInt(in.Config, "pidsMax", 0); pidsMax > 0 {
		requested.PidsMax = int64(pidsMax)
	}

	requested = withLimitDefaults(requested)
	if err := validateLimits(requested); err != nil {
		return errorPort(in, err.Error()), nil
	}

	result, err := p.svc.UpdateLimits(ctx, ct.ID, requested)
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	payload := map[string]any{
		"containerId": ct.ID, "name": ct.Name,
		"requested": result.Requested,
		"applied":   result.Applied,
		"clamped":   result.Clamped,
		// False means the container was stopped and the values are stored for
		// its next start rather than in force now.
		"live": result.Live,
	}

	if failOnClamp, _ := in.Config["failOnClamp"].(bool); failOnClamp && len(result.Clamped) > 0 {
		return plugin.NodeExecOutput{
			Port: "error",
			Msg: withPayload(in.Msg, payload,
				fmt.Sprintf("the kernel did not accept %d of the requested limits", len(result.Clamped))),
		}, nil
	}
	return outputPort(in, payload), nil
}

func (p *LXCPlugin) execSnapshot(
	ctx context.Context,
	in plugin.NodeExecInput,
	ct Container,
) (plugin.NodeExecOutput, error) {
	name, _ := in.Config["name"].(string)
	if name == "" {
		return errorPort(in, "this node needs a snapshot name"), nil
	}

	freeze, ok := in.Config["freeze"].(bool)
	if !ok {
		freeze = true
	}

	snapshot, err := p.svc.CreateSnapshot(ctx, ct.ID, CreateSnapshotRequest{
		Name:        name,
		Freeze:      &freeze,
		Description: fmt.Sprintf("Taken by workflow %s", in.WorkflowID),
	})
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	return outputPort(in, map[string]any{
		"containerId": ct.ID, "name": ct.Name,
		"snapshotId": snapshot.ID, "snapshotName": snapshot.Name,
		"mechanism": snapshot.Mechanism,
		// Surfaced so a backup workflow can record what its restore is
		// actually worth rather than assuming.
		"wasFrozen":  snapshot.WasFrozen,
		"chainDepth": snapshot.ChainDepth,
	}), nil
}

func (p *LXCPlugin) execClone(
	ctx context.Context,
	in plugin.NodeExecInput,
	ct Container,
) (plugin.NodeExecOutput, error) {
	name, _ := in.Config["name"].(string)
	if name == "" {
		return errorPort(in, "this node needs a name for the clone"), nil
	}
	if err := validateContainerName(name); err != nil {
		return errorPort(in, err.Error()), nil
	}
	start, _ := in.Config["start"].(bool)

	clone, err := p.svc.Clone(ctx, ct.ID, CloneRequest{Name: name, Start: start})
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	return outputPort(in, map[string]any{
		"sourceContainerId": ct.ID,
		// The new id is on `containerId` deliberately: a downstream lifecycle
		// node then acts on the clone with no extra wiring.
		"containerId": clone.ID,
		"name":        clone.Name,
		// Whether the clone shares its source's rootfs, which decides whether
		// deleting the source would destroy it.
		"linked": clone.TemplatePath != "",
	}), nil
}

func (p *LXCPlugin) execState(in plugin.NodeExecInput, ct Container) (plugin.NodeExecOutput, error) {
	payload := map[string]any{
		"containerId": ct.ID, "name": ct.Name, "state": ct.State,
		"initSystem": ct.InitSystem,
		"privileged": ct.Privileged,
		"isTemplate": ct.IsTemplate,
		"limits":     ct.Limits,
		// Read from the container's netns by the agent, so this is what the
		// host observed rather than what the container claims.
		"observedIps": ct.ObservedIPs,
	}

	expect, _ := in.Config["expectState"].(string)
	if expect != "" && expect != "any" && ct.State != expect {
		return plugin.NodeExecOutput{
			Port: "error",
			Msg: withPayload(in.Msg, payload,
				fmt.Sprintf("%s is %s, not %s", ct.Name, ct.State, expect)),
		}, nil
	}
	return outputPort(in, payload), nil
}

// --- Port helpers ---------------------------------------------------------

func outputPort(in plugin.NodeExecInput, payload map[string]any) plugin.NodeExecOutput {
	return plugin.NodeExecOutput{Port: "output", Msg: withPayload(in.Msg, payload, "")}
}

func errorPort(in plugin.NodeExecInput, reason string) plugin.NodeExecOutput {
	return plugin.NodeExecOutput{Port: "error", Msg: withPayload(in.Msg, nil, reason)}
}

// withPayload copies the incoming message and sets payload / error on it.
//
// The copy matters: the same msg object reaches every branch of a fan-out, and
// mutating it in place would have one branch's result appear in another's.
func withPayload(msg map[string]any, payload map[string]any, errText string) map[string]any {
	out := make(map[string]any, len(msg)+2)
	for key, value := range msg {
		out[key] = value
	}
	if payload != nil {
		out["payload"] = payload
		// Promoted so a downstream node's own containerId lookup finds it.
		if id, ok := payload["containerId"].(string); ok {
			out["containerId"] = id
		}
	}
	if errText != "" {
		out["error"] = errText
	}
	return out
}

// configInt reads a numeric config value, tolerating the float64 a JSON decode
// produces.
func configInt(config map[string]any, key string, fallback int) int {
	switch value := config[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return fallback
	}
}
