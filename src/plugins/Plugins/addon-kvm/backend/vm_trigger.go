// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// nodeVMTrigger keeps the key the host shipped this node under, so the
// workflow-node-alias in plugin.yaml resolves canvases saved before the move.
const nodeVMTrigger = "vm-trigger"

// vmTriggerFireTimeout bounds the bindings read one transition costs. The runs
// themselves are detached by the host, so this never cuts a workflow short.
const vmTriggerFireTimeout = 10 * time.Second

// vmTriggerDescriptor is the guest-transition trigger. Its VM field lists through the
// plugin's own picker-source, so the host renders it without naming this plugin's route.
func vmTriggerDescriptor() plugin.NodeDescriptor {
	return plugin.NodeDescriptor{
		Category:    "trigger",
		DisplayName: "VM Trigger",
		Description: "Start a workflow when a virtual machine changes state",
		Icon:        "IconServerBolt",
		Inputs:      []string{},
		Outputs:     []string{"output"},
		// It reports guest names and states, which is what vm.view guards.
		Requires: "vm.view",
		Fields: []map[string]any{
			{"key": "environment", "label": "Environment", "type": "environment-select", "required": false},
			{
				"key": "vm", "label": "Virtual Machine", "type": "plugin-select", "required": false,
				"allowCustom": true,
				"source":      map[string]any{"plugin": "addon-kvm", "target": "vm"},
			},
			{
				"key": "states", "label": "States", "type": "multiselect", "required": false,
				"allowCustom": true, "default": []string{},
				"options": []map[string]any{
					{"value": StateRunning, "label": "Running"},
					{"value": StatePaused, "label": "Paused"},
					{"value": "stopping", "label": "Stopping"},
					{"value": StateStopped, "label": "Stopped"},
					{"value": "crashed", "label": "Crashed"},
					{"value": "migrating", "label": "Migrating"},
					{"value": "defined", "label": "Defined"},
					{"value": "deleting", "label": "Deleting"},
				},
			},
			{
				"key": "severity", "label": "Severity", "type": "select", "required": false, "default": "any",
				"options": []map[string]any{
					{"value": "any", "label": "Any"},
					{"value": "success", "label": "Success"},
					{"value": "info", "label": "Info"},
					{"value": "warning", "label": "Warning"},
					{"value": "error", "label": "Error"},
				},
			},
		},
	}
}

// guestTransition is one recorded state change, the event a vm-trigger matches.
type guestTransition struct {
	GuestID       string
	GuestName     string
	EnvironmentID string
	State         string
	Severity      string
	Metadata      map[string]any
	Timestamp     string
}

// guestTransitionMatches applies a node's filters. Every filter left blank matches
// everything, so an unconfigured node fires on all guest transitions.
func guestTransitionMatches(config map[string]any, ev guestTransition) bool {
	if envID := trimmed(config["environment"]); envID != "" && envID != ev.EnvironmentID {
		return false
	}
	// A guest is named by id in the picker but by name in a typed value, and both are
	// what the operator sees, so either identifies the same machine.
	if guest := trimmed(config["vm"]); guest != "" && guest != ev.GuestID && !strings.EqualFold(guest, ev.GuestName) {
		return false
	}
	if states := stringList(config["states"]); len(states) > 0 {
		matched := false
		for _, state := range states {
			if strings.EqualFold(strings.TrimSpace(state), ev.State) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if severity := trimmed(config["severity"]); severity != "" && severity != "any" && severity != ev.Severity {
		return false
	}
	return true
}

// guestTransitionPayload is msg.payload of a fired vm-trigger, the shape the host's
// watcher emitted before the node moved here.
func guestTransitionPayload(ev guestTransition, autoTriggered bool) map[string]any {
	payload := map[string]any{
		"trigger":       "vm",
		"vm":            map[string]any{"id": ev.GuestID, "name": ev.GuestName},
		"environmentId": ev.EnvironmentID,
		"action":        ev.State,
		"state":         ev.State,
		"severity":      ev.Severity,
		"timestamp":     ev.Timestamp,
		"autoTriggered": autoTriggered,
	}
	if len(ev.Metadata) > 0 {
		payload["metadata"] = ev.Metadata
		if failure, ok := ev.Metadata["error"].(string); ok && failure != "" {
			payload["error"] = failure
		}
	}
	return payload
}

// transitionSeverity is the severity a timeline row and a trigger carry for a state.
func transitionSeverity(state, lastError string) string {
	switch {
	case lastError != "":
		return "error"
	case state == StateRunning:
		return "success"
	case state == StateStopped, state == StatePaused:
		return "warning"
	default:
		return "info"
	}
}

// fireGuestTrigger starts every active workflow whose vm-trigger accepts ev. It runs
// where the timeline row is written, so the trigger and the timeline share one rule
// about what counts as a transition.
func (s *Service) fireGuestTrigger(ctx context.Context, ev guestTransition) {
	if s.workflows == nil {
		return
	}
	fireCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), vmTriggerFireTimeout)
	defer cancel()
	match := func(b plugin.WorkflowTriggerBinding) bool { return guestTransitionMatches(b.Config, ev) }
	if _, err := s.workflows.Fire(fireCtx, nodeVMTrigger, guestTransitionPayload(ev, true), match); err != nil {
		s.logger.Warn("kvm: firing the vm-trigger failed", "guest", ev.GuestID, "state", ev.State, "error", err)
	}
}

// execVMTrigger answers both runs of the node. A fired run already carries the
// transition and passes it on; a manual run from the editor has none, so it reports the
// current state of the first guest the filters accept, letting the operator test the
// flow against a real machine instead of an empty message.
func (p *KVMPlugin) execVMTrigger(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if payload, ok := in.Msg["payload"].(map[string]any); ok && payload["trigger"] == "vm" {
		return plugin.NodeExecOutput{Port: "output", Msg: in.Msg}, nil
	}
	msg := map[string]any{}
	for k, v := range in.Msg {
		msg[k] = v
	}
	msg["payload"] = map[string]any{"trigger": "vm", "autoTriggered": false, "matched": false}
	if p.svc == nil {
		return plugin.NodeExecOutput{Port: "output", Msg: msg}, nil
	}
	envID := trimmed(in.Config["environment"])
	if envID == "" {
		envID = in.EnvironmentID
	}
	guests, err := p.svc.List(ctx, envID, false)
	if err != nil {
		p.logger.Warn("kvm: listing guests for a manual vm-trigger run failed", "error", err)
		return plugin.NodeExecOutput{Port: "output", Msg: msg}, nil
	}
	for _, vm := range guests {
		ev := guestTransition{GuestID: vm.ID, GuestName: vm.Name, EnvironmentID: vm.EnvironmentID,
			State: vm.State, Severity: transitionSeverity(vm.State, vm.LastError),
			Timestamp: time.Now().UTC().Format(time.RFC3339)}
		if guestTransitionMatches(in.Config, ev) {
			payload := guestTransitionPayload(ev, false)
			payload["matched"] = true
			msg["payload"] = payload
			break
		}
	}
	return plugin.NodeExecOutput{Port: "output", Msg: msg}, nil
}

func trimmed(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// stringList reads a multiselect value: []any from a saved canvas, or a JSON array or
// comma list typed as text, with blanks dropped.
func stringList(v any) []string {
	var raw []string
	switch list := v.(type) {
	case []string:
		raw = list
	case []any:
		for _, item := range list {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	case string:
		text := strings.TrimSpace(list)
		if text == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			raw = strings.Split(text, ",")
		}
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
