// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Workflow nodes contributed by the Kubernetes add-on.
//
// A node is a canvas config plus one Service call. It answers on the
// "output" port with its result in msg.payload, or on the "error" port
// with a short reason — never with a Go error for an operational
// failure, because that would abort the run and the author wired the
// error port to decide what happens next.
//
// The "cluster" field names one of the plugin's saved clusters by id;
// empty means the default cluster, exactly as the plugin's own pages
// resolve it (ClientPool.ResolveClusterID).
//
// RegisterNodes runs before Init, so every executor reads p.svc at call
// time rather than capturing it.

const (
	nodeK8sScaleDeployment  = "kubernetes-scale-deployment"
	nodeK8sDeploymentAction = "kubernetes-deployment-action"
	nodeK8sDeletePod        = "kubernetes-delete-pod"
	nodeK8sPodLogs          = "kubernetes-pod-logs"
	nodeK8sSummary          = "kubernetes-summary"

	// nodeTimeout caps one node's API round trips. The Service bounds each
	// client-go call on its own; this is the whole node.
	nodeTimeout = 2 * time.Minute
	// nodeLogsMaxBytes caps what a logs node puts in msg.payload. A
	// workflow message is not a log archive; the pod-logs page is.
	nodeLogsMaxBytes = 1 << 20
)

func (p *KubernetesPlugin) kubernetesNodes() []struct {
	key  string
	desc plugin.NodeDescriptor
} {
	cluster := map[string]any{
		"key": "cluster", "label": "Cluster ID (empty = default)", "type": "text", "required": false,
	}
	namespace := map[string]any{
		"key": "namespace", "label": "Namespace", "type": "text", "required": true, "default": "default",
	}
	return []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{nodeK8sScaleDeployment, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Kubernetes: Scale Deployment",
			Description: "Set the replica count of a Deployment.",
			Icon:        "IconGauge",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "kubernetes.manage",
			Fields: []map[string]any{
				cluster, namespace,
				{"key": "name", "label": "Deployment", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-kubernetes", "target": "k8s_deployment"}},
				{"key": "replicas", "label": "Replicas", "type": "number", "required": true, "default": 1},
			},
			Execute: p.execScaleDeployment,
		}},
		{nodeK8sDeploymentAction, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Kubernetes: Deployment Action",
			Description: "Restart (rollout) or delete a Deployment.",
			Icon:        "IconTopologyComplex",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "kubernetes.manage",
			Fields: []map[string]any{
				cluster, namespace,
				{"key": "name", "label": "Deployment", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-kubernetes", "target": "k8s_deployment"}},
				{"key": "action", "label": "Action", "type": "select", "required": true, "default": "restart",
					"options": []map[string]any{
						{"value": "restart", "label": "Rollout restart"},
						{"value": "delete", "label": "Delete"},
					}},
			},
			Execute: p.execDeploymentAction,
		}},
		{nodeK8sDeletePod, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Kubernetes: Delete Pod",
			Description: "Delete a Pod so its controller reschedules it.",
			Icon:        "IconTrashX",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "kubernetes.delete",
			Fields: []map[string]any{
				cluster, namespace,
				{"key": "name", "label": "Pod", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-kubernetes", "target": "k8s_pod"}},
			},
			Execute: p.execDeletePod,
		}},
		{nodeK8sPodLogs, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Kubernetes: Pod Logs",
			Description: "Read the tail of a Pod's logs into msg.payload.",
			Icon:        "IconFileText",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "kubernetes.logs",
			Fields: []map[string]any{
				cluster, namespace,
				{"key": "name", "label": "Pod", "type": "plugin-select", "required": true, "allowCustom": true, "source": map[string]any{"plugin": "addon-kubernetes", "target": "k8s_pod"}},
				{"key": "container", "label": "Container (optional)", "type": "text", "required": false},
				{"key": "tail", "label": "Tail lines", "type": "number", "required": false, "default": 200},
				{"key": "previous", "label": "Previous container instance", "type": "toggle", "required": false, "default": false},
			},
			Execute: p.execPodLogs,
		}},
		{nodeK8sSummary, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Kubernetes: Cluster Summary",
			Description: "Read workload, node and event counts for a cluster into msg.payload.",
			Icon:        "IconTopologyComplex",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "kubernetes.view",
			Fields: []map[string]any{
				cluster,
				{"key": "namespace", "label": "Namespace (empty = all)", "type": "text", "required": false},
				{"key": "detail", "label": "Include full object lists", "type": "toggle", "required": false, "default": false},
			},
			Execute: p.execSummary,
		}},
	}
}

// --- executors -------------------------------------------------------------

func (p *KubernetesPlugin) execScaleDeployment(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	cluster, ns, name := nodeTarget(in)
	if ns == "" || name == "" {
		return nodeFail(in, "namespace and deployment are required"), nil
	}
	replicas := nodeInt(in.Config, "replicas", -1)
	if replicas < 0 {
		return nodeFail(in, "replicas must be zero or more"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	if err := svc.ScaleDeployment(ctx, cluster, ns, name, int32(replicas)); err != nil {
		p.nodeLog("kubernetes: scale deployment node failed", err, in, "namespace", ns, "name", name)
		return nodeFail(in, "scale failed"), nil
	}
	p.nodeAudit("plugin.kubernetes.node.scale_deployment", in, map[string]any{"cluster": cluster, "namespace": ns, "name": name, "replicas": replicas})
	return nodeOK(in, map[string]any{"cluster": cluster, "namespace": ns, "name": name, "replicas": replicas}), nil
}

func (p *KubernetesPlugin) execDeploymentAction(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	cluster, ns, name := nodeTarget(in)
	if ns == "" || name == "" {
		return nodeFail(in, "namespace and deployment are required"), nil
	}
	action := nodeString(in.Config, "action", "restart")
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	var err error
	switch action {
	case "restart":
		err = svc.RestartDeployment(ctx, cluster, ns, name)
	case "delete":
		err = svc.DeleteDeployment(ctx, cluster, ns, name)
	default:
		return nodeFail(in, "unknown action"), nil
	}
	if err != nil {
		p.nodeLog("kubernetes: deployment action node failed", err, in, "action", action, "namespace", ns, "name", name)
		return nodeFail(in, action+" failed"), nil
	}
	p.nodeAudit("plugin.kubernetes.node.deployment_"+action, in, map[string]any{"cluster": cluster, "namespace": ns, "name": name})
	return nodeOK(in, map[string]any{"cluster": cluster, "namespace": ns, "name": name, "action": action}), nil
}

func (p *KubernetesPlugin) execDeletePod(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	cluster, ns, name := nodeTarget(in)
	if ns == "" || name == "" {
		return nodeFail(in, "namespace and pod are required"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	if err := svc.DeletePod(ctx, cluster, ns, name); err != nil {
		p.nodeLog("kubernetes: delete pod node failed", err, in, "namespace", ns, "name", name)
		return nodeFail(in, "delete failed"), nil
	}
	p.nodeAudit("plugin.kubernetes.node.delete_pod", in, map[string]any{"cluster": cluster, "namespace": ns, "name": name})
	return nodeOK(in, map[string]any{"cluster": cluster, "namespace": ns, "name": name, "deleted": true}), nil
}

func (p *KubernetesPlugin) execPodLogs(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	cluster, ns, name := nodeTarget(in)
	if ns == "" || name == "" {
		return nodeFail(in, "namespace and pod are required"), nil
	}
	container := nodeString(in.Config, "container", "")
	tail := nodeInt(in.Config, "tail", 200)
	if tail < 1 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	previous := nodeBool(in.Config, "previous", false)
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	stream, err := svc.PodLogs(ctx, cluster, ns, name, container, int64(tail), previous)
	if err != nil {
		p.nodeLog("kubernetes: pod logs node failed", err, in, "namespace", ns, "name", name)
		return nodeFail(in, "logs failed"), nil
	}
	defer func() { _ = stream.Close() }()
	data, err := io.ReadAll(io.LimitReader(stream, nodeLogsMaxBytes))
	if err != nil {
		p.nodeLog("kubernetes: pod logs node read failed", err, in, "namespace", ns, "name", name)
		return nodeFail(in, "logs read failed"), nil
	}
	text := string(data)
	return nodeOK(in, map[string]any{
		"cluster":   cluster,
		"namespace": ns,
		"name":      name,
		"container": container,
		"tail":      tail,
		"previous":  previous,
		"truncated": len(data) >= nodeLogsMaxBytes,
		"lines":     strings.Count(text, "\n"),
		"logs":      text,
	}), nil
}

func (p *KubernetesPlugin) execSummary(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	svc, out := p.nodeService(in)
	if svc == nil {
		return out, nil
	}
	cluster := nodeString(in.Config, "cluster", "")
	ns := nodeString(in.Config, "namespace", "")
	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()
	summary, err := svc.Summary(ctx, cluster, ns)
	if err != nil {
		p.nodeLog("kubernetes: summary node failed", err, in, "namespace", ns)
		return nodeFail(in, "summary failed"), nil
	}
	if !summary.Connected {
		o := nodeFail(in, "cluster not connected")
		o.Msg["payload"].(map[string]any)["reason"] = summary.Reason
		return o, nil
	}
	if nodeBool(in.Config, "detail", false) {
		return nodeOK(in, nodeJSON(summary)), nil
	}
	// The compact shape is what a condition or notification node wants:
	// counts and the version, not every pod in the cluster.
	return nodeOK(in, map[string]any{
		"cluster":        cluster,
		"connected":      summary.Connected,
		"version":        summary.Version,
		"namespaceScope": summary.NamespaceScope,
		"counts":         summary.Counts,
		"nodes":          len(summary.Nodes),
		"pods":           len(summary.Pods),
		"deployments":    len(summary.Deployments),
		"events":         len(summary.Events),
	}), nil
}

// --- helpers ---------------------------------------------------------------

// nodeService resolves the Service for a node call. It is nil — with an
// error-port output ready to return — until Init has run, which can only
// happen if a workflow fires during boot.
func (p *KubernetesPlugin) nodeService(in plugin.NodeExecInput) (*Service, plugin.NodeExecOutput) {
	if p.svc == nil {
		return nil, nodeFail(in, "kubernetes plugin not ready")
	}
	return p.svc, plugin.NodeExecOutput{}
}

// nodeTarget reads the cluster, namespace and object name. The picker stores an object as
// namespace/name, which a Kubernetes name can never contain, so that form wins over the namespace
// field and a bare name from a workflow saved before the picker still resolves.
func nodeTarget(in plugin.NodeExecInput) (cluster, namespace, name string) {
	cluster = nodeString(in.Config, "cluster", "")
	namespace = nodeString(in.Config, "namespace", "")
	name = nodeString(in.Config, "name", "")
	if ns, obj, ok := strings.Cut(name, "/"); ok && ns != "" && obj != "" {
		return cluster, ns, obj
	}
	return cluster, namespace, name
}

func (p *KubernetesPlugin) nodeLog(msg string, err error, in plugin.NodeExecInput, kv ...any) {
	if p.host.Logger == nil {
		return
	}
	args := append([]any{"error", err, "node", in.Key, "workflow", in.WorkflowID}, kv...)
	p.host.Logger.Error(msg, args...)
}

func (p *KubernetesPlugin) nodeAudit(event string, in plugin.NodeExecInput, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["workflow_id"] = in.WorkflowID
	fields["user_id"] = in.UserID
	_ = p.host.Audit.Emit(event, fields)
}

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

// nodeJSON turns a typed struct into the generic map shape downstream
// nodes (json-path, change, condition) can walk. A marshal failure is
// impossible for the plugin's own JSON-tagged types; the fallback keeps
// the payload rather than dropping it.
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

func nodeString(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key]; ok && v != nil {
		if t := strings.TrimSpace(fmt.Sprint(v)); t != "" {
			return t
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
