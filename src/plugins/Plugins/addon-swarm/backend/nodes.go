// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Workflow nodes contributed by the Swarm add-on.
//
// Each node is a thin adapter from a canvas config to one Service call.
// The engine hands the node its saved config and the incoming msg; the
// node clones the msg, writes its result to payload and answers on the
// "output" port, or writes a short reason to payload and answers on the
// "error" port. It never returns a Go error for an operational failure
// — that would abort the whole run, and a workflow author wired the
// error port precisely so they could decide what happens next.
//
// The handler is attached at Init, after the loader has already called
// RegisterNodes, so every executor reads p.handler.svc at call time
// rather than capturing it. See the comment on Plugin in main.go.

const (
	nodeSwarmServiceScale     = "swarm-service-scale"
	nodeSwarmServiceAction    = "swarm-service-action"
	nodeSwarmStackDeploy      = "swarm-stack-deploy"
	nodeSwarmStackRemove      = "swarm-stack-remove"
	nodeSwarmNodeAvailability = "swarm-node-availability"
	nodeSwarmOverview         = "swarm-overview"

	// nodeTimeout caps one node's Docker round trips. The Service already
	// bounds each SDK call at 30 s; this is the whole node, deploy included.
	nodeTimeout = 5 * time.Minute
)

// swarmNodes is the catalogue RegisterNodes walks. Order is the order the
// palette lists them within the plugin's category.
func (p *SwarmPlugin) swarmNodes() []struct {
	key  string
	desc plugin.NodeDescriptor
} {
	env := map[string]any{
		"key": "environment", "label": "Environment", "type": "environment-select", "required": false,
	}
	return []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{nodeSwarmServiceScale, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Scale Service",
			Description: "Set the replica count of a replicated Swarm service.",
			Icon:        "IconGauge",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm_services.scale",
			Fields: []map[string]any{
				env,
				{"key": "service", "label": "Service (name or ID)", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-swarm", "target": "swarm_service"}},
				{"key": "replicas", "label": "Replicas", "type": "number", "required": true, "default": 1},
			},
			Execute: p.execServiceScale,
		}},
		{nodeSwarmServiceAction, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Service Action",
			Description: "Restart, roll back or remove a Swarm service.",
			Icon:        "IconTopologyStarRing3",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm_services.manage",
			Fields: []map[string]any{
				env,
				{"key": "service", "label": "Service (name or ID)", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-swarm", "target": "swarm_service"}},
				{"key": "action", "label": "Action", "type": "select", "required": true, "default": "restart",
					"options": []map[string]any{
						{"value": "restart", "label": "Restart (force update)"},
						{"value": "rollback", "label": "Roll back to previous spec"},
						{"value": "remove", "label": "Remove"},
					}},
			},
			Execute: p.execServiceAction,
		}},
		{nodeSwarmStackDeploy, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Deploy Stack",
			Description: "Deploy or update a Swarm stack from Compose YAML.",
			Icon:        "IconStack2",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm_stacks.manage",
			Fields: []map[string]any{
				env,
				{"key": "name", "label": "Stack name", "type": "text", "required": true},
				{"key": "source", "label": "Compose source", "type": "select", "required": true, "default": "inline",
					"options": []map[string]any{
						{"value": "inline", "label": "Inline YAML below"},
						{"value": "msg", "label": "msg.payload (string)"},
					}},
				{"key": "compose", "label": "Compose YAML", "type": "textarea", "required": false, "showWhen": map[string]any{"source": "inline"}},
				{"key": "prune", "label": "Prune services not in the file", "type": "toggle", "required": false, "default": false},
			},
			Execute: p.execStackDeploy,
		}},
		{nodeSwarmStackRemove, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Remove Stack",
			Description: "Remove a Swarm stack and its services and networks.",
			Icon:        "IconTrashX",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm_stacks.delete",
			Fields: []map[string]any{
				env,
				{"key": "name", "label": "Stack name", "type": "text", "required": true},
			},
			Execute: p.execStackRemove,
		}},
		{nodeSwarmNodeAvailability, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Node Availability",
			Description: "Set a Swarm node to active, pause or drain.",
			Icon:        "IconServer",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm_nodes.manage",
			Fields: []map[string]any{
				env,
				{"key": "node", "label": "Node ID", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-swarm", "target": "swarm_node"}},
				{"key": "availability", "label": "Availability", "type": "select", "required": true, "default": "active",
					"options": []map[string]any{
						{"value": "active", "label": "Active"},
						{"value": "pause", "label": "Pause"},
						{"value": "drain", "label": "Drain"},
					}},
			},
			Execute: p.execNodeAvailability,
		}},
		{nodeSwarmOverview, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Swarm: Cluster Overview",
			Description: "Read node, service and task counts for the cluster into msg.payload.",
			Icon:        "IconTopologyStarRing3",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "swarm.view",
			Fields:      []map[string]any{env},
			Execute:     p.execOverview,
		}},
	}
}

// --- executors -------------------------------------------------------------

func (p *SwarmPlugin) execServiceScale(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	id := nodeString(in.Config, "service", "")
	if id == "" {
		return nodeFail(in, "service is required"), nil
	}
	replicas := nodeInt(in.Config, "replicas", -1)
	if replicas < 0 {
		return nodeFail(in, "replicas must be zero or more"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	if err := svc.ScaleService(ctx, in.EnvironmentID, id, uint64(replicas)); err != nil {
		p.nodeLog("swarm: scale service node failed", err, in)
		return nodeFail(in, "scale failed"), nil
	}
	p.nodeAudit("plugin.swarm.node.service_scale", in, map[string]any{"service": id, "replicas": replicas})
	return nodeOK(in, map[string]any{"service": id, "replicas": replicas, "environmentId": in.EnvironmentID}), nil
}

func (p *SwarmPlugin) execServiceAction(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	id := nodeString(in.Config, "service", "")
	if id == "" {
		return nodeFail(in, "service is required"), nil
	}
	action := nodeString(in.Config, "action", "restart")
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	var err error
	switch action {
	case "restart":
		err = svc.RestartService(ctx, in.EnvironmentID, id)
	case "rollback":
		err = svc.RollbackService(ctx, in.EnvironmentID, id)
	case "remove":
		err = svc.RemoveService(ctx, in.EnvironmentID, id)
	default:
		return nodeFail(in, "unknown action"), nil
	}
	if err != nil {
		p.nodeLog("swarm: service action node failed", err, in, "action", action)
		return nodeFail(in, action+" failed"), nil
	}
	p.nodeAudit("plugin.swarm.node.service_"+action, in, map[string]any{"service": id})
	return nodeOK(in, map[string]any{"service": id, "action": action, "environmentId": in.EnvironmentID}), nil
}

func (p *SwarmPlugin) execStackDeploy(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	name := nodeString(in.Config, "name", "")
	if name == "" {
		return nodeFail(in, "stack name is required"), nil
	}
	compose := ""
	switch nodeString(in.Config, "source", "inline") {
	case "msg":
		if s, ok := in.Msg["payload"].(string); ok {
			compose = s
		}
	default:
		compose = nodeString(in.Config, "compose", "")
	}
	if strings.TrimSpace(compose) == "" {
		return nodeFail(in, "compose YAML is empty"), nil
	}
	prune := nodeBool(in.Config, "prune", false)
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	result, err := svc.DeployStack(ctx, in.EnvironmentID, name, compose, prune)
	if err != nil {
		p.nodeLog("swarm: deploy stack node failed", err, in, "stack", name)
		return nodeFail(in, "deploy failed"), nil
	}
	p.nodeAudit("plugin.swarm.node.stack_deploy", in, map[string]any{
		"stack": name, "created": len(result.Created), "updated": len(result.Updated), "removed": len(result.Removed),
	})
	payload := map[string]any{
		"stack":         result.Name,
		"created":       result.Created,
		"updated":       result.Updated,
		"removed":       result.Removed,
		"networks":      result.Networks,
		"rejected":      result.Rejected,
		"environmentId": in.EnvironmentID,
	}
	if len(result.Rejected) > 0 {
		// Some services were refused by the daemon. The stack exists, so
		// this is a partial success: report it on the error port with the
		// full result attached, so the author can inspect what happened.
		o := nodeOK(in, payload)
		o.Port = "error"
		return o, nil
	}
	return nodeOK(in, payload), nil
}

func (p *SwarmPlugin) execStackRemove(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	name := nodeString(in.Config, "name", "")
	if name == "" {
		return nodeFail(in, "stack name is required"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	if err := svc.RemoveStack(ctx, in.EnvironmentID, name); err != nil {
		p.nodeLog("swarm: remove stack node failed", err, in, "stack", name)
		return nodeFail(in, "remove failed"), nil
	}
	p.nodeAudit("plugin.swarm.node.stack_remove", in, map[string]any{"stack": name})
	return nodeOK(in, map[string]any{"stack": name, "removed": true, "environmentId": in.EnvironmentID}), nil
}

func (p *SwarmPlugin) execNodeAvailability(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	id := nodeString(in.Config, "node", "")
	if id == "" {
		return nodeFail(in, "node is required"), nil
	}
	availability := nodeString(in.Config, "availability", "active")
	switch availability {
	case "active", "pause", "drain":
	default:
		return nodeFail(in, "unknown availability"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	if err := svc.SetNodeAvailability(ctx, in.EnvironmentID, id, availability); err != nil {
		p.nodeLog("swarm: node availability node failed", err, in, "node", id)
		return nodeFail(in, "update failed"), nil
	}
	p.nodeAudit("plugin.swarm.node.node_availability", in, map[string]any{"node": id, "availability": availability})
	return nodeOK(in, map[string]any{"node": id, "availability": availability, "environmentId": in.EnvironmentID}), nil
}

func (p *SwarmPlugin) execOverview(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	overview, err := svc.Overview(ctx, in.EnvironmentID)
	if err != nil {
		p.nodeLog("swarm: overview node failed", err, in)
		return nodeFail(in, "overview failed"), nil
	}
	return nodeOK(in, nodeJSON(overview)), nil
}

// --- helpers ---------------------------------------------------------------

// nodeService resolves the Swarm service for a node call. It is nil —
// with an error-port output ready to return — until Init has attached
// the host, which can only happen if a workflow fires during boot.
func (p *SwarmPlugin) nodeService(in plugin.NodeExecInput) (*Service, plugin.NodeExecOutput) {
	if p.handler == nil || p.handler.svc == nil {
		return nil, nodeFail(in, "swarm plugin not ready")
	}
	return p.handler.svc, plugin.NodeExecOutput{}
}

func (p *SwarmPlugin) nodeLog(msg string, err error, in plugin.NodeExecInput, kv ...any) {
	if p.host.Logger == nil {
		return
	}
	args := append([]any{"error", err, "node", in.Key, "env", in.EnvironmentID, "workflow", in.WorkflowID}, kv...)
	p.host.Logger.Error(msg, args...)
}

func (p *SwarmPlugin) nodeAudit(event string, in plugin.NodeExecInput, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["environment_id"] = in.EnvironmentID
	fields["workflow_id"] = in.WorkflowID
	fields["user_id"] = in.UserID
	_ = p.host.Audit.Emit(event, fields)
}

// nodeJSON turns a typed struct into the generic map shape downstream
// nodes (json-path, change, condition) can walk.
func nodeJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

// cloneNodeMsg shallow-copies the incoming msg so the node never writes
// into the message another branch of the flow is still reading.
func cloneNodeMsg(msg map[string]any) map[string]any {
	out := make(map[string]any, len(msg)+1)
	for k, v := range msg {
		out[k] = v
	}
	return out
}

func nodeOK(in plugin.NodeExecInput, payload any) plugin.NodeExecOutput {
	out := cloneNodeMsg(in.Msg)
	out["payload"] = payload
	return plugin.NodeExecOutput{Port: "output", Msg: out}
}

func nodeFail(in plugin.NodeExecInput, reason string) plugin.NodeExecOutput {
	out := cloneNodeMsg(in.Msg)
	out["payload"] = map[string]any{"error": reason, "node": in.Key}
	return plugin.NodeExecOutput{Port: "error", Msg: out}
}

func nodeString(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key]; ok && v != nil {
		switch s := v.(type) {
		case string:
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		default:
			if t := strings.TrimSpace(fmt.Sprint(s)); t != "" {
				return t
			}
		}
	}
	return def
}

func nodeInt(cfg map[string]any, key string, def int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func nodeBool(cfg map[string]any, key string, def bool) bool {
	switch v := cfg[key].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return def
}
