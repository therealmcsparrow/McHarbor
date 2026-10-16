// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/events"
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
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
	tagStates  sync.Map // key: "workflowID:nodeID:image" -> value: registry digest

	coordinator triggerCoordinator
	nodeID      string
	hasCoord    bool

	// fires relays plugin trigger fires to the workflow-trigger leader (trigger_forward.go).
	fires *fireRelay
	// startRun starts one triggered run in the background; tests replace it to count runs.
	startRun func(ctx context.Context, workflowID, triggerNodeID, envID string, triggerMsg Msg)
}

// triggerCoordinator is the subset of the cluster coordinator needed by the
// automatic workflow trigger runtime.
type triggerCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

type fileWatchState struct {
	Exists  bool
	Size    int64
	ModTime time.Time
}

type fileWatchStorageLocation struct {
	ID           string
	Name         string
	LocationType string
	BasePath     string
}

// SetPluginNodeExecutor propagates the plugin node bridge to all workflow service instances.
func (ts *TriggerService) SetPluginNodeExecutor(executor PluginNodeExecutor) {
	ts.service.SetPluginNodeExecutor(executor)
	if ts.handler != nil {
		ts.handler.service.SetPluginNodeExecutor(executor)
	}
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

// SetImageScanner propagates the image scanner runtime adapter to workflow services.
func (ts *TriggerService) SetImageScanner(scanner ImageScanner) {
	ts.service.SetImageScanner(scanner)
	if ts.handler != nil {
		ts.handler.service.SetImageScanner(scanner)
	}
}

// SetContainerBackupRuntime propagates the container-backup runtime adapter to workflow services.
func (ts *TriggerService) SetContainerBackupRuntime(r ContainerBackupRunner) {
	ts.service.SetContainerBackupRuntime(r)
	if ts.handler != nil {
		ts.handler.service.SetContainerBackupRuntime(r)
	}
}

// SetStackContainerLinkerRuntime propagates the stack linker runtime adapter to workflow services.
func (ts *TriggerService) SetStackContainerLinkerRuntime(r StackContainerLinker) {
	ts.service.SetStackContainerLinkerRuntime(r)
	if ts.handler != nil {
		ts.handler.service.SetStackContainerLinkerRuntime(r)
	}
}

// SetStorageLocationRuntime propagates the storage-location runtime adapter to workflow services.
func (ts *TriggerService) SetStorageLocationRuntime(r StorageLocationReader) {
	ts.service.SetStorageLocationRuntime(r)
	if ts.handler != nil {
		ts.handler.service.SetStorageLocationRuntime(r)
	}
}

// SetStorageObjectRuntime propagates the storage-object runtime adapter to workflow services.
func (ts *TriggerService) SetStorageObjectRuntime(r StorageObjectOperator) {
	ts.service.SetStorageObjectRuntime(r)
	if ts.handler != nil {
		ts.handler.service.SetStorageObjectRuntime(r)
	}
}

// NewTriggerService creates a new background trigger service.
func NewTriggerService(app *router.AppDeps, hub *Hub) *TriggerService {
	dispatcher := corenotify.NewDispatcher(app.DB, app.Encryption).SetPluginRuntime(app.PluginAdapters)
	svc := NewService(app.DB, app.DockerPool, app.Logger, app.Encryption, dispatcher, app.Config.DataDir, app.BackupCrypto)
	ts := &TriggerService{
		app:     app,
		hub:     hub,
		service: svc,
		logger:  app.Logger.With("component", "workflow-trigger"),
	}
	if registered, ok := app.LookupService("coordinator"); ok {
		if coord, ok := registered.(triggerCoordinator); ok {
			ts.coordinator = coord
			ts.nodeID = coord.NodeID()
			ts.hasCoord = true
		}
	}
	if app.NodeLink != nil {
		ts.fires = newFireRelay(app.NodeLink, app.DB)
		app.NodeLink.SetTriggerFireReceiver(ts)
	} else {
		ts.fires = newFireRelay(nil, app.DB)
	}
	return ts
}

// Start begins listening for Docker events and metric polling in the background.
func (ts *TriggerService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancel = cancel
	ts.wg.Add(6)
	go ts.run(ctx)
	go ts.runMetricWatcher(ctx)
	go ts.runTimeWatcher(ctx)
	go ts.runFileWatcher(ctx)
	go ts.runRegistryTagWatcher(ctx)
	go ts.runFireLedgerPrune(ctx)
	ts.logger.Info("workflow trigger service started")
}

// Stop gracefully shuts down the trigger service.
func (ts *TriggerService) Stop() {
	if ts.cancel != nil {
		ts.cancel()
	}
	if ts.fires != nil {
		ts.fires.close()
	}
	ts.wg.Wait()
	ts.logger.Info("workflow trigger service stopped")
}

func (ts *TriggerService) run(ctx context.Context) {
	defer ts.wg.Done()

	// Track which environments already have a watcher goroutine
	watching := make(map[string]bool)

	for {
		envIDs, err := ts.service.ListActiveDockerEnvIDs(ctx)
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
	cli, err := ts.app.DockerPool.GetContext(ctx, envID)
	if err != nil {
		ts.logger.Warn("workflow trigger: cannot get docker client", "error", err, "env", envID)
		return
	}

	// No context.WithTimeout: the trigger watcher subscribes to the Docker
	// event stream for the lifetime of the workflow engine. ctx is cancelled
	// on shutdown, which is the bound that applies.
	eventStream := cli.Events(ctx, dockerclient.EventsListOptions{})
	eventsCh, errCh := eventStream.Messages, eventStream.Err

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-errCh:
			if !ok {
				return
			}
			if err != nil && ctx.Err() == nil {
				if dockerEventStreamClosed(ctx, err) {
					ts.logger.Debug("workflow trigger: event stream closed", "error", err, "env", envID)
					return
				}
				ts.logger.Warn("workflow trigger: event stream error", "error", err, "env", envID)
			}
			return
		case evt, ok := <-eventsCh:
			if !ok {
				return
			}
			ts.handleDockerEvent(ctx, evt, envID)
			if string(evt.Type) == "container" {
				ts.handleContainerEvent(ctx, cli, evt, envID)
				ts.handleContainerHealthEvent(ctx, evt, envID)
			}
		}
	}
}

func dockerEventStreamClosed(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func (ts *TriggerService) handleDockerEvent(ctx context.Context, evt events.Message, envID string) {
	if !ts.leaderFor(ctx) {
		ts.logger.Debug("workflow trigger: not leader for docker event; skipping", "node", ts.nodeID)
		return
	}

	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("docker event trigger: query error", "error", err)
		return
	}

	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}
		normalizeLegacyNodes(&canvas)

		blockedNodeIDs := blockedNodeMap(canvas)
		for _, node := range canvas.Nodes {
			if node.Action != "docker-event-trigger" || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}
			if !matchesDockerEventNode(&node, evt) {
				continue
			}
			go ts.executeWorkflowWithOutput(ctx, aw.ID, node.ID, envID, dockerEventMsg(evt, true))
		}
	}
}

func (ts *TriggerService) handleContainerHealthEvent(ctx context.Context, evt events.Message, envID string) {
	if !ts.leaderFor(ctx) {
		ts.logger.Debug("workflow trigger: not leader for container health event; skipping", "node", ts.nodeID)
		return
	}

	health := containerHealthFromEvent(evt)
	if health == "" {
		return
	}

	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("container health trigger: query error", "error", err)
		return
	}

	containerName := evt.Actor.Attributes["name"]
	containerID := evt.Actor.ID
	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}
		normalizeLegacyNodes(&canvas)

		blockedNodeIDs := blockedNodeMap(canvas)
		for _, node := range canvas.Nodes {
			if !isContainerTrigger(&node, "health") || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}
			cfgContainer, _ := node.Config["container"].(string)
			if cfgContainer != "" && cfgContainer != containerName && cfgContainer != containerID {
				continue
			}
			cfgHealth, _ := node.Config["health"].(string)
			if cfgHealth == "" {
				cfgHealth = "any"
			}
			if cfgHealth != "any" && cfgHealth != health {
				continue
			}

			triggerMsg := NewMsg(map[string]interface{}{
				"trigger":         "container-health",
				"container":       containerName,
				"containerId":     containerID,
				"health":          health,
				"event":           string(evt.Action),
				"eventTime":       evt.Time,
				"eventAttributes": evt.Actor.Attributes,
				"autoTriggered":   true,
			})
			triggerMsg["topic"] = "container-health"
			go ts.executeWorkflowWithOutput(ctx, aw.ID, node.ID, envID, triggerMsg)
		}
	}
}

func blockedNodeMap(canvas CanvasData) map[string]bool {
	blockedNodeIDs := make(map[string]bool)
	for _, g := range canvas.Groups {
		if !g.Blocked {
			continue
		}
		for _, nid := range g.NodeIDs {
			blockedNodeIDs[nid] = true
		}
	}
	return blockedNodeIDs
}

func (ts *TriggerService) handleContainerEvent(ctx context.Context, cli *dockerclient.Client, evt events.Message, envID string) {
	if !ts.leaderFor(ctx) {
		ts.logger.Debug("workflow trigger: not leader for container event; skipping", "node", ts.nodeID)
		return
	}

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
		normalizeLegacyNodes(&canvas)

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
			if !isContainerTrigger(&node, "status") {
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

	wf, err := ts.service.Get(ctx, workflowID)
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

	ts.service.RecordRun(ctx, workflowID, "auto", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
	ts.logger.Info("workflow trigger: execution completed", "workflow", workflowID)
}

// PluginTriggerBinding is one placement of a plugin trigger node on an
// active workflow. Exposed to internal/bootstrap, which bridges it to
// the plugin runtime.
type PluginTriggerBinding struct {
	WorkflowID string
	NodeID     string
	Config     map[string]interface{}
}

// PluginTriggerBindings lists every enabled, unblocked node with the
// given `plugin:<slug>:<key>` action across active workflows.
func (ts *TriggerService) PluginTriggerBindings(ctx context.Context, actionKey string) ([]PluginTriggerBinding, error) {
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PluginTriggerBinding, 0, 8)
	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}
		blockedNodeIDs := blockedNodeMap(canvas)
		for _, node := range canvas.Nodes {
			if ts.service.resolveAction(node.Action) != actionKey || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}
			cfg := node.Config
			if cfg == nil {
				cfg = map[string]interface{}{}
			}
			out = append(out, PluginTriggerBinding{WorkflowID: aw.ID, NodeID: node.ID, Config: cfg})
		}
	}
	return out, nil
}

// FirePluginTrigger is the host's own entry to the plugin trigger path, for the
// action keys named in hostFireActions (the ITSM status trigger); any other key is refused.
func (ts *TriggerService) FirePluginTrigger(ctx context.Context, actionKey string, payload map[string]interface{}, match func(PluginTriggerBinding) bool) (int, error) {
	return ts.fireFrom(ctx, FireSourceHost, actionKey, payload, match)
}

// FirePluginTriggerFor fires on behalf of plugin slug, which may only fire its own
// `plugin:<slug>:` action keys; the leader re-checks a forwarded fire the same way.
func (ts *TriggerService) FirePluginTriggerFor(ctx context.Context, slug, actionKey string, payload map[string]interface{}, match func(PluginTriggerBinding) bool) (int, error) {
	if slug == FireSourceHost {
		return 0, fmt.Errorf("%w: %q is reserved for the host", nodelink.ErrFireNotAllowed, slug)
	}
	return ts.fireFrom(ctx, slug, actionKey, payload, match)
}

// fireFrom starts every active workflow whose trigger node with the given
// action accepts the event. The runs execute in the background exactly
// like a Docker event trigger; the count of runs started is returned so
// the caller can log it. On a node that does not lead the workflow
// trigger the match runs here and the matched bindings are forwarded to
// the leader, which starts them once.
func (ts *TriggerService) fireFrom(ctx context.Context, source, actionKey string, payload map[string]interface{}, match func(PluginTriggerBinding) bool) (int, error) {
	if err := fireAllowed(source, actionKey); err != nil {
		return 0, err
	}
	bindings, err := ts.PluginTriggerBindings(ctx, actionKey)
	if err != nil {
		return 0, err
	}
	matched := make([]PluginTriggerBinding, 0, len(bindings))
	for _, b := range bindings {
		if match == nil || match(b) {
			matched = append(matched, b)
		}
	}
	if len(matched) == 0 {
		return 0, nil
	}
	if !ts.leaderFor(ctx) {
		return ts.forwardPluginFire(ctx, source, actionKey, payload, matched)
	}
	return ts.startPluginRuns(ctx, actionKey, payload, matched), nil
}

// startPluginRuns starts one background run per binding and returns how many started.
func (ts *TriggerService) startPluginRuns(ctx context.Context, actionKey string, payload map[string]interface{}, bindings []PluginTriggerBinding) int {
	run := ts.startRun
	if run == nil {
		run = ts.executeWorkflowWithOutput
	}
	key := actionKey[strings.LastIndex(actionKey, ":")+1:]
	for _, b := range bindings {
		envID, _ := b.Config["environment"].(string)
		triggerMsg := NewMsg(payload)
		triggerMsg["topic"] = key
		// The background context: the plugin's ctx is typically its
		// watcher's, and a run must not be cancelled because the event
		// stream that produced it reconnected.
		// goroutine lifecycle: the run ends when its flow completes, bounded by each node's own timeout.
		go run(context.WithoutCancel(ctx), b.WorkflowID, b.NodeID, envID, triggerMsg)
	}
	return len(bindings)
}

// ActiveDockerEnvIDs exposes the Docker environments the trigger
// service itself watches, for plugin watchers that subscribe per
// environment.
func (ts *TriggerService) ActiveDockerEnvIDs(ctx context.Context) ([]string, error) {
	return ts.service.ListActiveDockerEnvIDs(ctx)
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
		normalizeLegacyNodes(&canvas)

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

	wf, err := ts.service.Get(ctx, workflowID)
	if err != nil || wf == nil {
		return
	}

	result := ts.service.ExecuteWorkflow(ctx, wf, workflowRunOptions{
		WorkflowID:    workflowID,
		Trigger:       "link",
		StartNodeID:   linkInNodeID,
		StartInputMsg: storedMsg,
	}, emit)

	ts.service.RecordRun(ctx, workflowID, "link", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
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
			if !ts.leaderFor(ctx) {
				ts.logger.Debug("workflow metric watcher: not leader this tick; skipping", "node", ts.nodeID)
				continue
			}
			quiesce.Run(func() { ts.checkMetricTriggers(ctx, lastChecked) })
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
		normalizeLegacyNodes(&canvas)

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

			cli, err := ts.app.DockerPool.GetContext(ctx, sk.envID)
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
			if !ts.leaderFor(ctx) {
				ts.logger.Debug("workflow time watcher: not leader this tick; skipping", "node", ts.nodeID)
				continue
			}
			quiesce.Run(func() { ts.checkTimeTriggers(ctx) })
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
		normalizeLegacyNodes(&canvas)

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
			if node.Action != "schedule-trigger" {
				continue
			}

			spec, _ := node.Config["cron"].(string)
			if strings.TrimSpace(spec) == "" {
				continue
			}

			runAt := nowUTC
			if tz, _ := node.Config["timezone"].(string); strings.TrimSpace(tz) != "" {
				if loc, err := time.LoadLocation(tz); err == nil {
					runAt = nowUTC.In(loc)
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
			telemetry.ScheduleFired("workflow", nowUTC.Truncate(time.Minute))

			triggerMsg := NewMsg(map[string]interface{}{
				"trigger":       strings.TrimSuffix(node.Action, "-trigger"),
				"cron":          spec,
				"timezone":      runAt.Location().String(),
				"timestamp":     nowUTC.Format(time.RFC3339),
				"datetime":      runAt.Format("2006-01-02 15:04:05 MST"),
				"unix":          nowUTC.Unix(),
				"autoTriggered": true,
			})
			triggerMsg["topic"] = "schedule"

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
			if !ts.leaderFor(ctx) {
				ts.logger.Debug("workflow file watcher: not leader this tick; skipping", "node", ts.nodeID)
				continue
			}
			quiesce.Run(func() { ts.checkFileWatchTriggers(ctx) })
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
		normalizeLegacyNodes(&canvas)

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

			storageLocationID, _ := node.Config["storage_location_id"].(string)
			storageLocation := ts.fileWatchStorageLocation(ctx, storageLocationID)
			fullPath := resolveFileWatchPath(rawPath, storageLocation)
			current := readFileWatchState(fullPath)
			stateKey := aw.ID + ":" + node.ID + ":" + strings.TrimSpace(storageLocationID) + ":" + fullPath

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
				"trigger":         "file-watch",
				"path":            rawPath,
				"resolvedPath":    fullPath,
				"storageLocation": fileWatchStorageLocationPayload(storageLocation),
				"event":           eventType,
				"timestamp":       time.Now().UTC().Format(time.RFC3339),
				"autoTriggered":   true,
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

func (ts *TriggerService) fileWatchStorageLocation(ctx context.Context, id string) *fileWatchStorageLocation {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}

	var location fileWatchStorageLocation
	err := ts.app.DB.QueryRowContext(ctx, `
		SELECT id, name, location_type, COALESCE(base_path, '')
		FROM storage_locations
		WHERE id = ? AND enabled = 1
		LIMIT 1`, id,
	).Scan(&location.ID, &location.Name, &location.LocationType, &location.BasePath)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			ts.logger.Warn("file watcher: storage location lookup failed", "id", id, "error", err)
		}
		return nil
	}
	return &location
}

func resolveFileWatchPath(path string, location *fileWatchStorageLocation) string {
	path = strings.TrimSpace(path)
	if location == nil || strings.TrimSpace(location.BasePath) == "" {
		return resolveWorkflowPath(path)
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(location.BasePath, filepath.Clean(path))
}

func fileWatchStorageLocationPayload(location *fileWatchStorageLocation) map[string]interface{} {
	if location == nil {
		return nil
	}
	return map[string]interface{}{
		"id":           location.ID,
		"name":         location.Name,
		"locationType": location.LocationType,
		"basePath":     location.BasePath,
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

// matchesFileWatchEvent accepts the multiselect array the editor stores
// as well as the comma-separated string older workflows carry.
func matchesFileWatchEvent(raw interface{}, eventType string) bool {
	wanted := configStringSlice(raw)
	if len(wanted) == 0 {
		return true
	}
	for _, part := range wanted {
		if strings.EqualFold(part, eventType) {
			return true
		}
	}
	return false
}

func (ts *TriggerService) runRegistryTagWatcher(ctx context.Context) {
	defer ts.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !ts.leaderFor(ctx) {
				ts.logger.Debug("workflow registry tag watcher: not leader this tick; skipping", "node", ts.nodeID)
				continue
			}
			ts.checkRegistryTagTriggers(ctx)
		}
	}
}

func (ts *TriggerService) leaderFor(ctx context.Context) bool {
	if !ts.hasCoord {
		return true
	}
	return ts.coordinator.LeaderOf(ctx, "workflow-trigger")
}

func (ts *TriggerService) checkRegistryTagTriggers(ctx context.Context) {
	activeWorkflows, err := ts.service.ListActiveWorkflows(ctx)
	if err != nil {
		ts.logger.Error("registry tag watcher: query error", "error", err)
		return
	}

	now := time.Now()
	for _, aw := range activeWorkflows {
		var canvas CanvasData
		if err := json.Unmarshal([]byte(aw.CanvasData), &canvas); err != nil {
			continue
		}
		normalizeLegacyNodes(&canvas)

		blockedNodeIDs := blockedNodeMap(canvas)
		for _, node := range canvas.Nodes {
			if node.Action != "registry-tag-trigger" || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}

			imageRef, _ := node.Config["image"].(string)
			if strings.TrimSpace(imageRef) == "" {
				continue
			}
			interval := configFloat(node.Config, "interval", 300)
			if interval < 60 {
				interval = 60
			}
			stateKey := aw.ID + ":" + node.ID + ":" + imageRef
			lastCheckKey := stateKey + ":checked"
			if lastRaw, ok := ts.tagStates.Load(lastCheckKey); ok {
				if now.Sub(lastRaw.(time.Time)).Seconds() < interval {
					continue
				}
			}
			ts.tagStates.Store(lastCheckKey, now)

			envID, _ := node.Config["environment"].(string)
			digest, payload, err := ts.service.inspectRegistryTagDigest(ctx, &node, envID)
			if err != nil {
				ts.logger.Warn("registry tag watcher: inspect failed", "error", err, "image", imageRef)
				continue
			}
			previousRaw, loaded := ts.tagStates.Load(stateKey)
			ts.tagStates.Store(stateKey, digest)
			if !loaded || previousRaw.(string) == digest {
				continue
			}

			payload["previousDigest"] = previousRaw.(string)
			payload["digest"] = digest
			payload["changed"] = true
			payload["autoTriggered"] = true
			triggerMsg := NewMsg(payload)
			triggerMsg["topic"] = "registry-tag"
			go ts.executeWorkflowWithOutput(ctx, aw.ID, node.ID, envID, triggerMsg)
		}
	}
}
