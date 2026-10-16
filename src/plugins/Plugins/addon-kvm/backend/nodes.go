// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Guest lifecycle workflow nodes.
//
// Four nodes covering what a VM workflow actually does: change a guest's state,
// snapshot it, clone it, or read it and branch. Anything about pools, networks
// or images belongs to addon-compute, which owns those objects.
//
// Every node routes a failure down the error port rather than returning an
// error. A workflow that stops a guest before a backup wants to handle "it was
// already stopped" itself, not have the run die.

const (
	nodeLifecycle = "vm-lifecycle"
	nodeSnapshot  = "vm-snapshot"
	nodeClone     = "vm-clone"
	nodeState     = "vm-state"
)

// registerNodes attaches the four descriptors and their executors.
func (p *KVMPlugin) registerNodes(reg plugin.NodeRegistry) error {
	// Every node needs a guest, and the picker is the same each time.
	vmField := map[string]any{
		"key":      "vmId",
		"label":    "Virtual machine",
		"type":     "text",
		"required": true,
		"help":     "The virtual machine's id, from its detail page. Leave the msg payload to supply it at run time by setting msg.vmId.",
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
				DisplayName: "Virtual machine: lifecycle",
				Description: "Start, shut down, stop, reset, pause or resume a virtual machine.",
				Icon:        "IconPlayerPlay",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "vm.manage",
				Fields: []map[string]any{
					envField, vmField,
					{
						"key":      "action",
						"label":    "Action",
						"type":     "select",
						"required": true,
						"default":  "start",
						"options": []map[string]any{
							{"value": "start", "label": "Start"},
							{"value": "shutdown", "label": "Shut down (asks the guest)"},
							{"value": "stop", "label": "Stop (immediate)"},
							{"value": "reset", "label": "Reset"},
							{"value": "pause", "label": "Pause"},
							{"value": "resume", "label": "Resume"},
						},
					},
					{
						"key":      "graceSeconds",
						"label":    "Shutdown grace period",
						"type":     "number",
						"default":  90,
						"help":     "How long to wait for the guest to power itself down before forcing it. Only used by the shutdown action.",
						"showWhen": map[string]any{"action": "shutdown"},
					},
				},
			},
		},
		{
			key: nodeSnapshot,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Virtual machine: snapshot",
				Description: "Take a snapshot. Without the guest agent installed a snapshot is crash-consistent — the equivalent of pulling the power cord.",
				Icon:        "IconCameraPlus",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "vm.manage",
				Fields: []map[string]any{
					envField, vmField,
					{
						"key":      "name",
						"label":    "Snapshot name",
						"type":     "text",
						"required": true,
						"help":     "Supports {{msg}} substitution, so a scheduled workflow can stamp the date into the name.",
					},
					{
						"key":     "withMemory",
						"label":   "Include the guest's memory",
						"type":    "boolean",
						"default": false,
						"help":    "Reverting then resumes exactly where the guest was rather than booting it. Costs as much space as the guest has RAM.",
					},
				},
			},
		},
		{
			key: nodeClone,
			desc: plugin.NodeDescriptor{
				Category:    "action",
				DisplayName: "Virtual machine: clone",
				Description: "Clone a stopped guest or a template. The clone gets a new machine identity, new MAC addresses and a fresh cloud-init run.",
				Icon:        "IconCopy",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "vm.manage",
				Fields: []map[string]any{
					envField, vmField,
					{
						"key":      "name",
						"label":    "New name",
						"type":     "text",
						"required": true,
					},
					{
						"key":     "linked",
						"label":   "Linked clone",
						"type":    "boolean",
						"default": true,
						"help":    "Shares data with the source until either is written — instant and space-efficient. The source cannot be deleted while a linked clone exists.",
					},
				},
			},
		},
		{
			key: nodeState,
			desc: plugin.NodeDescriptor{
				Category:    "logic",
				DisplayName: "Virtual machine: state",
				Description: "Read a guest and branch on its state. Routes to the error port when the guest is not in the state you expect.",
				Icon:        "IconInfoCircle",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				// A read, so `view` rather than `manage`.
				Requires: "vm.view",
				Fields: []map[string]any{
					envField, vmField,
					{
						"key":     "expectState",
						"label":   "Expect state",
						"type":    "select",
						"default": "any",
						"options": []map[string]any{
							{"value": "any", "label": "Any"},
							{"value": "running", "label": "Running"},
							{"value": "stopped", "label": "Stopped"},
							{"value": "paused", "label": "Paused"},
						},
						"help": "Route to the error port when the guest is in a different state.",
					},
				},
			},
		},
	}

	for _, node := range nodes {
		desc := node.desc
		desc.Execute = p.execNode
		if err := reg.Add(node.key, desc); err != nil {
			return fmt.Errorf("kvm: register node %s: %w", node.key, err)
		}
	}
	trigger := vmTriggerDescriptor()
	trigger.Execute = p.execVMTrigger
	if err := reg.Add(nodeVMTrigger, trigger); err != nil {
		return fmt.Errorf("kvm: register node %s: %w", nodeVMTrigger, err)
	}
	return nil
}

// execNode is the one executor behind all four nodes.
func (p *KVMPlugin) execNode(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil {
		return errorPort(in, "virtual machines are not available on this installation"), nil
	}

	vmID := resolveVMID(in)
	if vmID == "" {
		return errorPort(in, "this node needs a virtual machine"), nil
	}

	vm, err := p.svc.Get(ctx, vmID)
	if err != nil {
		return errorPort(in, err.Error()), nil
	}
	if !vm.HostAvailable {
		// Compute commands are never queued, so a workflow that fires while a
		// host is offline gets told rather than silently deferred.
		return errorPort(in, "this guest's host cannot be reached"), nil
	}

	switch in.Key {
	case nodeLifecycle:
		return p.execLifecycle(ctx, in, vm)
	case nodeSnapshot:
		return p.execSnapshot(ctx, in, vm)
	case nodeClone:
		return p.execClone(ctx, in, vm)
	case nodeState:
		return p.execState(in, vm)
	default:
		return plugin.NodeExecOutput{}, fmt.Errorf("kvm: unknown node %q", in.Key)
	}
}

// resolveVMID takes the guest from the node's config, falling back to the
// message.
//
// The message fallback is what lets one workflow act on many guests: a list
// node upstream sets msg.vmId per branch and this node acts on each.
func resolveVMID(in plugin.NodeExecInput) string {
	if configured, _ := in.Config["vmId"].(string); configured != "" {
		return configured
	}
	if fromMsg, _ := in.Msg["vmId"].(string); fromMsg != "" {
		return fromMsg
	}
	// A payload from an upstream VM node carries the id too.
	if payload, ok := in.Msg["payload"].(map[string]any); ok {
		if id, _ := payload["vmId"].(string); id != "" {
			return id
		}
	}
	return ""
}

func (p *KVMPlugin) execLifecycle(
	ctx context.Context,
	in plugin.NodeExecInput,
	vm VM,
) (plugin.NodeExecOutput, error) {
	action, _ := in.Config["action"].(string)
	ctx = withIntentOrigin(ctx, "workflow:"+in.WorkflowID, "workflow")

	var err error
	escalated := false

	switch action {
	case "start":
		err = p.svc.Start(ctx, vm.ID)
	case "shutdown":
		escalated, err = p.svc.Shutdown(ctx, vm.ID, configInt(in.Config, "graceSeconds", 90))
	case "stop":
		err = p.svc.Stop(ctx, vm.ID)
	case "reset":
		err = p.svc.Reset(ctx, vm.ID)
	case "pause":
		err = p.svc.Pause(ctx, vm.ID)
	case "resume":
		err = p.svc.Resume(ctx, vm.ID)
	default:
		return errorPort(in, fmt.Sprintf("%q is not a lifecycle action", action)), nil
	}

	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	// A workflow driving a guest is the same act as a person doing it, and
	// until this it was the one path that left no audit row at all.
	p.svc.recordAudit("plugin.kvm.vm."+action, map[string]any{
		"vmId": vm.ID, "name": vm.Name, "environmentId": vm.EnvironmentID,
		"escalated": escalated, "source": "workflow", "workflowId": in.WorkflowID,
	})

	return outputPort(in, map[string]any{
		"vmId": vm.ID, "name": vm.Name, "action": action,
		// Carried so a workflow can notice a guest that ignored ACPI and, say,
		// raise a ticket about it.
		"escalated": escalated,
	}), nil
}

func (p *KVMPlugin) execSnapshot(
	ctx context.Context,
	in plugin.NodeExecInput,
	vm VM,
) (plugin.NodeExecOutput, error) {
	name, _ := in.Config["name"].(string)
	if name == "" {
		return errorPort(in, "this node needs a snapshot name"), nil
	}
	withMemory, _ := in.Config["withMemory"].(bool)

	snapshot, err := p.svc.CreateSnapshot(withIntentOrigin(ctx, "workflow:"+in.WorkflowID, "workflow"), vm.ID, SnapshotRequest{
		Name: name, WithMemory: withMemory,
		Description: fmt.Sprintf("Taken by workflow %s", in.WorkflowID),
	})
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	p.svc.recordAudit("plugin.kvm.snapshot.created", map[string]any{
		"vmId": vm.ID, "name": vm.Name, "environmentId": vm.EnvironmentID,
		"snapshotId": snapshot.ID, "snapshotName": snapshot.Name,
		"source": "workflow", "workflowId": in.WorkflowID,
	})

	return outputPort(in, map[string]any{
		"vmId": vm.ID, "name": vm.Name,
		"snapshotId": snapshot.ID, "snapshotName": snapshot.Name,
		"mechanism": snapshot.Mechanism,
		// Surfaced so a backup workflow can record what its restore is
		// actually worth rather than assuming.
		"consistency": snapshot.Consistency,
		"chainDepth":  snapshot.ChainDepth,
	}), nil
}

func (p *KVMPlugin) execClone(
	ctx context.Context,
	in plugin.NodeExecInput,
	vm VM,
) (plugin.NodeExecOutput, error) {
	name, _ := in.Config["name"].(string)
	if name == "" {
		return errorPort(in, "this node needs a name for the clone"), nil
	}
	linked, ok := in.Config["linked"].(bool)
	if !ok {
		linked = true
	}

	clone, err := p.svc.Clone(ctx, vm.ID, CloneRequest{Name: name, Linked: linked})
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	p.svc.recordAudit("plugin.kvm.vm.cloned", map[string]any{
		"sourceVmId": vm.ID, "vmId": clone.ID, "name": clone.Name,
		"environmentId": vm.EnvironmentID, "linked": linked,
		"source": "workflow", "workflowId": in.WorkflowID,
	})

	return outputPort(in, map[string]any{
		"sourceVmId": vm.ID,
		// The new id is on `vmId` deliberately: a downstream lifecycle node
		// then acts on the clone with no extra wiring.
		"vmId":   clone.ID,
		"name":   clone.Name,
		"linked": linked,
	}), nil
}

func (p *KVMPlugin) execState(in plugin.NodeExecInput, vm VM) (plugin.NodeExecOutput, error) {
	payload := map[string]any{
		"vmId": vm.ID, "name": vm.Name, "state": vm.State,
		"vcpus": vm.VCPUs, "memoryMb": vm.MemoryMB,
		"isTemplate": vm.IsTemplate,
		"guestAgent": vm.GuestAgentState,
		// Guest-reported and marked as such, so a workflow author can see that
		// this is what the guest claims rather than what the host observed.
		"guestReportedIps": vm.GuestReportedIPs,
	}

	expect, _ := in.Config["expectState"].(string)
	if expect != "" && expect != "any" && vm.State != expect {
		return plugin.NodeExecOutput{
			Port: "error",
			Msg: withPayload(in.Msg, payload,
				fmt.Sprintf("%s is %s, not %s", vm.Name, vm.State, expect)),
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
		// Promoted so a downstream node's own vmId lookup finds it.
		if id, ok := payload["vmId"].(string); ok {
			out["vmId"] = id
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
