// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// workflowDraftStatus is the editor's "inactive" state. The trigger service
// only ever loads status 'active', so a draft never fires on its own.
const workflowDraftStatus = "draft"

// wfStep is one node of a seeded workflow; port is the output that leads on.
type wfStep struct {
	action string
	label  string
	config map[string]any
	port   string
}

// workflowTemplate is one seeded workflow. Trigger nodes are real node types,
// but the workflow is stored as a draft so none of them is ever armed.
type workflowTemplate struct {
	name, description string
	steps             func(h *historyHost) []wfStep
}

func trig(action, label string, cfg map[string]any) wfStep {
	return wfStep{action: action, label: label, config: cfg, port: "output"}
}

func step(action, label string, cfg map[string]any) wfStep {
	return wfStep{action: action, label: label, config: cfg, port: "output"}
}

func firstStack(h *historyHost) string {
	if len(h.stacks) == 0 {
		return "shop"
	}
	return h.stacks[0]
}

func firstContainer(h *historyHost) string {
	if len(h.containers) == 0 {
		return ""
	}
	return h.containers[len(h.containers)/2].Name
}

var workflowTemplates = []workflowTemplate{
	{"Auto-heal unhealthy containers", "Restart a container after its health check turns unhealthy", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("container-trigger", "Unhealthy", map[string]any{"environment": h.envID, "container": firstContainer(h), "watch": "health", "health": "unhealthy"}),
			step("delay", "Grace period", map[string]any{"seconds": 30}),
			step("container-action", "Restart", map[string]any{"environment": h.envID, "container": firstContainer(h), "action": "restart", "timeout": 10}),
			step("send-notification", "Tell on-call", map[string]any{"title": "Auto-heal", "message": "Restarted {{payload.container}}"}),
		}
	}},
	{"Nightly stack backup", "Back up the primary stack every night at 02:00", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("schedule-trigger", "02:00 daily", map[string]any{"cron": "0 2 * * *", "timezone": "Europe/Amsterdam"}),
			step("stack-backup", "Back up stack", map[string]any{"environment": h.envID, "stack_name": firstStack(h)}),
			step("log", "Record result", map[string]any{"level": "info", "message": "Backup finished"}),
		}
	}},
	{"Image update notify", "Notify when a newer tag of a watched image is published", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("registry-tag-trigger", "New tag", map[string]any{"environment": h.envID, "image": "ghcr.io/acme/shop-api", "interval": 3600}),
			step("send-notification", "Notify", map[string]any{"title": "Image update", "message": "{{payload.image}} has a new tag"}),
		}
	}},
	{"Weekly prune", "Prune dangling images and stopped containers on Sunday night", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("schedule-trigger", "Sunday 03:30", map[string]any{"cron": "30 3 * * 0"}),
			step("docker-prune", "Prune images", map[string]any{"environment": h.envID, "resource": "images"}),
			step("docker-prune", "Prune containers", map[string]any{"environment": h.envID, "resource": "containers"}),
			step("log", "Log", map[string]any{"level": "info", "message": "Weekly prune complete"}),
		}
	}},
	{"Deploy on git push", "Redeploy the stack when the repository webhook fires", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("webhook-trigger", "Git push", map[string]any{"method": "POST", "path": "deploy-" + firstStack(h)}),
			{action: "condition", label: "Branch is main", config: map[string]any{"field": "payload.ref", "operator": "equals", "value": "refs/heads/main"}, port: "true"},
			step("stack-manage", "Redeploy", map[string]any{"environment": h.envID, "action": "up", "stack_name": firstStack(h)}),
		}
	}},
	{"Scale on alert", "Restart the worker when CPU stays above 90%", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("metric-trigger", "CPU > 90%", map[string]any{"environment": h.envID, "container": firstContainer(h), "logic": "and", "interval": 60, "cooldown": 900,
				"conditions": []map[string]any{{"metric": "cpu", "operator": ">", "value": 90}}}),
			step("container-action", "Restart worker", map[string]any{"environment": h.envID, "container": firstContainer(h), "action": "restart"}),
		}
	}},
	{"Crash loop triage", "Collect logs and stats when a container dies", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("docker-event-trigger", "Container died", map[string]any{"environment": h.envID, "type": "container", "action": "die"}),
			step("container-logs", "Last logs", map[string]any{"environment": h.envID, "container": "{{payload.name}}", "tail": 200}),
			step("container-stats", "Stats", map[string]any{"environment": h.envID, "container": "{{payload.name}}"}),
			step("send-notification", "Summarise", map[string]any{"title": "Crash", "message": "{{payload.name}} died"}),
		}
	}},
	{"Vulnerability scan", "Scan the storefront image and flag critical findings", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("manual-trigger", "Run", map[string]any{}),
			{action: "image-vulnerability-scan", label: "Scan", config: map[string]any{"environment": h.envID, "image": "ghcr.io/acme/storefront:4.12.3", "wait": true}, port: "critical"},
			step("send-notification", "Critical found", map[string]any{"title": "Critical CVE", "message": "Storefront image has critical findings"}),
		}
	}},
	{"Restart stack", "Manually restart every service of a stack", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("manual-trigger", "Run", map[string]any{}),
			step("stack-manage", "Down", map[string]any{"environment": h.envID, "action": "down", "stack_name": firstStack(h)}),
			step("stack-manage", "Up", map[string]any{"environment": h.envID, "action": "up", "stack_name": firstStack(h)}),
		}
	}},
	{"Container backup", "Back up a stateful container's volumes", func(h *historyHost) []wfStep {
		return []wfStep{
			trig("schedule-trigger", "01:15 daily", map[string]any{"cron": "15 1 * * *"}),
			step("container-backup-run", "Back up", map[string]any{"environment": h.envID, "container": firstContainer(h), "include_logs": false}),
		}
	}},
}

// workflowVariants multiplies the templates across hosts so the list reads
// like a real estate: the same automation, owned per environment.
const workflowVariants = 4

// seedHistoryWorkflows writes the seeded workflows as drafts, with a short
// run history from when someone last ran them by hand.
func seedHistoryWorkflows(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "workflows")
	if err != nil || !ok {
		return err
	}
	runsOK, err := tableExists(ctx, conn, "workflow_runs")
	if err != nil {
		return err
	}
	var wfRows, runRows [][]any
	for v := 0; v < workflowVariants; v++ {
		for i, t := range workflowTemplates {
			h := &hc.hosts[(i+v*len(workflowTemplates))%len(hc.hosts)]
			name := t.name
			if v > 0 {
				name = t.name + " (" + h.host.Name + ")"
			}
			id := stableID("workflow", name)
			canvas, err := workflowCanvas(t.steps(h))
			if err != nil {
				return err
			}
			author := hc.pickActor()
			created := hc.anyTime(hc.window)
			updated := created.Add(time.Duration(hc.rng.Int64N(int64(hc.now.Sub(created)) + 1))).Truncate(time.Second)
			lastRun := ""
			for r := 0; r < hc.rng.IntN(7); r++ {
				started := updated.Add(-time.Duration(hc.rng.Int64N(int64(20 * 24 * time.Hour)))).Truncate(time.Second)
				if started.Before(created) {
					started = created
				}
				status, errMsg := "completed", ""
				switch roll := hc.rng.Float64(); {
				case roll < 0.14:
					status, errMsg = "failed", pick(hc, []string{"node \"Restart\": container is marked as protected", "node \"Back up stack\": storage location is unreachable", "node \"Scan\": scanner timed out after 300s"})
				case roll < 0.18:
					status = "cancelled"
				}
				dur := int64(300 + hc.rng.IntN(40000))
				runRows = append(runRows, []any{stableID("workflow_run", fmt.Sprintf("%s/%d", name, r)), id, status,
					pick(hc, []string{"manual", "manual", "auto", "webhook"}), dur, len(t.steps(h)), errMsg,
					rfc(started), rfc(started.Add(time.Duration(dur) * time.Millisecond))})
				if s := rfc(started); s > lastRun {
					lastRun = s
				}
			}
			wfRows = append(wfRows, []any{id, name, t.description, workflowDraftStatus, canvas, author.username, author.username, lastRun, rfc(created), rfc(updated)})
		}
	}
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := insertAll(ctx, tx, `
			INSERT OR IGNORE INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, '{}', ?, ?, ?, ?, ?)`, wfRows); err != nil {
			return err
		}
		if !runsOK {
			return nil
		}
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO workflow_runs (id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, runRows)
	})
}

// workflowCanvas lays steps out left to right in the editor's saved shape.
func workflowCanvas(steps []wfStep) (string, error) {
	type node struct {
		ID       string         `json:"id"`
		Type     string         `json:"type"`
		Action   string         `json:"action"`
		Label    string         `json:"label"`
		Config   map[string]any `json:"config"`
		Position map[string]int `json:"position"`
	}
	type edge struct {
		ID           string `json:"id"`
		SourceNodeID string `json:"sourceNodeId"`
		SourcePort   string `json:"sourcePort"`
		TargetNodeID string `json:"targetNodeId"`
		TargetPort   string `json:"targetPort"`
	}
	canvas := struct {
		Nodes    []node         `json:"nodes"`
		Edges    []edge         `json:"edges"`
		Groups   []any          `json:"groups"`
		Viewport map[string]int `json:"viewport"`
	}{Nodes: []node{}, Edges: []edge{}, Groups: []any{}, Viewport: map[string]int{"x": 0, "y": 0, "zoom": 1}}
	for i, s := range steps {
		kind := "action"
		if i == 0 {
			kind = "trigger"
		}
		canvas.Nodes = append(canvas.Nodes, node{ID: fmt.Sprintf("n%d", i+1), Type: kind, Action: s.action, Label: s.label,
			Config: s.config, Position: map[string]int{"x": 80 + i*280, "y": 160 + (i%2)*40}})
		if i > 0 {
			canvas.Edges = append(canvas.Edges, edge{ID: fmt.Sprintf("e%d", i), SourceNodeID: fmt.Sprintf("n%d", i),
				SourcePort: steps[i-1].port, TargetNodeID: fmt.Sprintf("n%d", i+1), TargetPort: "input"})
		}
	}
	data, err := json.Marshal(canvas)
	if err != nil {
		return "", fmt.Errorf("encoding workflow canvas: %w", err)
	}
	return string(data), nil
}
