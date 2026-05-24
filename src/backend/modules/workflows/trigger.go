// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/events"
	dockerclient "github.com/docker/docker/client"

	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TriggerService watches Docker events and auto-triggers matching workflows.
type TriggerService struct {
	app        *router.AppDeps
	hub        *Hub
	service    *Service
	handler    *Handler
	logger     *slog.Logger
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	cooldown   sync.Map // key: "workflowID:nodeID" -> value: time.Time (last trigger time)
	cronFired  sync.Map // key: "workflowID:nodeID" -> value: time bucket string
	fileStates sync.Map // key: "workflowID:nodeID:path" -> value: fileWatchState
}

type fileWatchState struct {
	Exists  bool
	Size    int64
	ModTime time.Time
}

// SetCustomExecutor propagates the custom node executor to all workflow service instances.
func (ts *TriggerService) SetCustomExecutor(executor interface {
	ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error)
	IsCustomNode(key string) bool
}) {
	ts.service.SetCustomExecutor(executor)
	if ts.handler != nil {
		ts.handler.service.SetCustomExecutor(executor)
	}
}

// NewTriggerService creates a new background trigger service.
func NewTriggerService(app *router.AppDeps, hub *Hub) *TriggerService {
	svc := NewService(app.DB, app.DockerPool, app.Logger, app.Encryption, corenotify.NewDispatcher(app.DB, app.Encryption))
	return &TriggerService{
		app:     app,
		hub:     hub,
		service: svc,
		logger:  app.Logger.With("component", "workflow-trigger"),
	}
}

// Start begins listening for Docker events and metric polling in the background.
func (ts *TriggerService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel
	ts.wg.Add(4)
	go ts.run(ctx)
	go ts.runMetricWatcher(ctx)
	go ts.runTimeWatcher(ctx)
	go ts.runFileWatcher(ctx)
	ts.logger.Info("workflow trigger service started")
}

// Stop gracefully shuts down the trigger service.
func (ts *TriggerService) Stop() {
	if ts.cancel != nil {
		ts.cancel()
	}
	ts.wg.Wait()
	ts.logger.Info("workflow trigger service stopped")
}

func (ts *TriggerService) run(ctx context.Context) {
	defer ts.wg.Done()

	// Track which environments already have a watcher goroutine
	watching := make(map[string]bool)

	for {
		envIDs, err := ts.service.ListActiveDockerEnvIDs()
		if err != nil {
			ts.logger.Error("workflow trigger: query environments error", "error", err)
		}

		// Launch a watcher for each new environment (each has its own retry loop)
		for _, envID := range envIDs {
			if watching[envID] {
				continue
			}
			watching[envID] = true
			ts.wg.Add(1)
			go ts.watchEnv(ctx, envID)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			// Re-discover environments periodically (picks up newly added ones)
		}
	}
}

// watchEnv watches a single environment with its own retry loop.
func (ts *TriggerService) watchEnv(ctx context.Context, envID string) {
	defer ts.wg.Done()

	for {
		ts.listen(ctx, envID)

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			// Retry connection to this environment
		}
	}
}

func (ts *TriggerService) listen(ctx context.Context, envID string) {
	cli, err := ts.app.DockerPool.Get(envID)
	if err != nil {
		ts.logger.Warn("workflow trigger: cannot get docker client", "error", err, "env", envID)
		return
	}

	eventsCh, errCh := cli.Events(ctx, events.ListOptions{})

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				ts.logger.Warn("workflow trigger: event stream error", "error", err, "env", envID)
			}
			return
		case evt := <-eventsCh:
			if string(evt.Type) == "container" {
				ts.handleContainerEvent(ctx, cli, evt, envID)
			}
		}
	}
}

func (ts *TriggerService) handleContainerEvent(ctx context.Context, cli *dockerclient.Client, evt events.Message, envID string) {
	action := string(evt.Action)
	// Normalize action: Docker may include suffixes like "exec_start: /bin/bash"
	if idx := strings.Index(action, ":"); idx != -1 {
		action = strings.TrimSpace(action[:idx])
	}

	containerName := evt.Actor.Attributes["name"]
	containerID := evt.Actor.ID

	// Find workflows with active container-status-trigger nodes matching this event
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("workflow trigger: query error", "error", err)
		return
	}

	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}

		// Build set of blocked node IDs
		blockedNodeIDs := make(map[string]bool)
		for _, g := range canvas.Groups {
			if g.Blocked {
				for _, nid := range g.NodeIDs {
					blockedNodeIDs[nid] = true
				}
			}
		}

		for _, node := range canvas.Nodes {
			if node.Action != "container-status-trigger" {
				continue
			}
			if node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}

			cfgContainer, _ := node.Config["container"].(string)
			cfgStatus, _ := node.Config["status"].(string)
			if cfgStatus == "" {
				cfgStatus = "any"
			}

			// Match container name or ID
			if cfgContainer != "" && cfgContainer != containerName && cfgContainer != containerID {
				continue
			}

			// Match event type
			if cfgStatus != "any" && cfgStatus != action {
				continue
			}

			// Trigger workflow execution in background
			ts.logger.Info("auto-triggering workflow",
				"workflow", aw.ID,
				"trigger_node", node.ID,
				"container", containerName,
				"event", action,
			)

			go ts.executeWorkflow(ctx, aw.ID, node.ID, envID, containerName, action, evt)
		}
	}
}

func (ts *TriggerService) executeWorkflow(ctx context.Context, workflowID, triggerNodeID, envID, containerName, eventAction string, evt events.Message) {
	triggerMsg := NewMsg(map[string]interface{}{
		"container":       containerName,
		"statusEvent":     eventAction,
		"event":           eventAction,
		"eventTime":       evt.Time,
		"eventActor":      evt.Actor.ID,
		"eventAttributes": evt.Actor.Attributes,
		"autoTriggered":   true,
	})
	triggerMsg["topic"] = "container-status"
	ts.executeWorkflowWithOutput(ctx, workflowID, triggerNodeID, envID, triggerMsg)
}

func (ts *TriggerService) executeWorkflowWithOutput(ctx context.Context, workflowID, triggerNodeID, envID string, triggerMsg Msg) {
	emit := func(event string, data interface{}) {
		payload, err := json.Marshal(data)
		if err != nil {
			ts.logger.Error("workflow trigger: marshal event failed", "error", err, "workflow", workflowID, "event", event)
			return
		}
		ts.hub.Publish(workflowID, ExecutionEvent{Event: event, Data: payload})
	}

	wf, err := ts.service.Get(workflowID)
	if err != nil {
		ts.logger.Error("workflow trigger: load error", "error", err, "workflow", workflowID)
		return
	}
	if wf == nil {
		ts.logger.Error("workflow trigger: workflow not found", "workflow", workflowID)
		return
	}

	result := ts.service.ExecuteWorkflow(ctx, wf, workflowRunOptions{
		WorkflowID:    workflowID,
		Trigger:       "auto",
		StartNodeID:   triggerNodeID,
		StartMsg:      triggerMsg,
		FallbackEnvID: envID,
	}, emit)

	ts.service.RecordRun(workflowID, "auto", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
	ts.logger.Info("workflow trigger: execution completed", "workflow", workflowID)
}

// triggerLinkInWorkflows finds all workflows with link-in nodes referencing the
// given source (workflowID:nodeID) and auto-executes them with the stored msg.
func (ts *TriggerService) triggerLinkInWorkflows(ctx context.Context, sourceWorkflowID, sourceNodeID string, msg Msg) {
	sourceKey := sourceWorkflowID + ":" + sourceNodeID

	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("link-out trigger: query error", "error", err)
		return
	}

	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}

		for _, node := range canvas.Nodes {
			if node.Action != "link-in" || node.Disabled {
				continue
			}
			source, _ := node.Config["source"].(string)
			if source != sourceKey {
				continue
			}

			ts.logger.Info("link-out auto-triggering workflow",
				"source_workflow", sourceWorkflowID,
				"target_workflow", aw.ID,
				"link_in_node", node.ID,
			)

			go ts.executeLinkInWorkflow(ctx, aw.ID, node.ID, msg)
		}
	}
}

// executeLinkInWorkflow runs a workflow starting from a specific link-in node with the given msg.
func (ts *TriggerService) executeLinkInWorkflow(ctx context.Context, workflowID, linkInNodeID string, storedMsg Msg) {
	emit := func(event string, data interface{}) {
		payload, err := json.Marshal(data)
		if err != nil {
			ts.logger.Error("link-out trigger: marshal event failed", "error", err, "workflow", workflowID, "event", event)
			return
		}
		ts.hub.Publish(workflowID, ExecutionEvent{Event: event, Data: payload})
	}

	wf, err := ts.service.Get(workflowID)
	if err != nil || wf == nil {
		return
	}

	result := ts.service.ExecuteWorkflow(ctx, wf, workflowRunOptions{
		WorkflowID:    workflowID,
		Trigger:       "link",
		StartNodeID:   linkInNodeID,
		StartInputMsg: storedMsg,
	}, emit)

	ts.service.RecordRun(workflowID, "link", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
	ts.logger.Info("link-in auto-execution completed", "workflow", workflowID)
}

// ---------- Metric-based trigger watcher ----------

// runMetricWatcher polls active workflows with metric-trigger nodes on a 10-second tick.
func (ts *TriggerService) runMetricWatcher(ctx context.Context) {
	defer ts.wg.Done()

	// Track last check time per workflow:nodeID
	lastChecked := make(map[string]time.Time)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.checkMetricTriggers(ctx, lastChecked)
		}
	}
}

// checkMetricTriggers scans active workflows for metric-trigger nodes and evaluates conditions.
func (ts *TriggerService) checkMetricTriggers(ctx context.Context, lastChecked map[string]time.Time) {
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("metric watcher: query error", "error", err)
		return
	}

	// Collect all metric trigger nodes grouped by (envID, container) to deduplicate stats calls
	type triggerInfo struct {
		workflowID string
		node       CanvasNode
		envID      string
		container  string
	}

	var triggers []triggerInfo

	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}

		// Build blocked node IDs
		blockedNodeIDs := make(map[string]bool)
		for _, g := range canvas.Groups {
			if g.Blocked {
				for _, nid := range g.NodeIDs {
					blockedNodeIDs[nid] = true
				}
			}
		}

		for _, node := range canvas.Nodes {
			if node.Action != "metric-trigger" || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}

			envID, _ := node.Config["environment"].(string)
			containerName, _ := node.Config["container"].(string)
			if envID == "" || containerName == "" {
				continue
			}

			// Check interval
			interval := configFloat(node.Config, "interval", 30)
			if interval < 10 {
				interval = 10
			}

			key := aw.ID + ":" + node.ID
			if last, ok := lastChecked[key]; ok {
				if time.Since(last).Seconds() < interval {
					continue
				}
			}

			lastChecked[key] = time.Now()
			triggers = append(triggers, triggerInfo{
				workflowID: aw.ID,
				node:       node,
				envID:      envID,
				container:  containerName,
			})
		}
	}

	if len(triggers) == 0 {
		return
	}

	// Deduplicate stats fetches by (envID, container)
	type statsKey struct{ envID, container string }
	snapshots := make(map[statsKey]*containerMetricSnapshot)
	var mu sync.Mutex
	var wg sync.WaitGroup

	seen := make(map[statsKey]bool)
	for _, t := range triggers {
		sk := statsKey{t.envID, t.container}
		if seen[sk] {
			continue
		}
		seen[sk] = true

		wg.Add(1)
		go func(sk statsKey) {
			defer wg.Done()

			cli, err := ts.app.DockerPool.Get(sk.envID)
			if err != nil {
				ts.logger.Warn("metric watcher: docker client error", "error", err, "env", sk.envID)
				return
			}

			snap, err := FetchContainerMetric(ctx, cli, sk.container)
			if err != nil {
				ts.logger.Warn("metric watcher: stats fetch error", "error", err, "container", sk.container)
				return
			}

			mu.Lock()
			snapshots[sk] = snap
			mu.Unlock()
		}(sk)
	}
	wg.Wait()

	// Evaluate conditions and trigger matching workflows
	for _, t := range triggers {
		sk := statsKey{t.envID, t.container}
		snap, ok := snapshots[sk]
		if !ok {
			continue
		}

		if !evaluateMetricConditions(t.node.Config, snap) {
			continue
		}

		// Check cooldown
		cooldownSec := configFloat(t.node.Config, "cooldown", 300)
		cooldownKey := t.workflowID + ":" + t.node.ID
		if lastTime, ok := ts.cooldown.Load(cooldownKey); ok {
			if time.Since(lastTime.(time.Time)).Seconds() < cooldownSec {
				continue
			}
		}
		ts.cooldown.Store(cooldownKey, time.Now())

		ts.logger.Info("metric trigger firing",
			"workflow", t.workflowID,
			"node", t.node.ID,
			"container", t.container,
			"cpu", fmt.Sprintf("%.1f%%", snap.CPUPercent),
			"mem", fmt.Sprintf("%.1f%%", snap.MemPercent),
		)

		triggerMsg := NewMsg(map[string]interface{}{
			"container":     t.container,
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
			"autoTriggered": true,
			"cpu_percent":   snap.CPUPercent,
			"mem_percent":   snap.MemPercent,
			"mem_usage":     snap.MemUsage,
			"mem_limit":     snap.MemLimit,
			"net_rx":        snap.NetRx,
			"net_tx":        snap.NetTx,
			"block_read":    snap.BlockRead,
			"block_write":   snap.BlockWrite,
			"pids":          snap.PIDs,
		})
		triggerMsg["topic"] = "metric"

		go ts.executeWorkflowWithOutput(ctx, t.workflowID, t.node.ID, t.envID, triggerMsg)
	}
}

// evaluateMetricConditions evaluates all metric conditions using AND/OR logic.
func evaluateMetricConditions(config map[string]interface{}, snap *containerMetricSnapshot) bool {
	conditionsRaw, ok := config["conditions"]
	if !ok {
		return false
	}

	conditionsJSON, err := json.Marshal(conditionsRaw)
	if err != nil {
		return false
	}

	var conditions []struct {
		Metric    string  `json:"metric"`
		Operator  string  `json:"operator"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
		return false
	}

	if len(conditions) == 0 {
		return false
	}

	logic, _ := config["logic"].(string)
	if logic == "" {
		logic = "AND"
	}

	for _, cond := range conditions {
		actual := getMetricValue(cond.Metric, snap)
		match := compareMetric(actual, cond.Operator, cond.Threshold)

		if logic == "OR" && match {
			return true
		}
		if logic == "AND" && !match {
			return false
		}
	}

	// AND: all matched; OR: none matched
	return logic == "AND"
}

func (ts *TriggerService) runTimeWatcher(ctx context.Context) {
	defer ts.wg.Done()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.checkTimeTriggers(ctx)
		}
	}
}

func (ts *TriggerService) checkTimeTriggers(ctx context.Context) {
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("time watcher: query error", "error", err)
		return
	}

	nowUTC := time.Now().UTC()
	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}

		blockedNodeIDs := make(map[string]bool)
		for _, g := range canvas.Groups {
			if !g.Blocked {
				continue
			}
			for _, nodeID := range g.NodeIDs {
				blockedNodeIDs[nodeID] = true
			}
		}

		for _, node := range canvas.Nodes {
			if node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}
			if node.Action != "schedule-trigger" && node.Action != "cron-trigger" {
				continue
			}

			spec, _ := node.Config["cron"].(string)
			if strings.TrimSpace(spec) == "" {
				continue
			}

			runAt := nowUTC
			if node.Action == "cron-trigger" {
				if tz, _ := node.Config["timezone"].(string); strings.TrimSpace(tz) != "" {
					if loc, err := time.LoadLocation(tz); err == nil {
						runAt = nowUTC.In(loc)
					}
				}
			}

			if !cronMatchesTime(spec, runAt) {
				continue
			}

			bucket := runAt.Format("2006-01-02T15:04")
			key := aw.ID + ":" + node.ID
			if lastBucket, ok := ts.cronFired.Load(key); ok && lastBucket.(string) == bucket {
				continue
			}
			ts.cronFired.Store(key, bucket)

			triggerMsg := NewMsg(map[string]interface{}{
				"trigger":       strings.TrimSuffix(node.Action, "-trigger"),
				"cron":          spec,
				"timezone":      runAt.Location().String(),
				"timestamp":     nowUTC.Format(time.RFC3339),
				"datetime":      runAt.Format("2006-01-02 15:04:05 MST"),
				"unix":          nowUTC.Unix(),
				"autoTriggered": true,
			})
			if node.Action == "schedule-trigger" {
				triggerMsg["topic"] = "schedule"
			} else {
				triggerMsg["topic"] = "cron"
			}

			go ts.executeWorkflowWithOutput(ctx, aw.ID, node.ID, "", triggerMsg)
		}
	}
}

func (ts *TriggerService) runFileWatcher(ctx context.Context) {
	defer ts.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.checkFileWatchTriggers(ctx)
		}
	}
}

func (ts *TriggerService) checkFileWatchTriggers(ctx context.Context) {
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("file watcher: query error", "error", err)
		return
	}

	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}

		blockedNodeIDs := make(map[string]bool)
		for _, g := range canvas.Groups {
			if !g.Blocked {
				continue
			}
			for _, nodeID := range g.NodeIDs {
				blockedNodeIDs[nodeID] = true
			}
		}

		for _, node := range canvas.Nodes {
			if node.Action != "file-watch-trigger" || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}

			rawPath, _ := node.Config["path"].(string)
			if strings.TrimSpace(rawPath) == "" {
				continue
			}

			fullPath := resolveWorkflowPath(rawPath)
			current := readFileWatchState(fullPath)
			stateKey := aw.ID + ":" + node.ID + ":" + fullPath

			previousRaw, loaded := ts.fileStates.Load(stateKey)
			ts.fileStates.Store(stateKey, current)
			if !loaded {
				continue
			}

			previous := previousRaw.(fileWatchState)
			eventType := detectFileWatchEvent(previous, current)
			if eventType == "" || !matchesFileWatchEvent(node.Config["event_types"], eventType) {
				continue
			}

			triggerMsg := NewMsg(map[string]interface{}{
				"trigger":       "file-watch",
				"path":          rawPath,
				"resolvedPath":  fullPath,
				"event":         eventType,
				"timestamp":     time.Now().UTC().Format(time.RFC3339),
				"autoTriggered": true,
			})
			triggerMsg["topic"] = "file-watch"

			go ts.executeWorkflowWithOutput(ctx, aw.ID, node.ID, "", triggerMsg)
		}
	}
}

// getMetricValue maps a metric name to the snapshot field value.
func getMetricValue(name string, snap *containerMetricSnapshot) float64 {
	switch name {
	case "cpu_percent":
		return snap.CPUPercent
	case "mem_percent":
		return snap.MemPercent
	case "mem_usage_mb":
		return float64(snap.MemUsage) / (1024 * 1024)
	case "net_rx":
		return float64(snap.NetRx)
	case "net_tx":
		return float64(snap.NetTx)
	case "block_read":
		return float64(snap.BlockRead)
	case "block_write":
		return float64(snap.BlockWrite)
	case "pids":
		return float64(snap.PIDs)
	default:
		return 0
	}
}

// compareMetric compares an actual value against a threshold using the given operator.
func compareMetric(actual float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return actual > threshold
	case "<":
		return actual < threshold
	case ">=":
		return actual >= threshold
	case "<=":
		return actual <= threshold
	case "==":
		return actual == threshold
	default:
		return false
	}
}

func readFileWatchState(path string) fileWatchState {
	info, err := os.Stat(path)
	if err != nil {
		return fileWatchState{}
	}
	return fileWatchState{
		Exists:  true,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC(),
	}
}

func detectFileWatchEvent(previous, current fileWatchState) string {
	switch {
	case !previous.Exists && current.Exists:
		return "create"
	case previous.Exists && !current.Exists:
		return "remove"
	case previous.Exists && current.Exists && (previous.ModTime != current.ModTime || previous.Size != current.Size):
		return "write"
	default:
		return ""
	}
}

func matchesFileWatchEvent(raw interface{}, eventType string) bool {
	value, _ := raw.(string)
	if strings.TrimSpace(value) == "" {
		return true
	}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), eventType) {
			return true
		}
	}
	return false
}
