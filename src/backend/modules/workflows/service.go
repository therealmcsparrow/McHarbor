// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"gopkg.in/yaml.v3"

	"github.com/rs/xid"
	"github.com/therealmcsparrow/mcharbor/core/db"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

func unusedImagePruneFilters() filters.Args {
	return filters.NewArgs(filters.Arg("dangling", "false"))
}

// LinkOutCallback is called when a link-out node stores a message,
// enabling auto-triggering of downstream workflows with link-in nodes.
type LinkOutCallback func(ctx context.Context, sourceWorkflowID, sourceNodeID string, msg Msg)

// CustomNodeExecutor is the interface for running custom node scripts.
type CustomNodeExecutor interface {
	Execute(ctx context.Context, nodeKey string, config map[string]interface{}, msg map[string]interface{}, timeoutSec float64) (result interface {
		GetPort() string
		GetMsg() map[string]interface{}
	}, err error)
	IsCustomNode(key string) bool
}

// Service handles workflow persistence and business logic.
type Service struct {
	db             *sql.DB
	pool           *docker.ClientPool
	logger         *slog.Logger
	onLinkOut      LinkOutCallback
	customExecutor interface {
		ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error)
		IsCustomNode(key string) bool
	}
	notifier *corenotify.Dispatcher
}

// NewService creates a new workflow service.
func NewService(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger, notifier *corenotify.Dispatcher) *Service {
	return &Service{db: db, pool: pool, logger: logger, notifier: notifier}
}

// SetCustomExecutor registers the custom node executor for handling third-party nodes.
func (s *Service) SetCustomExecutor(executor interface {
	ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error)
	IsCustomNode(key string) bool
}) {
	s.customExecutor = executor
}

// SetLinkOutCallback registers a callback invoked when a link-out node fires.
func (s *Service) SetLinkOutCallback(cb LinkOutCallback) {
	s.onLinkOut = cb
}

// List returns paginated workflows, optionally filtered by status.
func (s *Service) List(status string, page, perPage int) ([]Workflow, int64, error) {
	offset := (page - 1) * perPage

	var total int64
	var rows *sql.Rows
	var err error

	if status != "" {
		err = s.db.QueryRow("SELECT COUNT(*) FROM workflows WHERE status = ?", status).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflows: %w", err)
		}
		rows, err = s.db.Query(
			"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at FROM workflows WHERE status = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?",
			status, perPage, offset,
		)
	} else {
		err = s.db.QueryRow("SELECT COUNT(*) FROM workflows").Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflows: %w", err)
		}
		rows, err = s.db.Query(
			"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at FROM workflows ORDER BY updated_at DESC LIMIT ? OFFSET ?",
			perPage, offset,
		)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("listing workflows: %w", err)
	}
	defer rows.Close()

	var items []Workflow
	for rows.Next() {
		var wf Workflow
		if err := rows.Scan(&wf.ID, &wf.Name, &wf.Description, &wf.Status, &wf.CanvasData, &wf.Variables, &wf.CreatedBy, &wf.UpdatedBy, &wf.LastRunAt, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning workflow: %w", err)
		}
		items = append(items, wf)
	}
	if items == nil {
		items = []Workflow{}
	}

	return items, total, nil
}

// Get returns a single workflow by ID.
func (s *Service) Get(id string) (*Workflow, error) {
	var wf Workflow
	err := s.db.QueryRow(
		"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at FROM workflows WHERE id = ?", id,
	).Scan(&wf.ID, &wf.Name, &wf.Description, &wf.Status, &wf.CanvasData, &wf.Variables, &wf.CreatedBy, &wf.UpdatedBy, &wf.LastRunAt, &wf.CreatedAt, &wf.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow: %w", err)
	}

	return &wf, nil
}

// Create inserts a new workflow and returns it.
func (s *Service) Create(input CreateInput) (*Workflow, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	wf := Workflow{
		ID:          xid.New().String(),
		Name:        input.Name,
		Description: input.Description,
		Status:      "draft",
		CanvasData:  `{"nodes":[],"edges":[],"groups":[],"viewport":{"x":0,"y":0,"zoom":1}}`,
		Variables:   "{}",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := s.db.Exec(
		"INSERT INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		wf.ID, wf.Name, wf.Description, wf.Status, wf.CanvasData, wf.Variables, wf.CreatedBy, wf.UpdatedBy, wf.LastRunAt, wf.CreatedAt, wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating workflow: %w", err)
	}

	return &wf, nil
}

// Update applies partial updates to an existing workflow and returns the updated record.
func (s *Service) Update(id string, input UpdateInput) (*Workflow, error) {
	var exists int
	if err := s.db.QueryRow("SELECT 1 FROM workflows WHERE id = ?", id).Scan(&exists); err != nil {
		return nil, nil // not found
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.Exec("UPDATE workflows SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.Exec("UPDATE workflows SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow description: %w", err)
		}
	}
	if input.Status != nil {
		if _, err := s.db.Exec("UPDATE workflows SET status = ?, updated_at = ? WHERE id = ?", *input.Status, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow status: %w", err)
		}
	}
	if input.CanvasData != nil {
		if _, err := s.db.Exec("UPDATE workflows SET canvas_data = ?, updated_at = ? WHERE id = ?", *input.CanvasData, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow canvas data: %w", err)
		}
	}
	if input.Variables != nil {
		if _, err := s.db.Exec("UPDATE workflows SET variables = ?, updated_at = ? WHERE id = ?", *input.Variables, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow variables: %w", err)
		}
	}

	return s.Get(id)
}

// Delete removes a workflow by ID. Returns true if a row was deleted.
func (s *Service) Delete(id string) (bool, error) {
	result, err := s.db.Exec("DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting workflow: %w", err)
	}

	return db.RowsAffected(result) > 0, nil
}

// ListRuns returns paginated workflow runs, optionally filtered by workflow ID.
func (s *Service) ListRuns(workflowID string, page, perPage int) ([]WorkflowRun, int64, error) {
	offset := (page - 1) * perPage

	var total int64
	var rows *sql.Rows
	var err error

	if workflowID != "" {
		err = s.db.QueryRow("SELECT COUNT(*) FROM workflow_runs WHERE workflow_id = ?", workflowID).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflow runs: %w", err)
		}
		rows, err = s.db.Query(
			"SELECT id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at FROM workflow_runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?",
			workflowID, perPage, offset,
		)
	} else {
		err = s.db.QueryRow("SELECT COUNT(*) FROM workflow_runs").Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflow runs: %w", err)
		}
		rows, err = s.db.Query(
			"SELECT id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at FROM workflow_runs ORDER BY started_at DESC LIMIT ? OFFSET ?",
			perPage, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing workflow runs: %w", err)
	}
	defer rows.Close()

	var runs []WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		if err := rows.Scan(&run.ID, &run.WorkflowID, &run.Status, &run.Trigger, &run.DurationMs, &run.NodeCount, &run.Error, &run.StartedAt, &run.FinishedAt); err != nil {
			s.logger.Error("workflows: scan run error", "error", err)
			continue
		}
		runs = append(runs, run)
	}
	if runs == nil {
		runs = []WorkflowRun{}
	}

	return runs, total, nil
}

// RecordRun inserts a workflow run record into the database.
func (s *Service) RecordRun(workflowID, trigger, status string, durationMs int64, nodeCount int, errMsg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	finishedAt := ""
	if status != "running" {
		finishedAt = now
	}
	_, err := s.db.Exec(
		"INSERT INTO workflow_runs (id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		xid.New().String(), workflowID, status, trigger, durationMs, nodeCount, errMsg, now, finishedAt,
	)
	if err != nil {
		s.logger.Error("workflows: record run error", "error", err)
	}
}

// UpdateLastRunAt sets the last_run_at and updated_at timestamps on a workflow.
func (s *Service) UpdateLastRunAt(id string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec("UPDATE workflows SET last_run_at = ?, updated_at = ? WHERE id = ?", now, now, id); err != nil {
		s.logger.Error("workflows: update last_run_at error", "id", id, "error", err)
	}
}

// ListActiveWorkflows returns the id and canvas_data for all active workflows.
func (s *Service) ListActiveWorkflows(ctx context.Context) ([]struct{ ID, CanvasData string }, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, canvas_data FROM workflows WHERE status = 'active' LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("querying active workflows: %w", err)
	}
	defer rows.Close()

	var result []struct{ ID, CanvasData string }
	for rows.Next() {
		var item struct{ ID, CanvasData string }
		if err := rows.Scan(&item.ID, &item.CanvasData); err != nil {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

// ListActiveDockerEnvIDs returns IDs of all active Docker environments.
func (s *Service) ListActiveDockerEnvIDs() ([]string, error) {
	rows, err := s.db.Query(
		"SELECT id FROM environments WHERE is_active = 1 AND orchestrator_type = 'docker' LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("querying docker environments: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// StoreLinkMessage upserts a link message for a link-out node.
func (s *Service) StoreLinkMessage(workflowID, nodeID, name string, msg Msg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling link message: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO workflow_link_messages (id, workflow_id, node_id, name, msg, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(workflow_id, node_id) DO UPDATE SET name = excluded.name, msg = excluded.msg, updated_at = excluded.updated_at`,
		xid.New().String(), workflowID, nodeID, name, string(data), now,
	)
	if err != nil {
		return fmt.Errorf("upserting link message: %w", err)
	}
	return nil
}

// FetchLinkMessage retrieves the stored msg for a link-out node.
func (s *Service) FetchLinkMessage(workflowID, nodeID string) (Msg, error) {
	var raw string
	err := s.db.QueryRow(
		"SELECT msg FROM workflow_link_messages WHERE workflow_id = ? AND node_id = ? LIMIT 1",
		workflowID, nodeID,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching link message: %w", err)
	}
	var msg Msg
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil, fmt.Errorf("unmarshalling link message: %w", err)
	}
	return msg, nil
}

// ListLinkOutputs returns all link-out nodes across all workflows.
func (s *Service) ListLinkOutputs() ([]LinkOutputInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, name, canvas_data FROM workflows LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("querying workflows for link outputs: %w", err)
	}
	defer rows.Close()

	var results []LinkOutputInfo
	for rows.Next() {
		var wfID, wfName, canvasRaw string
		if err := rows.Scan(&wfID, &wfName, &canvasRaw); err != nil {
			continue
		}
		var canvas CanvasData
		if err := json.Unmarshal([]byte(canvasRaw), &canvas); err != nil {
			continue
		}
		for _, node := range canvas.Nodes {
			if node.Action != "link-out" {
				continue
			}
			name, _ := node.Config["name"].(string)
			results = append(results, LinkOutputInfo{
				WorkflowID:   wfID,
				WorkflowName: wfName,
				NodeID:       node.ID,
				Name:         name,
			})
		}
	}
	if results == nil {
		results = []LinkOutputInfo{}
	}
	return results, nil
}

// ExecuteNode executes a single workflow node and returns the output port, output msg, and error.
// Trigger nodes (msg == nil) create the initial msg; all other nodes receive and transform the msg.
func (s *Service) ExecuteNode(ctx context.Context, node *CanvasNode, msg Msg, flowCtx *FlowContext, fallbackEnvID string) (string, Msg, error) {
	nodeEnvID, _ := node.Config["environment"].(string)
	if nodeEnvID == "" {
		nodeEnvID = fallbackEnvID
	}

	switch node.Action {
	case "manual-trigger":
		time.Sleep(200 * time.Millisecond)
		now := time.Now().UTC()
		out := NewMsg(map[string]interface{}{
			"trigger":   "manual",
			"timestamp": now.Format(time.RFC3339),
			"datetime":  now.Format("2006-01-02 15:04:05 UTC"),
			"unix":      now.Unix(),
		})
		out["topic"] = "manual-trigger"
		return "output", out, nil

	case "webhook-trigger":
		time.Sleep(200 * time.Millisecond)
		now := time.Now().UTC()
		method, _ := node.Config["method"].(string)
		if method == "" {
			method = "POST"
		}
		out := NewMsg(map[string]interface{}{
			"body": map[string]interface{}{},
		})
		out["topic"] = "webhook"
		out["req"] = map[string]interface{}{
			"method":  method,
			"headers": map[string]interface{}{"content-type": "application/json"},
		}
		out["headers"] = map[string]interface{}{"content-type": "application/json"}
		out["_triggerTimestamp"] = now.Format(time.RFC3339)
		return "output", out, nil

	case "schedule-trigger":
		time.Sleep(200 * time.Millisecond)
		now := time.Now().UTC()
		cron, _ := node.Config["cron"].(string)
		out := NewMsg(map[string]interface{}{
			"trigger":   "schedule",
			"cron":      cron,
			"timestamp": now.Format(time.RFC3339),
			"datetime":  now.Format("2006-01-02 15:04:05 UTC"),
			"unix":      now.Unix(),
		})
		out["topic"] = "schedule"
		return "output", out, nil

	case "container-status-trigger":
		return s.executeContainerStatusTrigger(ctx, node, nodeEnvID)

	case "metric-trigger":
		return s.executeMetricTrigger(ctx, node, nodeEnvID)

	case "http-request":
		return s.executeHTTPRequest(ctx, node, msg)

	case "send-email":
		return s.executeSendConfiguredEmail(ctx, node, msg)

	case "send-notification":
		return s.executeSendConfiguredNotification(ctx, node, msg)

	case "condition":
		time.Sleep(100 * time.Millisecond)
		field, _ := node.Config["field"].(string)
		operator, _ := node.Config["operator"].(string)
		value, _ := node.Config["value"].(string)
		result := evaluateCondition(field, operator, value, msg)
		port := "true"
		if !result {
			port = "false"
		}
		// Pass msg through unchanged — routing decision only
		return port, msg, nil

	case "delay":
		seconds := configFloat(node.Config, "seconds", 1.0)
		if seconds > 30 {
			seconds = 30
		}
		if seconds < 0 {
			seconds = 0
		}
		select {
		case <-time.After(time.Duration(seconds * float64(time.Second))):
		case <-ctx.Done():
			return "", nil, fmt.Errorf("cancelled")
		}
		// Pass msg through unchanged — delay only
		return "output", msg, nil

	case "set-variable":
		time.Sleep(100 * time.Millisecond)
		name, _ := node.Config["name"].(string)
		value := node.Config["value"]
		if flowCtx != nil && name != "" {
			flowCtx.FlowVars[name] = value
		}
		// Pass msg through unchanged
		return "output", msg, nil

	case "log":
		time.Sleep(100 * time.Millisecond)
		// Pass msg through unchanged — log is a side-effect
		return "output", msg, nil

	case "junction":
		// Pass msg through unchanged — junction is a routing point
		return "output", msg, nil

	case "docker-action":
		return s.executeDockerAction(ctx, node, msg, nodeEnvID)

	case "container-power":
		return s.executeContainerPower(ctx, node, msg, nodeEnvID)

	case "loop":
		return s.executeLoopRuntime(node, msg, flowCtx)

	case "change":
		time.Sleep(100 * time.Millisecond)
		actionType, _ := node.Config["action_type"].(string)
		scope, _ := node.Config["scope"].(string)
		property, _ := node.Config["property"].(string)
		value := node.Config["value"]

		out := CloneMsg(msg)
		out = EnsureMsgID(out)

		// Determine target based on scope
		var target map[string]interface{}
		switch scope {
		case "flow":
			if flowCtx != nil {
				target = flowCtx.FlowVars
			}
		case "global":
			if flowCtx != nil {
				target = flowCtx.GlobalVars
			}
		default: // "msg" or empty
			target = out
		}

		if target != nil && property != "" {
			if actionType == "set" {
				SetPath(target, property, value)
			} else if actionType == "delete" {
				DeletePath(target, property)
			}
		}

		return "output", out, nil

	case "debug":
		time.Sleep(100 * time.Millisecond)
		// Pass msg through unchanged — debug output is a side-effect via SSE
		return "output", msg, nil

	case "docker-info":
		return s.executeDockerInfo(ctx, node, msg, nodeEnvID)

	case "docker-prune":
		return s.executeDockerPrune(ctx, node, msg, nodeEnvID)

	case "docker-image-pull":
		return s.executeDockerImagePull(ctx, node, msg, nodeEnvID)

	case "host-info":
		return s.executeHostInfo(ctx, node, msg, nodeEnvID)

	case "host-exec":
		return s.executeHostExec(ctx, node, msg, nodeEnvID)

	case "join":
		return s.executeJoinRuntime(node, msg, flowCtx)

	case "parse-json":
		return s.executeParseJSON(node, msg)

	case "parse-csv":
		return s.executeParseCSV(node, msg)

	case "parse-xml":
		return s.executeParseXML(node, msg)

	case "parse-yaml":
		return s.executeParseYAML(node, msg)

	case "parse-html":
		return s.executeParseHTML(node, msg)

	case "node-script":
		code, _ := node.Config["code"].(string)
		if code == "" {
			return "error", msg, nil
		}
		timeoutSec := configFloat(node.Config, "timeout", 10)
		if timeoutSec < 1 {
			timeoutSec = 1
		}
		if timeoutSec > 60 {
			timeoutSec = 60
		}
		if s.customExecutor != nil {
			port, outMsg, execErr := s.customExecutor.ExecuteCustom(ctx, "", node.Config, msg, timeoutSec)
			if execErr != nil {
				s.logger.Error("workflows: node-script execution failed", "error", execErr)
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{"error": execErr.Error()}
				return "error", out, nil
			}
			if outMsg != nil {
				outMsg = EnsureMsgID(outMsg)
			}
			return port, outMsg, nil
		}
		return "output", msg, nil

	// Switch node
	case "switch":
		return s.executeSwitch(node, msg)

	// Container nodes
	case "container-list":
		return s.executeContainerList(ctx, node, msg, nodeEnvID)
	case "container-inspect":
		return s.executeContainerInspect(ctx, node, msg, nodeEnvID)
	case "container-logs":
		return s.executeContainerLogs(ctx, node, msg, nodeEnvID)
	case "container-exec":
		return s.executeContainerExec(ctx, node, msg, nodeEnvID)
	case "container-create":
		return s.executeContainerCreate(ctx, node, msg, nodeEnvID)

	// Stack nodes
	case "stack-deploy":
		return s.executeStackDeployRuntime(ctx, node, msg, nodeEnvID)
	case "stack-list":
		return s.executeStackList(ctx, node, msg, nodeEnvID)
	case "stack-status":
		return s.executeStackStatus(ctx, node, msg, nodeEnvID)
	case "stack-remove":
		return s.executeStackRemove(ctx, node, msg, nodeEnvID)

	// Image nodes
	case "image-list":
		return s.executeImageList(ctx, node, msg, nodeEnvID)
	case "image-inspect":
		return s.executeImageInspect(ctx, node, msg, nodeEnvID)
	case "image-remove":
		return s.executeImageRemove(ctx, node, msg, nodeEnvID)
	case "image-tag":
		return s.executeImageTag(ctx, node, msg, nodeEnvID)

	// Volume nodes
	case "volume-list":
		return s.executeVolumeList(ctx, node, msg, nodeEnvID)
	case "volume-create":
		return s.executeVolumeCreate(ctx, node, msg, nodeEnvID)
	case "volume-remove":
		return s.executeVolumeRemove(ctx, node, msg, nodeEnvID)
	case "volume-inspect":
		return s.executeVolumeInspect(ctx, node, msg, nodeEnvID)

	// Network nodes
	case "network-list":
		return s.executeNetworkList(ctx, node, msg, nodeEnvID)
	case "network-create":
		return s.executeNetworkCreate(ctx, node, msg, nodeEnvID)
	case "network-remove":
		return s.executeNetworkRemove(ctx, node, msg, nodeEnvID)
	case "network-inspect":
		return s.executeNetworkInspect(ctx, node, msg, nodeEnvID)
	case "network-connect":
		return s.executeNetworkConnect(ctx, node, msg, nodeEnvID)

	// Advanced Docker nodes
	case "container-stats":
		return s.executeContainerStats(ctx, node, msg, nodeEnvID)
	case "container-rename":
		return s.executeContainerRename(ctx, node, msg, nodeEnvID)
	case "container-wait":
		return s.executeContainerWait(ctx, node, msg, nodeEnvID)
	case "image-build":
		return s.executeImageBuildRuntime(ctx, node, msg, nodeEnvID)
	case "image-push":
		return s.executeImagePush(ctx, node, msg, nodeEnvID)
	case "registry-search":
		return s.executeRegistrySearch(ctx, node, msg, nodeEnvID)
	case "compose-up":
		return s.executeComposeUp(ctx, node, msg, nodeEnvID)
	case "compose-down":
		return s.executeComposeDown(ctx, node, msg, nodeEnvID)

	// Trigger nodes
	case "cron-trigger":
		return s.executeCronTrigger(node)
	case "file-watch-trigger":
		return s.executeFileWatchTrigger(node)

	// Logic / Flow nodes
	case "range":
		return s.executeRangeRuntime(node, msg, flowCtx)
	case "aggregate":
		return s.executeAggregateRuntime(node, msg, flowCtx)
	case "rate-limit":
		return s.executeRateLimitRuntime(node, msg, flowCtx)
	case "filter":
		return s.executeFilter(node, msg)
	case "sort":
		return s.executeSort(node, msg)
	case "deduplicate":
		return s.executeDeduplicateRuntime(node, msg)
	case "try-catch":
		return executeTryCatchNode(node, msg)
	case "comment":
		return "output", msg, nil

	// Data transformation nodes
	case "template":
		return s.executeTemplate(node, msg)
	case "map":
		return s.executeMap(node, msg)
	case "pick":
		return s.executePick(node, msg)
	case "omit":
		return s.executeOmit(node, msg)
	case "merge-objects":
		return s.executeMergeObjects(node, msg)
	case "base64":
		return s.executeBase64(node, msg)
	case "hash":
		return s.executeHash(node, msg)
	case "date-format":
		return s.executeDateFormat(node, msg)
	case "math":
		return s.executeMath(node, msg)
	case "string-ops":
		return s.executeStringOps(node, msg)
	case "json-path":
		return s.executeJSONPath(node, msg)

	// HTTP nodes
	case "webhook-response":
		return s.executeWebhookResponseRuntime(node, msg, flowCtx)
	case "http-response":
		return s.executeHTTPResponseRuntime(node, msg, flowCtx)
	case "graphql-request":
		return s.executeGraphQLRequest(ctx, node, msg)

	// Notification nodes
	case "send-slack":
		return s.executeSendWebhookGeneric(ctx, node, msg, "slack")
	case "send-discord":
		return s.executeSendWebhookGeneric(ctx, node, msg, "discord")
	case "send-teams":
		return s.executeSendWebhookGeneric(ctx, node, msg, "teams")
	case "send-telegram":
		return s.executeSendTelegram(ctx, node, msg)
	case "send-webhook":
		return s.executeSendOutboundWebhook(ctx, node, msg)
	case "send-whatsapp":
		return s.executeSendWhatsApp(ctx, node, msg)
	case "send-signal":
		return s.executeSendSignal(ctx, node, msg)
	case "send-ntfy":
		return s.executeSendNtfy(ctx, node, msg)
	case "send-gotify":
		return s.executeSendGotify(ctx, node, msg)

	// Storage / State nodes
	case "read-file":
		return s.executeReadFile(node, msg)
	case "write-file":
		return s.executeWriteFile(node, msg)
	case "kv-get":
		return s.executeKVGet(node, msg)
	case "kv-set":
		return s.executeKVSet(node, msg)
	case "kv-delete":
		return s.executeKVDelete(node, msg)
	case "sql-query":
		return s.executeSQLQuery(ctx, node, msg)

	// Monitoring nodes
	case "assert":
		return s.executeAssert(node, msg)
	case "metric-record":
		return s.executeMetricRecord(node, msg, flowCtx)
	case "health-check":
		return s.executeHealthCheck(ctx, node, msg)

	// External service nodes
	case "ssh-exec":
		return s.executeSSHExecRuntime(ctx, node, msg)
	case "ftp-upload":
		return s.executeFTPUploadRuntime(ctx, node, msg)
	case "dns-lookup":
		return s.executeDNSLookup(node, msg)
	case "ping":
		return s.executePing(node, msg)

	case "link-out":
		name, _ := node.Config["name"].(string)
		wfID, _ := flowCtx.FlowVars["_workflowId"].(string)
		if wfID != "" {
			if err := s.StoreLinkMessage(wfID, node.ID, name, msg); err != nil {
				s.logger.Error("link-out: store failed", "error", err, "node", node.ID)
			}
			if s.onLinkOut != nil {
				s.onLinkOut(ctx, wfID, node.ID, msg)
			}
		}
		return "", msg, nil

	case "link-in":
		source, _ := node.Config["source"].(string)
		parts := strings.SplitN(source, ":", 2)
		if len(parts) == 2 {
			stored, err := s.FetchLinkMessage(parts[0], parts[1])
			if err != nil {
				s.logger.Error("link-in: fetch failed", "error", err, "source", source)
				return "output", NewMsg(nil), nil
			}
			if stored != nil {
				return "output", stored, nil
			}
		}
		return "output", NewMsg(nil), nil

	default:
		// Check if this is a custom (third-party) node
		if s.customExecutor != nil && s.customExecutor.IsCustomNode(node.Action) {
			timeoutSec := configFloat(node.Config, "timeout", 10)
			port, outMsg, execErr := s.customExecutor.ExecuteCustom(ctx, node.Action, node.Config, msg, timeoutSec)
			if execErr != nil {
				s.logger.Error("workflows: custom node execution failed", "error", execErr, "action", node.Action)
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{"error": "custom node execution failed"}
				return "error", out, nil
			}
			if outMsg != nil {
				outMsg = EnsureMsgID(outMsg)
			}
			return port, outMsg, nil
		}

		time.Sleep(200 * time.Millisecond)
		return "output", msg, nil
	}
}

// executeContainerStatusTrigger handles the container-status-trigger node action.
func (s *Service) executeContainerStatusTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	containerName, _ := node.Config["container"].(string)
	statusEvent, _ := node.Config["status"].(string)
	if statusEvent == "" {
		statusEvent = "any"
	}

	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	payload := map[string]interface{}{
		"container":   containerName,
		"statusEvent": statusEvent,
	}

	if containerName != "" {
		inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		info, inspectErr := cli.ContainerInspect(inspectCtx, containerName)
		if inspectErr == nil {
			payload["currentState"] = info.State.Status
			payload["running"] = info.State.Running
			payload["startedAt"] = info.State.StartedAt
			payload["finishedAt"] = info.State.FinishedAt
			payload["exitCode"] = info.State.ExitCode
			payload["image"] = info.Config.Image
			if info.State.Health != nil {
				payload["health"] = info.State.Health.Status
			}
		} else {
			payload["inspectError"] = inspectErr.Error()
		}

		filterArgs := filters.NewArgs()
		filterArgs.Add("type", "container")
		if containerName != "" {
			filterArgs.Add("container", containerName)
		}
		if statusEvent != "any" {
			filterArgs.Add("event", statusEvent)
		}

		eventCtx, eventCancel := context.WithTimeout(ctx, 15*time.Second)
		defer eventCancel()
		eventsCh, errCh := cli.Events(eventCtx, events.ListOptions{Filters: filterArgs})

		select {
		case evt := <-eventsCh:
			payload["event"] = string(evt.Action)
			payload["eventTime"] = evt.Time
			payload["eventActor"] = evt.Actor.ID
			payload["eventAttributes"] = evt.Actor.Attributes
		case eventErr := <-errCh:
			if eventErr != nil {
				payload["eventTimeout"] = false
				payload["eventError"] = eventErr.Error()
			}
		case <-eventCtx.Done():
			payload["eventTimeout"] = true
		}
	}

	out := NewMsg(payload)
	out["topic"] = "container-status"
	return "output", out, nil
}

// executeMetricTrigger handles the metric-trigger node action.
func (s *Service) executeMetricTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container name or ID is required")
	}

	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	snapshot, fetchErr := FetchContainerMetric(ctx, cli, containerName)
	if fetchErr != nil {
		return "", nil, fmt.Errorf("failed to fetch metrics: %w", fetchErr)
	}

	out := NewMsg(map[string]interface{}{
		"container":   containerName,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"cpu_percent": snapshot.CPUPercent,
		"mem_percent": snapshot.MemPercent,
		"mem_usage":   snapshot.MemUsage,
		"mem_limit":   snapshot.MemLimit,
		"net_rx":      snapshot.NetRx,
		"net_tx":      snapshot.NetTx,
		"block_read":  snapshot.BlockRead,
		"block_write": snapshot.BlockWrite,
		"pids":        snapshot.PIDs,
	})
	out["topic"] = "metric"
	return "output", out, nil
}

// executeDockerAction handles the docker-action node action.
func (s *Service) executeDockerAction(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	action, _ := node.Config["action"].(string)
	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container name or ID is required")
	}
	if action == "" {
		return "", nil, fmt.Errorf("action is required")
	}

	cli, err := s.pool.Get(envID)
	if err != nil {
		return "error", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Check current state before acting
	if action != "restart" && action != "remove" {
		info, inspectErr := cli.ContainerInspect(opCtx, containerName)
		if inspectErr == nil {
			alreadyRunning := info.State.Running
			if action == "start" && alreadyRunning {
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{
					"action":    action,
					"container": containerName,
					"error":     "Container is already running",
					"state":     info.State.Status,
				}
				return "error", out, nil
			}
			if action == "stop" && !alreadyRunning {
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{
					"action":    action,
					"container": containerName,
					"error":     fmt.Sprintf("Container is already in state %s", info.State.Status),
					"state":     info.State.Status,
				}
				return "error", out, nil
			}
		}
	}

	var opErr error
	switch action {
	case "start":
		opErr = cli.ContainerStart(opCtx, containerName, container.StartOptions{})
	case "stop":
		opErr = cli.ContainerStop(opCtx, containerName, container.StopOptions{})
	case "restart":
		opErr = cli.ContainerRestart(opCtx, containerName, container.StopOptions{})
	case "remove":
		opErr = cli.ContainerRemove(opCtx, containerName, container.RemoveOptions{Force: true})
	default:
		return "", nil, fmt.Errorf("unknown docker action: %s", action)
	}

	if opErr != nil {
		s.logger.Error("workflows: docker action failed", "error", opErr, "action", action, "container", containerName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"action":    action,
			"container": containerName,
			"error":     "operation failed",
		}
		return "error", out, nil
	}

	result := map[string]interface{}{
		"action":    action,
		"container": containerName,
		"status":    "success",
	}
	if action != "remove" {
		info, inspectErr := cli.ContainerInspect(opCtx, containerName)
		if inspectErr == nil {
			result["state"] = info.State.Status
			result["running"] = info.State.Running
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = result
	return "output", out, nil
}

func (s *Service) executeContainerPower(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	action, _ := node.Config["action"].(string)
	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container name or ID is required")
	}
	if action == "" {
		return "", nil, fmt.Errorf("action is required")
	}

	cli, err := s.pool.Get(envID)
	if err != nil {
		return "error", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, inspectErr := cli.ContainerInspect(opCtx, containerName)
	if inspectErr == nil {
		if action == "enable" && info.State.Running {
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{
				"action":    action,
				"container": containerName,
				"error":     "container is already running",
				"state":     info.State.Status,
			}
			return "error", out, nil
		}
		if action == "disable" && !info.State.Running {
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{
				"action":    action,
				"container": containerName,
				"error":     fmt.Sprintf("container is already in state %s", info.State.Status),
				"state":     info.State.Status,
			}
			return "error", out, nil
		}
	}

	var opErr error
	switch action {
	case "enable":
		opErr = cli.ContainerStart(opCtx, containerName, container.StartOptions{})
	case "disable":
		opErr = cli.ContainerStop(opCtx, containerName, container.StopOptions{})
	default:
		return "", nil, fmt.Errorf("unknown container-power action: %s", action)
	}

	if opErr != nil {
		s.logger.Error("workflows: container-power action failed", "error", opErr, "action", action, "container", containerName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"action":    action,
			"container": containerName,
			"error":     "operation failed",
		}
		return "error", out, nil
	}

	result := map[string]interface{}{
		"action":    action,
		"container": containerName,
		"status":    "success",
	}
	postInfo, postErr := cli.ContainerInspect(opCtx, containerName)
	if postErr == nil {
		result["state"] = postInfo.State.Status
		result["running"] = postInfo.State.Running
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = result
	return "output", out, nil
}

// FetchContainerMetric fetches a one-shot Docker stats reading and computes metrics.
// Exported so both handler and trigger can use it without duplication.
func FetchContainerMetric(ctx context.Context, cli *dockerclient.Client, containerID string) (*containerMetricSnapshot, error) {
	statsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := cli.ContainerStats(statsCtx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var stats container.StatsResponse
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}

	// Calculate CPU percentage
	var cpuPercent float64
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if numCPUs == 0 {
			numCPUs = 1
		}
		cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	// Memory percentage
	var memPercent float64
	if stats.MemoryStats.Limit > 0 {
		memPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	// Network I/O
	var netRx, netTx uint64
	for _, net := range stats.Networks {
		netRx += net.RxBytes
		netTx += net.TxBytes
	}

	// Block I/O
	var blockRead, blockWrite uint64
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "read", "Read":
			blockRead += entry.Value
		case "write", "Write":
			blockWrite += entry.Value
		}
	}

	return &containerMetricSnapshot{
		CPUPercent: cpuPercent,
		MemUsage:   stats.MemoryStats.Usage,
		MemLimit:   stats.MemoryStats.Limit,
		MemPercent: memPercent,
		NetRx:      netRx,
		NetTx:      netTx,
		BlockRead:  blockRead,
		BlockWrite: blockWrite,
		PIDs:       stats.PidsStats.Current,
	}, nil
}

// configFloat extracts a float64 from a config map, handling both float64 and string values.
func configFloat(config map[string]interface{}, key string, fallback float64) float64 {
	v, ok := config[key]
	if !ok {
		return fallback
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f
		}
	}
	return fallback
}

// executeDockerInfo retrieves Docker system information.
func (s *Service) executeDockerInfo(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	infoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, err := cli.Info(infoCtx)
	if err != nil {
		s.logger.Error("workflows: docker info failed", "error", err, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to get docker info"}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"serverVersion":  info.ServerVersion,
		"os":             info.OperatingSystem,
		"osType":         info.OSType,
		"architecture":   info.Architecture,
		"kernelVersion":  info.KernelVersion,
		"containers":     info.Containers,
		"running":        info.ContainersRunning,
		"paused":         info.ContainersPaused,
		"stopped":        info.ContainersStopped,
		"images":         info.Images,
		"driver":         info.Driver,
		"memoryTotal":    info.MemTotal,
		"cpus":           info.NCPU,
		"hostname":       info.Name,
		"dockerRootDir":  info.DockerRootDir,
		"registryConfig": info.RegistryConfig,
	}
	return "output", out, nil
}

// executeDockerPrune removes unused Docker resources.
func (s *Service) executeDockerPrune(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	resource, _ := node.Config["resource"].(string)
	if resource == "" {
		resource = "all"
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result := map[string]interface{}{"resource": resource}

	switch resource {
	case "containers":
		report, pruneErr := cli.ContainersPrune(opCtx, filters.Args{})
		if pruneErr != nil {
			s.logger.Error("workflows: container prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "container prune failed"}
			return "error", out, nil
		}
		result["containersDeleted"] = report.ContainersDeleted
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "images":
		report, pruneErr := cli.ImagesPrune(opCtx, unusedImagePruneFilters())
		if pruneErr != nil {
			s.logger.Error("workflows: image prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "image prune failed"}
			return "error", out, nil
		}
		result["imagesDeleted"] = len(report.ImagesDeleted)
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "volumes":
		report, pruneErr := cli.VolumesPrune(opCtx, filters.Args{})
		if pruneErr != nil {
			s.logger.Error("workflows: volume prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "volume prune failed"}
			return "error", out, nil
		}
		result["volumesDeleted"] = report.VolumesDeleted
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "networks":
		report, pruneErr := cli.NetworksPrune(opCtx, filters.Args{})
		if pruneErr != nil {
			s.logger.Error("workflows: network prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "network prune failed"}
			return "error", out, nil
		}
		result["networksDeleted"] = report.NetworksDeleted

	case "all":
		var totalReclaimed uint64
		cr, _ := cli.ContainersPrune(opCtx, filters.Args{})
		result["containersDeleted"] = cr.ContainersDeleted
		totalReclaimed += cr.SpaceReclaimed

		ir, _ := cli.ImagesPrune(opCtx, unusedImagePruneFilters())
		result["imagesDeleted"] = len(ir.ImagesDeleted)
		totalReclaimed += ir.SpaceReclaimed

		vr, _ := cli.VolumesPrune(opCtx, filters.Args{})
		result["volumesDeleted"] = vr.VolumesDeleted
		totalReclaimed += vr.SpaceReclaimed

		nr, _ := cli.NetworksPrune(opCtx, filters.Args{})
		result["networksDeleted"] = nr.NetworksDeleted
		result["spaceReclaimed"] = totalReclaimed
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = result
	return "output", out, nil
}

// executeDockerImagePull pulls a Docker image from a registry.
func (s *Service) executeDockerImagePull(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image name is required")
	}

	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	reader, err := cli.ImagePull(pullCtx, imageName, image.PullOptions{})
	if err != nil {
		s.logger.Error("workflows: image pull failed", "error", err, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image pull failed", "image": imageName}
		return "error", out, nil
	}
	defer reader.Close()

	// Drain the pull output to completion
	if _, copyErr := io.Copy(io.Discard, reader); copyErr != nil {
		s.logger.Warn("workflows: image pull stream drain failed", "error", copyErr, "image", imageName, "envID", envID)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"image":  imageName,
		"status": "pulled",
	}
	return "output", out, nil
}

// executeHostInfo returns system information from the Docker host via Docker info.
func (s *Service) executeHostInfo(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	infoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, err := cli.Info(infoCtx)
	if err != nil {
		s.logger.Error("workflows: host info failed", "error", err, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to get host info"}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"hostname":      info.Name,
		"os":            info.OperatingSystem,
		"osType":        info.OSType,
		"architecture":  info.Architecture,
		"kernelVersion": info.KernelVersion,
		"cpus":          info.NCPU,
		"memoryTotal":   info.MemTotal,
		"serverVersion": info.ServerVersion,
	}
	return "output", out, nil
}

// executeHostExec runs a command on the Docker host via a temporary container.
func (s *Service) executeHostExec(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	command, _ := node.Config["command"].(string)
	if command == "" {
		return "", nil, fmt.Errorf("command is required")
	}

	timeoutSec := configFloat(node.Config, "timeout", 30)
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	if timeoutSec > 300 {
		timeoutSec = 300
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second+10*time.Second)
	defer cancel()

	// Run command via a temporary alpine container with host PID namespace
	resp, err := cli.ContainerCreate(execCtx, &container.Config{
		Image: "alpine:3.20",
		Cmd:   []string{"sh", "-c", command},
	}, &container.HostConfig{
		AutoRemove: true,
		PidMode:    "host",
	}, nil, nil, "")
	if err != nil {
		s.logger.Error("workflows: host exec create failed", "error", err, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to create exec container", "command": command}
		return "error", out, nil
	}

	if startErr := cli.ContainerStart(execCtx, resp.ID, container.StartOptions{}); startErr != nil {
		s.logger.Error("workflows: host exec start failed", "error", startErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to start exec container", "command": command}
		return "error", out, nil
	}

	// Wait for completion
	statusCh, errCh := cli.ContainerWait(execCtx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int64
	select {
	case waitResult := <-statusCh:
		exitCode = waitResult.StatusCode
	case waitErr := <-errCh:
		if waitErr != nil {
			s.logger.Error("workflows: host exec wait failed", "error", waitErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "exec container wait failed", "command": command}
			return "error", out, nil
		}
	}

	// Capture logs
	logReader, logErr := cli.ContainerLogs(execCtx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	var output string
	if logErr == nil {
		defer logReader.Close()
		logBytes, _ := io.ReadAll(logReader)
		output = string(logBytes)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"command":  command,
		"exitCode": exitCode,
		"output":   output,
	}

	if exitCode != 0 {
		return "error", out, nil
	}
	return "output", out, nil
}

// executeParseJSON parses a JSON string in msg.payload (or configured property) into an object.
func (s *Service) executeParseJSON(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	str, ok := raw.(string)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property is not a string", "property": property}
		return "error", out, nil
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(str), &parsed); err != nil {
		s.logger.Warn("workflows: parse json failed", "error", err, "property", property)
		return "error", workflowErrorMsg(msg, "invalid JSON", map[string]interface{}{"property": property}), nil
	}

	SetPath(out, property, parsed)
	return "output", out, nil
}

// executeParseCSV parses CSV/TSV text in msg.payload into an array of objects or arrays.
func (s *Service) executeParseCSV(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	delimiter, _ := node.Config["delimiter"].(string)
	if delimiter == "" {
		delimiter = ","
	}
	hasHeaders := true
	if v, ok := node.Config["has_headers"].(bool); ok {
		hasHeaders = v
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	str, ok := raw.(string)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property is not a string", "property": property}
		return "error", out, nil
	}

	reader := csv.NewReader(strings.NewReader(str))
	if delimiter == "\\t" {
		reader.Comma = '\t'
	} else if len(delimiter) > 0 {
		reader.Comma = rune(delimiter[0])
	}
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		s.logger.Warn("workflows: parse csv failed", "error", err, "property", property)
		return "error", workflowErrorMsg(msg, "CSV parse failed", map[string]interface{}{"property": property}), nil
	}

	if len(records) == 0 {
		SetPath(out, property, []interface{}{})
		return "output", out, nil
	}

	if hasHeaders && len(records) > 1 {
		headers := records[0]
		rows := make([]interface{}, 0, len(records)-1)
		for _, record := range records[1:] {
			row := make(map[string]interface{}, len(headers))
			for j, h := range headers {
				if j < len(record) {
					row[h] = record[j]
				}
			}
			rows = append(rows, row)
		}
		SetPath(out, property, rows)
	} else {
		rows := make([]interface{}, 0, len(records))
		for _, record := range records {
			row := make([]interface{}, len(record))
			for j, v := range record {
				row[j] = v
			}
			rows = append(rows, row)
		}
		SetPath(out, property, rows)
	}

	return "output", out, nil
}

// executeParseXML parses an XML string into a nested map structure.
func (s *Service) executeParseXML(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	str, ok := raw.(string)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property is not a string", "property": property}
		return "error", out, nil
	}

	parsed, err := xmlToMap([]byte(str))
	if err != nil {
		s.logger.Warn("workflows: parse xml failed", "error", err, "property", property)
		return "error", workflowErrorMsg(msg, "invalid XML", map[string]interface{}{"property": property}), nil
	}

	SetPath(out, property, parsed)
	return "output", out, nil
}

// xmlToMap converts an XML document into a nested map[string]interface{}.
func xmlToMap(data []byte) (interface{}, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	return xmlDecodeElement(decoder, xml.StartElement{})
}

func xmlDecodeElement(decoder *xml.Decoder, start xml.StartElement) (interface{}, error) {
	result := map[string]interface{}{}

	// Add attributes
	for _, attr := range start.Attr {
		result["@"+attr.Name.Local] = attr.Value
	}

	var text strings.Builder
	children := map[string][]interface{}{}

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, childErr := xmlDecodeElement(decoder, t)
			if childErr != nil {
				return nil, childErr
			}
			children[t.Name.Local] = append(children[t.Name.Local], child)

		case xml.CharData:
			text.Write(t)

		case xml.EndElement:
			// If there are no children and no attributes, return text directly
			trimmed := strings.TrimSpace(text.String())
			if len(children) == 0 && len(start.Attr) == 0 {
				if trimmed != "" {
					return trimmed, nil
				}
				return "", nil
			}

			// Add text content if present
			if trimmed != "" {
				result["#text"] = trimmed
			}

			// Flatten single-element arrays
			for k, v := range children {
				if len(v) == 1 {
					result[k] = v[0]
				} else {
					result[k] = v
				}
			}

			return result, nil
		}
	}

	return result, nil
}

// executeParseYAML parses a YAML string into a map/slice.
func (s *Service) executeParseYAML(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	str, ok := raw.(string)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property is not a string", "property": property}
		return "error", out, nil
	}

	var parsed interface{}
	if err := yaml.Unmarshal([]byte(str), &parsed); err != nil {
		s.logger.Warn("workflows: parse yaml failed", "error", err, "property", property)
		return "error", workflowErrorMsg(msg, "invalid YAML", map[string]interface{}{"property": property}), nil
	}

	// yaml.v3 uses map[string]interface{} for maps by default, which is what we want
	SetPath(out, property, normalizeYAML(parsed))
	return "output", out, nil
}

// normalizeYAML converts map[interface{}]interface{} (from yaml.v3) to map[string]interface{}.
func normalizeYAML(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			result[k] = normalizeYAML(v2)
		}
		return result
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			result[fmt.Sprintf("%v", k)] = normalizeYAML(v2)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v2 := range val {
			result[i] = normalizeYAML(v2)
		}
		return result
	default:
		return v
	}
}

// executeParseHTML extracts data from HTML using a CSS selector.
func (s *Service) executeParseHTML(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	selector, _ := node.Config["selector"].(string)
	if selector == "" {
		selector = "body"
	}
	outputType, _ := node.Config["output_type"].(string)
	if outputType == "" {
		outputType = "text"
	}
	attrName, _ := node.Config["attribute"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	str, ok := raw.(string)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property is not a string", "property": property}
		return "error", out, nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(str))
	if err != nil {
		s.logger.Warn("workflows: parse html failed", "error", err, "property", property)
		return "error", workflowErrorMsg(msg, "invalid HTML", map[string]interface{}{"property": property}), nil
	}

	selection := doc.Find(selector)
	results := make([]interface{}, 0, selection.Length())

	selection.Each(func(_ int, sel *goquery.Selection) {
		switch outputType {
		case "html":
			h, htmlErr := sel.Html()
			if htmlErr == nil {
				results = append(results, h)
			}
		case "attribute":
			if attrName != "" {
				if val, exists := sel.Attr(attrName); exists {
					results = append(results, val)
				}
			}
		default: // "text"
			results = append(results, strings.TrimSpace(sel.Text()))
		}
	})

	// If single result, unwrap from array
	if len(results) == 1 {
		SetPath(out, property, results[0])
	} else {
		SetPath(out, property, results)
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Container node implementations
// ---------------------------------------------------------------------------

// executeContainerList lists containers, optionally filtered by name.
func (s *Service) executeContainerList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	showAll := true
	if v, ok := node.Config["show_all"].(bool); ok {
		showAll = v
	}
	nameFilter, _ := node.Config["name_filter"].(string)

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := container.ListOptions{All: showAll}
	if nameFilter != "" {
		f := filters.NewArgs()
		f.Add("name", nameFilter)
		opts.Filters = f
	}

	containers, listErr := cli.ContainerList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: container list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container list failed"}
		return "error", out, nil
	}

	items := make([]interface{}, 0, len(containers))
	for _, c := range containers {
		items = append(items, map[string]interface{}{
			"id":      c.ID[:12],
			"names":   c.Names,
			"image":   c.Image,
			"state":   c.State,
			"status":  c.Status,
			"created": c.Created,
			"ports":   c.Ports,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeContainerInspect returns detailed information about a container.
func (s *Service) executeContainerInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, inspectErr := cli.ContainerInspect(opCtx, containerName)
	if inspectErr != nil {
		s.logger.Error("workflows: container inspect failed", "error", inspectErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container inspect failed", "container": containerName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":         info.ID[:12],
		"name":       strings.TrimPrefix(info.Name, "/"),
		"image":      info.Config.Image,
		"state":      info.State.Status,
		"running":    info.State.Running,
		"startedAt":  info.State.StartedAt,
		"finishedAt": info.State.FinishedAt,
		"exitCode":   info.State.ExitCode,
		"platform":   info.Platform,
		"mounts":     info.Mounts,
		"ports":      info.NetworkSettings.Ports,
		"env":        info.Config.Env,
		"labels":     info.Config.Labels,
		"restartPolicy": map[string]interface{}{
			"name":          info.HostConfig.RestartPolicy.Name,
			"maxRetryCount": info.HostConfig.RestartPolicy.MaximumRetryCount,
		},
	}
	return "output", out, nil
}

// executeContainerLogs retrieves recent logs from a container.
func (s *Service) executeContainerLogs(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	tail := int(configFloat(node.Config, "tail", 100))
	if tail < 1 {
		tail = 100
	}
	if tail > 10000 {
		tail = 10000
	}
	since, _ := node.Config["since"].(string)

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tail),
	}
	if since != "" {
		opts.Since = since
	}

	reader, logErr := cli.ContainerLogs(opCtx, containerName, opts)
	if logErr != nil {
		s.logger.Error("workflows: container logs failed", "error", logErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container logs failed", "container": containerName}
		return "error", out, nil
	}
	defer reader.Close()

	logBytes, _ := io.ReadAll(reader)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"container": containerName,
		"logs":      string(logBytes),
		"lines":     tail,
	}
	return "output", out, nil
}

// executeContainerExec runs a command inside a running container using detached exec + inspect.
func (s *Service) executeContainerExec(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}
	command, _ := node.Config["command"].(string)
	if command == "" {
		return "", nil, fmt.Errorf("command is required")
	}
	workdir, _ := node.Config["workdir"].(string)

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	execConfig := container.ExecOptions{
		Cmd:          []string{"sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
	}
	if workdir != "" {
		execConfig.WorkingDir = workdir
	}

	execResp, execErr := cli.ContainerExecCreate(opCtx, containerName, execConfig)
	if execErr != nil {
		s.logger.Error("workflows: container exec create failed", "error", execErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "exec create failed", "container": containerName}
		return "error", out, nil
	}

	attachResp, attachErr := cli.ContainerExecAttach(opCtx, execResp.ID, container.ExecAttachOptions{})
	if attachErr != nil {
		s.logger.Error("workflows: container exec attach failed", "error", attachErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "exec attach failed", "container": containerName}
		return "error", out, nil
	}
	defer attachResp.Close()

	outputBytes, _ := io.ReadAll(attachResp.Reader)

	// Get exit code
	inspectResp, inspectErr := cli.ContainerExecInspect(opCtx, execResp.ID)
	var exitCode int
	if inspectErr == nil {
		exitCode = inspectResp.ExitCode
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"container": containerName,
		"command":   command,
		"output":    string(outputBytes),
		"exitCode":  exitCode,
	}

	if exitCode != 0 {
		return "error", out, nil
	}
	return "output", out, nil
}

// executeContainerCreate creates (and optionally starts) a new container.
func (s *Service) executeContainerCreate(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}
	containerName, _ := node.Config["name"].(string)
	portsStr, _ := node.Config["ports"].(string)
	restartPolicy, _ := node.Config["restart_policy"].(string)
	if restartPolicy == "" {
		restartPolicy = "no"
	}
	autoStart := true
	if v, ok := node.Config["auto_start"].(bool); ok {
		autoStart = v
	}

	// Parse environment variables
	var envVars []string
	if kvRaw, ok := node.Config["env_vars"].(map[string]interface{}); ok {
		for k, v := range kvRaw {
			envVars = append(envVars, fmt.Sprintf("%s=%v", k, v))
		}
	}

	// Parse port bindings
	exposedPorts := make(map[string]struct{})
	portBindings := make(map[string][]map[string]string)
	if portsStr != "" {
		for _, mapping := range strings.Split(portsStr, ",") {
			parts := strings.SplitN(strings.TrimSpace(mapping), ":", 2)
			if len(parts) == 2 {
				containerPort := parts[1] + "/tcp"
				exposedPorts[containerPort] = struct{}{}
				portBindings[containerPort] = []map[string]string{{"HostPort": parts[0]}}
			}
		}
	}

	opCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	config := &container.Config{
		Image: imageName,
		Env:   envVars,
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(restartPolicy)},
	}

	resp, createErr := cli.ContainerCreate(opCtx, config, hostConfig, nil, nil, containerName)
	if createErr != nil {
		s.logger.Error("workflows: container create failed", "error", createErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container create failed", "image": imageName}
		return "error", out, nil
	}

	result := map[string]interface{}{
		"id":    resp.ID[:12],
		"image": imageName,
	}
	if containerName != "" {
		result["name"] = containerName
	}

	if autoStart {
		if startErr := cli.ContainerStart(opCtx, resp.ID, container.StartOptions{}); startErr != nil {
			s.logger.Error("workflows: container start after create failed", "error", startErr, "envID", envID)
			result["started"] = false
			result["startError"] = "failed to start container"
		} else {
			result["started"] = true
		}
	} else {
		result["started"] = false
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = result
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Stack node implementations
// ---------------------------------------------------------------------------

// executeStackDeploy deploys a compose stack by writing a temp compose file and running docker compose up.
func (s *Service) executeStackDeploy(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}

	// Stack operations use labels on containers to identify stack membership
	// We list containers with the stack label to check if the stack exists
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+stackName)
	existing, _ := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	// Legacy fallback retained for compatibility. Runtime execution uses executeStackDeployRuntime.
	out["payload"] = map[string]interface{}{
		"stack":         stackName,
		"action":        "deploy",
		"existingCount": len(existing),
		"status":        "acknowledged",
	}
	return "output", out, nil
}

// executeStackList lists compose stacks by grouping containers by their compose project label.
func (s *Service) executeStackList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project")
	containers, listErr := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack list failed"}
		return "error", out, nil
	}

	stacks := map[string]map[string]interface{}{}
	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		if project == "" {
			continue
		}
		if _, ok := stacks[project]; !ok {
			stacks[project] = map[string]interface{}{
				"name":     project,
				"services": 0,
				"running":  0,
				"stopped":  0,
			}
		}
		stacks[project]["services"] = stacks[project]["services"].(int) + 1
		if c.State == "running" {
			stacks[project]["running"] = stacks[project]["running"].(int) + 1
		} else {
			stacks[project]["stopped"] = stacks[project]["stopped"].(int) + 1
		}
	}

	items := make([]interface{}, 0, len(stacks))
	for _, stack := range stacks {
		items = append(items, stack)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeStackStatus returns the status of a specific compose stack.
func (s *Service) executeStackStatus(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+stackName)
	containers, listErr := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack status failed", "error", listErr, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack status failed", "stack": stackName}
		return "error", out, nil
	}

	services := make([]interface{}, 0, len(containers))
	running := 0
	for _, c := range containers {
		svcName := c.Labels["com.docker.compose.service"]
		services = append(services, map[string]interface{}{
			"id":      c.ID[:12],
			"service": svcName,
			"image":   c.Image,
			"state":   c.State,
			"status":  c.Status,
		})
		if c.State == "running" {
			running++
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"stack":    stackName,
		"services": services,
		"total":    len(containers),
		"running":  running,
		"stopped":  len(containers) - running,
		"healthy":  running == len(containers) && len(containers) > 0,
	}
	return "output", out, nil
}

// executeStackRemove removes all containers belonging to a compose stack.
func (s *Service) executeStackRemove(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	removeVolumes := false
	if v, ok := node.Config["remove_volumes"].(bool); ok {
		removeVolumes = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+stackName)
	containers, listErr := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack remove list failed", "error", listErr, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack remove failed", "stack": stackName}
		return "error", out, nil
	}

	removed := 0
	for _, c := range containers {
		rmErr := cli.ContainerRemove(opCtx, c.ID, container.RemoveOptions{Force: true, RemoveVolumes: removeVolumes})
		if rmErr != nil {
			s.logger.Error("workflows: stack remove container failed", "error", rmErr, "container", c.ID[:12], "stack", stackName)
			continue
		}
		removed++
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"stack":   stackName,
		"removed": removed,
		"total":   len(containers),
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Image node implementations
// ---------------------------------------------------------------------------

// executeImageList lists Docker images.
func (s *Service) executeImageList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	showAll := false
	if v, ok := node.Config["show_all"].(bool); ok {
		showAll = v
	}
	filterDangling := false
	if v, ok := node.Config["filter_dangling"].(bool); ok {
		filterDangling = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := image.ListOptions{All: showAll}
	if filterDangling {
		f := filters.NewArgs()
		f.Add("dangling", "true")
		opts.Filters = f
	}

	images, listErr := cli.ImageList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: image list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image list failed"}
		return "error", out, nil
	}

	items := make([]interface{}, 0, len(images))
	for _, img := range images {
		items = append(items, map[string]interface{}{
			"id":      img.ID[7:19],
			"tags":    img.RepoTags,
			"size":    img.Size,
			"created": img.Created,
			"digests": img.RepoDigests,
			"labels":  img.Labels,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeImageInspect returns detailed info about a Docker image.
func (s *Service) executeImageInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, _, inspectErr := cli.ImageInspectWithRaw(opCtx, imageName)
	if inspectErr != nil {
		s.logger.Error("workflows: image inspect failed", "error", inspectErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image inspect failed", "image": imageName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":           info.ID[7:19],
		"tags":         info.RepoTags,
		"size":         info.Size,
		"architecture": info.Architecture,
		"os":           info.Os,
		"created":      info.Created,
		"author":       info.Author,
		"layers":       len(info.RootFS.Layers),
		"env":          info.Config.Env,
		"cmd":          info.Config.Cmd,
		"entrypoint":   info.Config.Entrypoint,
		"labels":       info.Config.Labels,
		"exposedPorts": info.Config.ExposedPorts,
	}
	return "output", out, nil
}

// executeImageRemove removes a Docker image.
func (s *Service) executeImageRemove(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}
	force := false
	if v, ok := node.Config["force"].(bool); ok {
		force = v
	}
	pruneChildren := true
	if v, ok := node.Config["prune_children"].(bool); ok {
		pruneChildren = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deleted, rmErr := cli.ImageRemove(opCtx, imageName, image.RemoveOptions{Force: force, PruneChildren: pruneChildren})
	if rmErr != nil {
		s.logger.Error("workflows: image remove failed", "error", rmErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image remove failed", "image": imageName}
		return "error", out, nil
	}

	untagged := 0
	deletedCount := 0
	for _, d := range deleted {
		if d.Untagged != "" {
			untagged++
		}
		if d.Deleted != "" {
			deletedCount++
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"image":    imageName,
		"untagged": untagged,
		"deleted":  deletedCount,
	}
	return "output", out, nil
}

// executeImageTag tags a Docker image with a new name.
func (s *Service) executeImageTag(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	source, _ := node.Config["source"].(string)
	target, _ := node.Config["target"].(string)
	if source == "" || target == "" {
		return "", nil, fmt.Errorf("source and target are required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if tagErr := cli.ImageTag(opCtx, source, target); tagErr != nil {
		s.logger.Error("workflows: image tag failed", "error", tagErr, "source", source, "target", target, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image tag failed", "source": source, "target": target}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"source": source,
		"target": target,
		"status": "tagged",
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Volume node implementations
// ---------------------------------------------------------------------------

// executeVolumeList lists Docker volumes.
func (s *Service) executeVolumeList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	filterDangling := false
	if v, ok := node.Config["filter_dangling"].(bool); ok {
		filterDangling = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := volume.ListOptions{}
	if filterDangling {
		f := filters.NewArgs()
		f.Add("dangling", "true")
		opts.Filters = f
	}

	resp, listErr := cli.VolumeList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: volume list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume list failed"}
		return "error", out, nil
	}

	items := make([]interface{}, 0, len(resp.Volumes))
	for _, vol := range resp.Volumes {
		items = append(items, map[string]interface{}{
			"name":       vol.Name,
			"driver":     vol.Driver,
			"mountpoint": vol.Mountpoint,
			"scope":      vol.Scope,
			"labels":     vol.Labels,
			"createdAt":  vol.CreatedAt,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeVolumeCreate creates a Docker volume.
func (s *Service) executeVolumeCreate(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("volume name is required")
	}
	driver, _ := node.Config["driver"].(string)
	if driver == "" {
		driver = "local"
	}

	var labels map[string]string
	if kvRaw, ok := node.Config["labels"].(map[string]interface{}); ok {
		labels = make(map[string]string, len(kvRaw))
		for k, v := range kvRaw {
			labels[k] = fmt.Sprintf("%v", v)
		}
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vol, createErr := cli.VolumeCreate(opCtx, volume.CreateOptions{
		Name:   name,
		Driver: driver,
		Labels: labels,
	})
	if createErr != nil {
		s.logger.Error("workflows: volume create failed", "error", createErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume create failed", "name": name}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"name":       vol.Name,
		"driver":     vol.Driver,
		"mountpoint": vol.Mountpoint,
		"scope":      vol.Scope,
	}
	return "output", out, nil
}

// executeVolumeRemove removes a Docker volume.
func (s *Service) executeVolumeRemove(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("volume name is required")
	}
	force := false
	if v, ok := node.Config["force"].(bool); ok {
		force = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if rmErr := cli.VolumeRemove(opCtx, name, force); rmErr != nil {
		s.logger.Error("workflows: volume remove failed", "error", rmErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume remove failed", "name": name}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"name":   name,
		"status": "removed",
	}
	return "output", out, nil
}

// executeVolumeInspect returns detailed info about a Docker volume.
func (s *Service) executeVolumeInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("volume name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vol, inspectErr := cli.VolumeInspect(opCtx, name)
	if inspectErr != nil {
		s.logger.Error("workflows: volume inspect failed", "error", inspectErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume inspect failed", "name": name}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"name":       vol.Name,
		"driver":     vol.Driver,
		"mountpoint": vol.Mountpoint,
		"scope":      vol.Scope,
		"labels":     vol.Labels,
		"options":    vol.Options,
		"createdAt":  vol.CreatedAt,
		"status":     vol.Status,
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Network node implementations
// ---------------------------------------------------------------------------

// executeNetworkList lists Docker networks.
func (s *Service) executeNetworkList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	networks, listErr := cli.NetworkList(opCtx, network.ListOptions{})
	if listErr != nil {
		s.logger.Error("workflows: network list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network list failed"}
		return "error", out, nil
	}

	items := make([]interface{}, 0, len(networks))
	for _, n := range networks {
		items = append(items, map[string]interface{}{
			"id":       n.ID[:12],
			"name":     n.Name,
			"driver":   n.Driver,
			"scope":    n.Scope,
			"internal": n.Internal,
			"labels":   n.Labels,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeNetworkCreate creates a Docker network.
func (s *Service) executeNetworkCreate(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("network name is required")
	}
	driver, _ := node.Config["driver"].(string)
	if driver == "" {
		driver = "bridge"
	}
	internal := false
	if v, ok := node.Config["internal"].(bool); ok {
		internal = v
	}
	subnet, _ := node.Config["subnet"].(string)

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := network.CreateOptions{
		Driver:   driver,
		Internal: internal,
	}
	if subnet != "" {
		opts.IPAM = &network.IPAM{
			Config: []network.IPAMConfig{{Subnet: subnet}},
		}
	}

	resp, createErr := cli.NetworkCreate(opCtx, name, opts)
	if createErr != nil {
		s.logger.Error("workflows: network create failed", "error", createErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network create failed", "name": name}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":     resp.ID[:12],
		"name":   name,
		"driver": driver,
	}
	return "output", out, nil
}

// executeNetworkRemove removes a Docker network.
func (s *Service) executeNetworkRemove(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("network name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if rmErr := cli.NetworkRemove(opCtx, name); rmErr != nil {
		s.logger.Error("workflows: network remove failed", "error", rmErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network remove failed", "name": name}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"name":   name,
		"status": "removed",
	}
	return "output", out, nil
}

// executeNetworkInspect returns detailed info about a Docker network.
func (s *Service) executeNetworkInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name, _ := node.Config["name"].(string)
	if name == "" {
		return "", nil, fmt.Errorf("network name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, inspectErr := cli.NetworkInspect(opCtx, name, network.InspectOptions{})
	if inspectErr != nil {
		s.logger.Error("workflows: network inspect failed", "error", inspectErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network inspect failed", "name": name}
		return "error", out, nil
	}

	containers := make(map[string]interface{}, len(info.Containers))
	for id, ep := range info.Containers {
		containers[id[:12]] = map[string]interface{}{
			"name":        ep.Name,
			"ipv4Address": ep.IPv4Address,
			"ipv6Address": ep.IPv6Address,
			"macAddress":  ep.MacAddress,
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":         info.ID[:12],
		"name":       info.Name,
		"driver":     info.Driver,
		"scope":      info.Scope,
		"internal":   info.Internal,
		"labels":     info.Labels,
		"containers": containers,
		"ipam":       info.IPAM,
	}
	return "output", out, nil
}

// executeNetworkConnect connects or disconnects a container from a network.
func (s *Service) executeNetworkConnect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	networkName, _ := node.Config["network"].(string)
	containerName, _ := node.Config["container"].(string)
	action, _ := node.Config["action"].(string)
	if networkName == "" || containerName == "" {
		return "", nil, fmt.Errorf("network and container are required")
	}
	if action == "" {
		action = "connect"
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var opErr error
	if action == "disconnect" {
		opErr = cli.NetworkDisconnect(opCtx, networkName, containerName, false)
	} else {
		opErr = cli.NetworkConnect(opCtx, networkName, containerName, nil)
	}

	if opErr != nil {
		s.logger.Error("workflows: network connect/disconnect failed", "error", opErr, "network", networkName, "container", containerName, "action", action, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network " + action + " failed", "network": networkName, "container": containerName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"network":   networkName,
		"container": containerName,
		"action":    action,
		"status":    "success",
	}
	return "output", out, nil
}

// executeSwitch routes a message to a matching case output port based on property value.
func (s *Service) executeSwitch(node *CanvasNode, msg Msg) (string, Msg, error) {
	time.Sleep(100 * time.Millisecond)

	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	checkType, _ := node.Config["check_type"].(string)
	if checkType == "" {
		checkType = "value"
	}

	// Resolve the property value from the message
	var propValue string
	var propType string
	if msg != nil {
		if v, ok := GetPath(msg, property); ok {
			propValue = fmt.Sprintf("%v", v)
			propType = fmt.Sprintf("%T", v)
		}
	}

	// Get the switch cases
	casesRaw, _ := node.Config["switch_cases"].([]interface{})

	for i, caseRaw := range casesRaw {
		caseMap, ok := caseRaw.(map[string]interface{})
		if !ok {
			continue
		}
		caseValue, _ := caseMap["value"].(string)

		matched := false
		switch checkType {
		case "type":
			matched = strings.EqualFold(propType, caseValue) || strings.Contains(propType, caseValue)
		case "regex":
			if re, err := regexp.Compile(caseValue); err == nil {
				matched = re.MatchString(propValue)
			}
		default: // "value"
			matched = propValue == caseValue
		}

		if matched {
			port := fmt.Sprintf("case_%d", i)
			return port, msg, nil
		}
	}

	// No match — route to default
	return "default", msg, nil
}

// evaluateCondition checks a simple condition against the msg using dot-notation paths.
// Fields like "payload.status" resolve to msg["payload"]["status"].
func evaluateCondition(field, operator, value string, msg Msg) bool {
	if field == "" {
		return true
	}

	var fieldValue string
	if msg != nil {
		if v, ok := GetPath(msg, field); ok {
			fieldValue = fmt.Sprintf("%v", v)
		}
	}

	switch operator {
	case "==":
		return fieldValue == value
	case "!=":
		return fieldValue != value
	case "contains":
		return strings.Contains(fieldValue, value)
	case "is_empty":
		return fieldValue == ""
	case "is_not_empty":
		return fieldValue != ""
	case "regex":
		if re, err := regexp.Compile(value); err == nil {
			return re.MatchString(fieldValue)
		}
		return false
	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// Trigger node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeCronTrigger(node *CanvasNode) (string, Msg, error) {
	time.Sleep(200 * time.Millisecond)
	cron, _ := node.Config["cron"].(string)
	tz, _ := node.Config["timezone"].(string)
	if tz == "" {
		tz = "UTC"
	}
	now := time.Now().UTC()
	out := NewMsg(map[string]interface{}{
		"trigger":   "cron",
		"cron":      cron,
		"timezone":  tz,
		"timestamp": now.Format(time.RFC3339),
		"unix":      now.Unix(),
	})
	out["topic"] = "cron"
	return "output", out, nil
}

func (s *Service) executeFileWatchTrigger(node *CanvasNode) (string, Msg, error) {
	time.Sleep(200 * time.Millisecond)
	watchPath, _ := node.Config["path"].(string)
	eventTypes, _ := node.Config["event_types"].(string)
	now := time.Now().UTC()
	out := NewMsg(map[string]interface{}{
		"trigger":    "file-watch",
		"path":       watchPath,
		"eventTypes": eventTypes,
		"timestamp":  now.Format(time.RFC3339),
	})
	out["topic"] = "file-watch"
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Logic / Flow node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeRange(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		// Not an array — pass through with the single item
		return "output", out, nil
	}
	// In a real executor, range would emit one msg per item.
	// For now, set parts metadata and pass the first item.
	if len(arr) > 0 {
		SetPath(out, property, arr[0])
	}
	out["parts"] = map[string]interface{}{
		"type":  "array",
		"count": len(arr),
		"index": 0,
	}
	return "output", out, nil
}

func (s *Service) executeAggregate(node *CanvasNode, msg Msg) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeAggregateRuntime.
	time.Sleep(100 * time.Millisecond)
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["_aggregate"] = map[string]interface{}{"status": "collected"}
	return "output", out, nil
}

func (s *Service) executeRateLimit(node *CanvasNode, msg Msg) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeRateLimitRuntime.
	time.Sleep(100 * time.Millisecond)
	return "output", msg, nil
}

func (s *Service) executeFilter(node *CanvasNode, msg Msg) (string, Msg, error) {
	time.Sleep(100 * time.Millisecond)
	field, _ := node.Config["field"].(string)
	operator, _ := node.Config["operator"].(string)
	value, _ := node.Config["value"].(string)
	result := evaluateCondition(field, operator, value, msg)
	if result {
		return "pass", msg, nil
	}
	return "block", msg, nil
}

func (s *Service) executeSort(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	sortBy, _ := node.Config["sort_by"].(string)
	direction, _ := node.Config["direction"].(string)
	if direction == "" {
		direction = "asc"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return "output", out, nil
	}

	sorted := make([]interface{}, len(arr))
	copy(sorted, arr)

	sort.SliceStable(sorted, func(i, j int) bool {
		var vi, vj string
		if sortBy != "" {
			if m, ok := sorted[i].(map[string]interface{}); ok {
				vi = fmt.Sprintf("%v", m[sortBy])
			}
			if m, ok := sorted[j].(map[string]interface{}); ok {
				vj = fmt.Sprintf("%v", m[sortBy])
			}
		} else {
			vi = fmt.Sprintf("%v", sorted[i])
			vj = fmt.Sprintf("%v", sorted[j])
		}
		if direction == "desc" {
			return vi > vj
		}
		return vi < vj
	})

	SetPath(out, property, sorted)
	return "output", out, nil
}

func (s *Service) executeDeduplicate(node *CanvasNode, msg Msg) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeDeduplicateRuntime.
	time.Sleep(100 * time.Millisecond)
	return "unique", msg, nil
}

// ---------------------------------------------------------------------------
// Data transformation node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeTemplate(node *CanvasNode, msg Msg) (string, Msg, error) {
	tmpl, _ := node.Config["template"].(string)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}
	if tmpl == "" {
		return "error", msg, nil
	}

	// Simple Mustache-style replacement: {{field}} → value from msg
	result := regexp.MustCompile(`\{\{([^}]+)\}\}`).ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if v, ok := GetPath(msg, key); ok {
			return fmt.Sprintf("%v", v)
		}
		return match
	})

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	SetPath(out, outputProp, result)
	return "output", out, nil
}

func (s *Service) executeMap(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	mapping, _ := node.Config["mapping"].(map[string]interface{})

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return "output", out, nil
	}

	mapped := make([]interface{}, 0, len(arr))
	for _, item := range arr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			mapped = append(mapped, item)
			continue
		}
		newItem := make(map[string]interface{})
		for src, dst := range mapping {
			dstStr, _ := dst.(string)
			if dstStr == "" {
				dstStr = src
			}
			if v, exists := itemMap[src]; exists {
				newItem[dstStr] = v
			}
		}
		mapped = append(mapped, newItem)
	}
	SetPath(out, property, mapped)
	return "output", out, nil
}

func (s *Service) executePick(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	fieldsStr, _ := node.Config["fields"].(string)
	fields := strings.Split(fieldsStr, ",")

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		return "output", out, nil
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return "output", out, nil
	}

	picked := make(map[string]interface{})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if v, exists := obj[f]; exists {
			picked[f] = v
		}
	}
	SetPath(out, property, picked)
	return "output", out, nil
}

func (s *Service) executeOmit(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	fieldsStr, _ := node.Config["fields"].(string)
	fields := strings.Split(fieldsStr, ",")
	omitSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		omitSet[strings.TrimSpace(f)] = true
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		return "output", out, nil
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return "output", out, nil
	}

	result := make(map[string]interface{})
	for k, v := range obj {
		if !omitSet[k] {
			result[k] = v
		}
	}
	SetPath(out, property, result)
	return "output", out, nil
}

func (s *Service) executeMergeObjects(node *CanvasNode, msg Msg) (string, Msg, error) {
	sourcesStr, _ := node.Config["sources"].(string)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	merged := make(map[string]interface{})
	for _, src := range strings.Split(sourcesStr, ",") {
		src = strings.TrimSpace(src)
		if raw, ok := GetPath(out, src); ok {
			if obj, ok := raw.(map[string]interface{}); ok {
				for k, v := range obj {
					merged[k] = v
				}
			}
		}
	}
	SetPath(out, outputProp, merged)
	return "output", out, nil
}

func (s *Service) executeBase64(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	action, _ := node.Config["action"].(string)
	if action == "" {
		action = "encode"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}
	str := fmt.Sprintf("%v", raw)

	if action == "decode" {
		decoded, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			s.logger.Warn("workflows: base64 decode failed", "error", err, "property", property)
			return "error", workflowErrorMsg(msg, "base64 decode failed", map[string]interface{}{"property": property}), nil
		}
		SetPath(out, property, string(decoded))
	} else {
		SetPath(out, property, base64.StdEncoding.EncodeToString([]byte(str)))
	}
	return "output", out, nil
}

func (s *Service) executeHash(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	algorithm, _ := node.Config["algorithm"].(string)
	if algorithm == "" {
		algorithm = "sha256"
	}
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		return "output", out, nil
	}
	data := []byte(fmt.Sprintf("%v", raw))

	var hashStr string
	switch algorithm {
	case "md5":
		h := md5.Sum(data)
		hashStr = hex.EncodeToString(h[:])
	case "sha1":
		h := sha1.Sum(data)
		hashStr = hex.EncodeToString(h[:])
	case "sha512":
		h := sha512.Sum512(data)
		hashStr = hex.EncodeToString(h[:])
	default: // sha256
		h := sha256.Sum256(data)
		hashStr = hex.EncodeToString(h[:])
	}
	SetPath(out, outputProp, hashStr)
	return "output", out, nil
}

func (s *Service) executeDateFormat(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	outputFormat, _ := node.Config["output_format"].(string)
	if outputFormat == "" {
		outputFormat = time.RFC3339
	}
	tz, _ := node.Config["timezone"].(string)
	if tz == "" {
		tz = "UTC"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}

	str := fmt.Sprintf("%v", raw)
	// Try common formats
	formats := []string{time.RFC3339, time.RFC1123, "2006-01-02 15:04:05", "2006-01-02", "01/02/2006", time.RFC822}
	var parsed time.Time
	var parseErr error
	for _, f := range formats {
		parsed, parseErr = time.Parse(f, str)
		if parseErr == nil {
			break
		}
	}
	// Try unix timestamp
	if parseErr != nil {
		if unix, err := strconv.ParseInt(str, 10, 64); err == nil {
			parsed = time.Unix(unix, 0)
			parseErr = nil
		}
	}
	if parseErr != nil {
		out["payload"] = map[string]interface{}{"error": "could not parse date", "input": str}
		return "error", out, nil
	}

	loc, locErr := time.LoadLocation(tz)
	if locErr != nil {
		loc = time.UTC
	}
	SetPath(out, property, parsed.In(loc).Format(outputFormat))
	return "output", out, nil
}

func (s *Service) executeMath(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	operation, _ := node.Config["operation"].(string)
	operand := configFloat(node.Config, "operand", 0)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}

	var val float64
	switch v := raw.(type) {
	case float64:
		val = v
	case int:
		val = float64(v)
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			out["payload"] = map[string]interface{}{"error": "not a number"}
			return "error", out, nil
		}
		val = parsed
	default:
		out["payload"] = map[string]interface{}{"error": "not a number"}
		return "error", out, nil
	}

	var result float64
	switch operation {
	case "add":
		result = val + operand
	case "subtract":
		result = val - operand
	case "multiply":
		result = val * operand
	case "divide":
		if operand == 0 {
			out["payload"] = map[string]interface{}{"error": "division by zero"}
			return "error", out, nil
		}
		result = val / operand
	case "round":
		result = math.Round(val)
	case "floor":
		result = math.Floor(val)
	case "ceil":
		result = math.Ceil(val)
	case "abs":
		result = math.Abs(val)
	case "min":
		result = math.Min(val, operand)
	case "max":
		result = math.Max(val, operand)
	case "modulo":
		if operand == 0 {
			out["payload"] = map[string]interface{}{"error": "division by zero"}
			return "error", out, nil
		}
		result = math.Mod(val, operand)
	default:
		result = val
	}

	SetPath(out, outputProp, result)
	return "output", out, nil
}

func (s *Service) executeStringOps(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	operation, _ := node.Config["operation"].(string)
	arg1, _ := node.Config["arg1"].(string)
	arg2, _ := node.Config["arg2"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found"}
		return "error", out, nil
	}
	str := fmt.Sprintf("%v", raw)

	var result interface{}
	switch operation {
	case "uppercase":
		result = strings.ToUpper(str)
	case "lowercase":
		result = strings.ToLower(str)
	case "trim":
		result = strings.TrimSpace(str)
	case "replace":
		result = strings.ReplaceAll(str, arg1, arg2)
	case "split":
		if arg1 == "" {
			arg1 = ","
		}
		parts := strings.Split(str, arg1)
		iparts := make([]interface{}, len(parts))
		for i, p := range parts {
			iparts[i] = strings.TrimSpace(p)
		}
		result = iparts
	case "join":
		if arr, ok := raw.([]interface{}); ok {
			parts := make([]string, len(arr))
			for i, v := range arr {
				parts[i] = fmt.Sprintf("%v", v)
			}
			if arg1 == "" {
				arg1 = ","
			}
			result = strings.Join(parts, arg1)
		} else {
			result = str
		}
	case "substring":
		start, _ := strconv.Atoi(arg1)
		end, _ := strconv.Atoi(arg2)
		if start < 0 {
			start = 0
		}
		if end <= 0 || end > len(str) {
			end = len(str)
		}
		if start > len(str) {
			start = len(str)
		}
		result = str[start:end]
	case "reverse":
		runes := []rune(str)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		result = string(runes)
	default:
		result = str
	}
	SetPath(out, property, result)
	return "output", out, nil
}

func (s *Service) executeJSONPath(node *CanvasNode, msg Msg) (string, Msg, error) {
	expression, _ := node.Config["expression"].(string)
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	// Simple JSONPath: support $.field.subfield and $.field[*].subfield
	// For complex JSONPath, a full library would be needed.
	if expression != "" && strings.HasPrefix(expression, "$.") {
		path := strings.TrimPrefix(expression, "$.")
		path = strings.ReplaceAll(path, "[*].", ".*.")
		if v, ok := GetPath(out, "payload."+strings.ReplaceAll(path, ".*.", ".")); ok {
			SetPath(out, outputProp, v)
			return "output", out, nil
		}
	}

	// Fallback: use expression as dot-path on payload
	if raw, ok := GetPath(out, property); ok {
		SetPath(out, outputProp, raw)
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// HTTP node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeWebhookResponse(node *CanvasNode, msg Msg) (string, Msg, error) {
	statusCode := int(configFloat(node.Config, "status_code", 200))
	contentType, _ := node.Config["content_type"].(string)
	body, _ := node.Config["body"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["res"] = map[string]interface{}{
		"status":      statusCode,
		"contentType": contentType,
		"body":        body,
	}
	return "output", out, nil
}

func (s *Service) executeHTTPResponse(node *CanvasNode, msg Msg) (string, Msg, error) {
	statusCode := int(configFloat(node.Config, "status_code", 200))
	headers, _ := node.Config["headers"].(map[string]interface{})

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	hdrs := make(map[string]interface{})
	for k, v := range headers {
		hdrs[k] = v
	}
	out["res"] = map[string]interface{}{"status": statusCode}
	out["headers"] = hdrs
	return "output", out, nil
}

func (s *Service) executeGraphQLRequest(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	urlStr, _ := node.Config["url"].(string)
	query, _ := node.Config["query"].(string)
	variables, _ := node.Config["variables"].(map[string]interface{})
	headers, _ := node.Config["headers"].(map[string]interface{})

	if urlStr == "" || query == "" {
		return "", nil, fmt.Errorf("url and query are required")
	}

	body := map[string]interface{}{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	bodyBytes, _ := json.Marshal(body)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: graphql request failed", "error", err, "url", urlStr)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "graphql request failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var parsed interface{}
	if jsonErr := json.Unmarshal(respBody, &parsed); jsonErr != nil {
		parsed = string(respBody)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = parsed
	out["statusCode"] = resp.StatusCode
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Notification node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeSendConfiguredEmail(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.notifier == nil {
		return "", nil, fmt.Errorf("notification dispatcher is not configured")
	}

	to, _ := node.Config["to"].(string)
	subject, _ := node.Config["subject"].(string)
	body, _ := node.Config["body"].(string)
	serverID, _ := node.Config["server_id"].(string)
	if strings.TrimSpace(to) == "" {
		return "", nil, fmt.Errorf("recipient is required")
	}
	if strings.TrimSpace(body) == "" {
		return "", nil, fmt.Errorf("body is required")
	}

	delivery, err := s.notifier.SendEmail(ctx, corenotify.EmailRequest{
		ServerID: serverID,
		To:       to,
		Subject:  subject,
		Body:     body,
	})
	if err != nil {
		s.logger.Error("workflows: configured email send failed", "error", err, "serverID", serverID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"error":   "email send failed",
			"to":      to,
			"subject": subject,
		}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"sent":       true,
		"to":         to,
		"subject":    subject,
		"serverId":   delivery.TargetID,
		"serverName": delivery.TargetName,
	}
	return "output", out, nil
}

func (s *Service) executeSendConfiguredNotification(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.notifier == nil {
		return "", nil, fmt.Errorf("notification dispatcher is not configured")
	}

	title, _ := node.Config["title"].(string)
	message, _ := node.Config["message"].(string)
	if strings.TrimSpace(message) == "" {
		return "", nil, fmt.Errorf("message is required")
	}

	delivery, err := s.notifier.SendChannel(ctx, corenotify.ChannelRequest{
		ChannelType: "internal",
		Title:       title,
		Message:     message,
	})
	if err != nil {
		s.logger.Error("workflows: internal notification send failed", "error", err)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"error":       "notification send failed",
			"channelType": "internal",
			"title":       title,
		}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"notified":    true,
		"channelId":   delivery.TargetID,
		"channelName": delivery.TargetName,
		"channelType": delivery.TargetType,
		"title":       title,
		"message":     message,
	}
	return "output", out, nil
}

// executeSendWebhookGeneric handles Slack, Discord, and Teams webhook messages.
func (s *Service) executeSendWebhookGeneric(ctx context.Context, node *CanvasNode, msg Msg, platform string) (string, Msg, error) {
	webhookURL, _ := node.Config["webhook_url"].(string)
	message, _ := node.Config["message"].(string)
	if webhookURL == "" || message == "" {
		return "", nil, fmt.Errorf("webhook_url and message are required")
	}

	var body map[string]interface{}
	switch platform {
	case "discord":
		username, _ := node.Config["username"].(string)
		body = map[string]interface{}{"content": message}
		if username != "" {
			body["username"] = username
		}
	case "teams":
		title, _ := node.Config["title"].(string)
		color, _ := node.Config["color"].(string)
		body = map[string]interface{}{"text": message}
		if title != "" {
			body["title"] = title
		}
		if color != "" {
			body["themeColor"] = strings.TrimPrefix(color, "#")
		}
	default: // slack
		channel, _ := node.Config["channel"].(string)
		username, _ := node.Config["username"].(string)
		body = map[string]interface{}{"text": message}
		if channel != "" {
			body["channel"] = channel
		}
		if username != "" {
			body["username"] = username
		}
	}

	return s.postJSON(ctx, webhookURL, body, msg, platform)
}

func (s *Service) executeSendTelegram(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	botToken, _ := node.Config["bot_token"].(string)
	chatID, _ := node.Config["chat_id"].(string)
	message, _ := node.Config["message"].(string)
	parseMode, _ := node.Config["parse_mode"].(string)

	if botToken == "" || chatID == "" || message == "" {
		return "", nil, fmt.Errorf("bot_token, chat_id, and message are required")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	body := map[string]interface{}{"chat_id": chatID, "text": message}
	if parseMode != "" {
		body["parse_mode"] = parseMode
	}
	return s.postJSON(ctx, apiURL, body, msg, "telegram")
}

func (s *Service) executeSendOutboundWebhook(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	urlStr, _ := node.Config["url"].(string)
	method, _ := node.Config["method"].(string)
	if method == "" {
		method = "POST"
	}
	headers, _ := node.Config["headers"].(map[string]interface{})
	bodySource, _ := node.Config["body_source"].(string)
	if bodySource == "" {
		bodySource = "payload"
	}

	if urlStr == "" {
		return "", nil, fmt.Errorf("url is required")
	}

	var bodyData interface{}
	if raw, ok := GetPath(msg, bodySource); ok {
		bodyData = raw
	}
	bodyBytes, _ := json.Marshal(bodyData)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: outbound webhook failed", "error", err, "url", urlStr)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "webhook request failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"statusCode": resp.StatusCode,
		"body":       string(respBody),
	}
	return "output", out, nil
}

func (s *Service) executeSendWhatsApp(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	apiURL, _ := node.Config["api_url"].(string)
	accessToken, _ := node.Config["access_token"].(string)
	phoneNumber, _ := node.Config["phone_number"].(string)
	message, _ := node.Config["message"].(string)

	if apiURL == "" || accessToken == "" || phoneNumber == "" || message == "" {
		return "", nil, fmt.Errorf("api_url, access_token, phone_number, and message are required")
	}

	body := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phoneNumber,
		"type":              "text",
		"text":              map[string]interface{}{"body": message},
	}
	bodyBytes, _ := json.Marshal(body)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating whatsapp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: whatsapp send failed", "error", err)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "whatsapp send failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"statusCode": resp.StatusCode, "body": string(respBody)}
	if resp.StatusCode >= 400 {
		return "error", out, nil
	}
	return "output", out, nil
}

func (s *Service) executeSendSignal(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	apiURL, _ := node.Config["api_url"].(string)
	sender, _ := node.Config["sender"].(string)
	recipient, _ := node.Config["recipient"].(string)
	message, _ := node.Config["message"].(string)

	if apiURL == "" || sender == "" || recipient == "" || message == "" {
		return "", nil, fmt.Errorf("api_url, sender, recipient, and message are required")
	}

	body := map[string]interface{}{
		"message":    message,
		"number":     sender,
		"recipients": []string{recipient},
	}
	return s.postJSON(ctx, apiURL+"/v2/send", body, msg, "signal")
}

func (s *Service) executeSendNtfy(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	serverURL, _ := node.Config["server_url"].(string)
	if serverURL == "" {
		serverURL = "https://ntfy.sh"
	}
	topic, _ := node.Config["topic"].(string)
	message, _ := node.Config["message"].(string)
	title, _ := node.Config["title"].(string)
	priority, _ := node.Config["priority"].(string)
	tags, _ := node.Config["tags"].(string)

	if topic == "" || message == "" {
		return "", nil, fmt.Errorf("topic and message are required")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", serverURL+"/"+topic, strings.NewReader(message))
	if err != nil {
		return "", nil, fmt.Errorf("creating ntfy request: %w", err)
	}
	if title != "" {
		req.Header.Set("Title", title)
	}
	if priority != "" {
		req.Header.Set("Priority", priority)
	}
	if tags != "" {
		req.Header.Set("Tags", tags)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: ntfy send failed", "error", err)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "ntfy send failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"status": "sent", "topic": topic, "statusCode": resp.StatusCode}
	return "output", out, nil
}

func (s *Service) executeSendGotify(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	serverURL, _ := node.Config["server_url"].(string)
	appToken, _ := node.Config["app_token"].(string)
	message, _ := node.Config["message"].(string)
	title, _ := node.Config["title"].(string)
	priority := int(configFloat(node.Config, "priority", 5))

	if serverURL == "" || appToken == "" || message == "" {
		return "", nil, fmt.Errorf("server_url, app_token, and message are required")
	}

	body := map[string]interface{}{
		"message":  message,
		"priority": priority,
	}
	if title != "" {
		body["title"] = title
	}
	bodyBytes, _ := json.Marshal(body)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", serverURL+"/message?token="+appToken, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating gotify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: gotify send failed", "error", err)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "gotify send failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"status": "sent", "statusCode": resp.StatusCode}
	return "output", out, nil
}

// postJSON is a helper for notification nodes that POST JSON to a webhook URL.
func (s *Service) postJSON(ctx context.Context, url string, body map[string]interface{}, msg Msg, platform string) (string, Msg, error) {
	bodyBytes, _ := json.Marshal(body)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating %s request: %w", platform, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("workflows: "+platform+" send failed", "error", err, "url", url)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": platform + " send failed"}
		return "error", out, nil
	}
	defer resp.Body.Close()

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"status":     "sent",
		"platform":   platform,
		"statusCode": resp.StatusCode,
	}
	if resp.StatusCode >= 400 {
		return "error", out, nil
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Storage / State node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeReadFile(node *CanvasNode, msg Msg) (string, Msg, error) {
	filePath, _ := node.Config["path"].(string)
	encoding, _ := node.Config["encoding"].(string)
	if encoding == "" {
		encoding = "utf-8"
	}
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	if filePath == "" {
		return "", nil, fmt.Errorf("path is required")
	}

	// Resolve relative to DATA_DIR
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	fullPath := filepath.Join(dataDir, "files", filepath.Clean(filePath))

	data, err := os.ReadFile(fullPath)
	if err != nil {
		s.logger.Error("workflows: read file failed", "error", err, "path", fullPath)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "file read failed", "path": filePath}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	if encoding == "base64" {
		SetPath(out, outputProp, base64.StdEncoding.EncodeToString(data))
	} else {
		SetPath(out, outputProp, string(data))
	}
	return "output", out, nil
}

func (s *Service) executeWriteFile(node *CanvasNode, msg Msg) (string, Msg, error) {
	filePath, _ := node.Config["path"].(string)
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	mode, _ := node.Config["mode"].(string)
	if mode == "" {
		mode = "overwrite"
	}
	encoding, _ := node.Config["encoding"].(string)
	if encoding == "" {
		encoding = "utf-8"
	}

	if filePath == "" {
		return "", nil, fmt.Errorf("path is required")
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	fullPath := filepath.Join(dataDir, "files", filepath.Clean(filePath))

	raw, ok := GetPath(msg, property)
	if !ok {
		return "", nil, fmt.Errorf("property %s not found", property)
	}

	var data []byte
	if encoding == "base64" {
		str, _ := raw.(string)
		decoded, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "base64 decode failed"}
			return "error", out, nil
		}
		data = decoded
	} else {
		data = []byte(fmt.Sprintf("%v", raw))
	}

	// Ensure directory exists
	if mkErr := os.MkdirAll(filepath.Dir(fullPath), 0o755); mkErr != nil {
		s.logger.Error("workflows: write file mkdir failed", "error", mkErr)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "mkdir failed"}
		return "error", out, nil
	}

	var writeErr error
	if mode == "append" {
		f, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			writeErr = err
		} else {
			_, writeErr = f.Write(data)
			f.Close()
		}
	} else {
		writeErr = os.WriteFile(fullPath, data, 0o644)
	}

	if writeErr != nil {
		s.logger.Error("workflows: write file failed", "error", writeErr, "path", fullPath)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "file write failed", "path": filePath}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"path": filePath, "bytes": len(data), "mode": mode}
	return "output", out, nil
}

func (s *Service) executeKVGet(node *CanvasNode, msg Msg) (string, Msg, error) {
	key, _ := node.Config["key"].(string)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}
	defaultValue, _ := node.Config["default_value"].(string)

	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}

	var value sql.NullString
	err := s.db.QueryRow("SELECT value FROM workflow_kv WHERE key = ? AND (expires_at IS NULL OR expires_at > datetime('now')) LIMIT 1", key).Scan(&value)
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	if err != nil || !value.Valid {
		SetPath(out, outputProp, defaultValue)
	} else {
		// Try to parse as JSON
		var parsed interface{}
		if jsonErr := json.Unmarshal([]byte(value.String), &parsed); jsonErr == nil {
			SetPath(out, outputProp, parsed)
		} else {
			SetPath(out, outputProp, value.String)
		}
	}
	return "output", out, nil
}

func (s *Service) executeKVSet(node *CanvasNode, msg Msg) (string, Msg, error) {
	key, _ := node.Config["key"].(string)
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	ttl := int(configFloat(node.Config, "ttl", 0))

	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}

	raw, _ := GetPath(msg, property)
	valueBytes, _ := json.Marshal(raw)
	valueStr := string(valueBytes)

	var expiresAt *string
	if ttl > 0 {
		t := time.Now().UTC().Add(time.Duration(ttl) * time.Second).Format("2006-01-02 15:04:05")
		expiresAt = &t
	}

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO workflow_kv (key, value, expires_at, updated_at) VALUES (?, ?, ?, datetime('now'))",
		key, valueStr, expiresAt,
	)
	if err != nil {
		s.logger.Error("workflows: kv set failed", "error", err, "key", key)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"key": key, "status": "set"}
	return "output", out, nil
}

func (s *Service) executeKVDelete(node *CanvasNode, msg Msg) (string, Msg, error) {
	key, _ := node.Config["key"].(string)
	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}

	_, err := s.db.Exec("DELETE FROM workflow_kv WHERE key = ?", key)
	if err != nil {
		s.logger.Error("workflows: kv delete failed", "error", err, "key", key)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"key": key, "status": "deleted"}
	return "output", out, nil
}

func (s *Service) executeSQLQuery(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	query, _ := node.Config["query"].(string)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	normalizedQuery, validationErr := validateWorkflowSQLQuery(query)
	if validationErr != "" {
		return "error", workflowErrorMsg(msg, validationErr, nil), nil
	}

	rows, cleanup, err := openReadOnlyWorkflowSQLRows(ctx, s.db, s.logger, normalizedQuery)
	if err != nil {
		s.logger.Error("workflows: sql query failed", "error", err)
		return "error", workflowErrorMsg(msg, "sql query failed", nil), nil
	}
	defer cleanup()
	defer rows.Close()

	cols, _ := rows.Columns()
	results := make([]interface{}, 0, sqlQueryMaxRows)
	for rows.Next() {
		if len(results) >= sqlQueryMaxRows {
			break
		}

		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if scanErr := rows.Scan(ptrs...); scanErr != nil {
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("workflows: sql query row iteration failed", "error", err)
		return "error", workflowErrorMsg(msg, "sql query failed", nil), nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	SetPath(out, outputProp, results)
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Monitoring node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeAssert(node *CanvasNode, msg Msg) (string, Msg, error) {
	field, _ := node.Config["field"].(string)
	operator, _ := node.Config["operator"].(string)
	value, _ := node.Config["value"].(string)
	customMsg, _ := node.Config["message"].(string)

	result := evaluateCondition(field, operator, value, msg)
	if result {
		return "pass", msg, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	errMsg := customMsg
	if errMsg == "" {
		errMsg = fmt.Sprintf("assertion failed: %s %s %s", field, operator, value)
	}
	out["_assertError"] = errMsg
	return "fail", out, nil
}

func (s *Service) executeMetricRecord(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	metricName, _ := node.Config["metric_name"].(string)
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		return "", nil, fmt.Errorf("metric_name is required")
	}
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	metricType, _ := node.Config["metric_type"].(string)
	metricType = strings.ToLower(strings.TrimSpace(metricType))
	switch metricType {
	case "", "gauge":
		metricType = "gauge"
	case "counter", "event":
	default:
		metricType = "gauge"
	}
	unit, _ := node.Config["unit"].(string)
	labels := configMap(node.Config["labels"])

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	workflowID := ""
	if flowCtx != nil {
		workflowID, _ = flowCtx.FlowVars["_workflowId"].(string)
	}

	recordedID, recordedAt, numericValue, err := s.recordWorkflowMetric(workflowID, node.ID, metricName, metricType, property, unit, raw, labels)
	if err != nil {
		s.logger.Error("workflows: metric record failed", "error", err, "metric", metricName)
		out["payload"] = map[string]interface{}{"error": "metric record failed", "metric": metricName}
		return "error", out, nil
	}

	s.logger.Info("workflows: metric recorded", "metric", metricName, "type", metricType, "workflowId", workflowID)
	out["_metric"] = map[string]interface{}{
		"id":             recordedID,
		"name":           metricName,
		"type":           metricType,
		"value":          raw,
		"numericValue":   numericValue,
		"unit":           unit,
		"labels":         labels,
		"sourceProperty": property,
		"workflowId":     workflowID,
		"recordedAt":     recordedAt,
	}
	return "output", out, nil
}

func (s *Service) recordWorkflowMetric(workflowID, nodeID, metricName, metricType, sourceProperty, unit string, raw interface{}, labels map[string]interface{}) (string, string, interface{}, error) {
	if s.db == nil {
		return "", "", nil, fmt.Errorf("database is required")
	}

	metricID := xid.New().String()
	recordedAt := time.Now().UTC().Format(time.RFC3339)
	var workflowRef interface{}
	if strings.TrimSpace(workflowID) != "" {
		workflowRef = workflowID
	}

	valueJSON, err := json.Marshal(raw)
	if err != nil {
		valueJSON, _ = json.Marshal(fmt.Sprintf("%v", raw))
	}

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return "", "", nil, fmt.Errorf("marshalling metric labels: %w", err)
	}

	var numericValue interface{}
	switch metricType {
	case "counter":
		value := floatValue(raw)
		if value == 0 && raw == nil {
			value = 1
		}
		numericValue = value
	case "gauge":
		switch raw.(type) {
		case float64, float32, int, int32, int64, json.Number, string:
			numericValue = floatValue(raw)
		}
	}

	if _, err := s.db.Exec(
		`INSERT INTO workflow_metrics (
			id,
			workflow_id,
			node_id,
			metric_name,
			metric_type,
			source_property,
			unit,
			value_json,
			numeric_value,
			labels,
			recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		metricID,
		workflowRef,
		nodeID,
		metricName,
		metricType,
		sourceProperty,
		unit,
		string(valueJSON),
		numericValue,
		string(labelsJSON),
		recordedAt,
	); err != nil {
		return "", "", nil, fmt.Errorf("inserting workflow metric: %w", err)
	}

	return metricID, recordedAt, numericValue, nil
}

func (s *Service) executeHealthCheck(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	urlStr, _ := node.Config["url"].(string)
	method, _ := node.Config["method"].(string)
	if method == "" {
		method = "GET"
	}
	expectedStatus := int(configFloat(node.Config, "expected_status", 200))
	timeout := configFloat(node.Config, "timeout", 10)
	retries := int(configFloat(node.Config, "retries", 3))

	if urlStr == "" {
		return "", nil, fmt.Errorf("url is required")
	}

	var lastErr error
	var statusCode int
	for i := 0; i <= retries; i++ {
		reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, method, urlStr, nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		statusCode = resp.StatusCode
		if statusCode == expectedStatus {
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{
				"url":        urlStr,
				"statusCode": statusCode,
				"healthy":    true,
				"attempts":   i + 1,
			}
			return "healthy", out, nil
		}
		lastErr = fmt.Errorf("unexpected status %d", statusCode)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	errDetail := ""
	if lastErr != nil {
		errDetail = lastErr.Error()
	}
	out["payload"] = map[string]interface{}{
		"url":        urlStr,
		"statusCode": statusCode,
		"healthy":    false,
		"error":      errDetail,
		"attempts":   retries + 1,
	}
	return "unhealthy", out, nil
}

// ---------------------------------------------------------------------------
// External service node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeSSHExec(node *CanvasNode, msg Msg) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeSSHExecRuntime.
	host, _ := node.Config["host"].(string)
	command, _ := node.Config["command"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"host":    host,
		"command": command,
		"status":  "stub",
		"note":    "SSH execution requires runtime implementation",
	}
	return "output", out, nil
}

func (s *Service) executeFTPUpload(node *CanvasNode, msg Msg) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeFTPUploadRuntime.
	host, _ := node.Config["host"].(string)
	remotePath, _ := node.Config["remote_path"].(string)
	protocol, _ := node.Config["protocol"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"host":       host,
		"remotePath": remotePath,
		"protocol":   protocol,
		"status":     "stub",
		"note":       "FTP/SFTP upload requires runtime implementation",
	}
	return "output", out, nil
}

func (s *Service) executeDNSLookup(node *CanvasNode, msg Msg) (string, Msg, error) {
	hostname, _ := node.Config["hostname"].(string)
	recordType, _ := node.Config["record_type"].(string)
	if recordType == "" {
		recordType = "A"
	}

	if hostname == "" {
		return "", nil, fmt.Errorf("hostname is required")
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	var records []string
	var err error
	switch recordType {
	case "A", "AAAA":
		ips, lookupErr := net.LookupHost(hostname)
		err = lookupErr
		records = ips
	case "MX":
		mxs, lookupErr := net.LookupMX(hostname)
		err = lookupErr
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
		}
	case "TXT":
		txts, lookupErr := net.LookupTXT(hostname)
		err = lookupErr
		records = txts
	case "CNAME":
		cname, lookupErr := net.LookupCNAME(hostname)
		err = lookupErr
		records = []string{cname}
	case "NS":
		nss, lookupErr := net.LookupNS(hostname)
		err = lookupErr
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	}

	if err != nil {
		s.logger.Warn("workflows: dns lookup failed", "error", err, "hostname", hostname, "recordType", recordType)
		return "error", workflowErrorMsg(msg, "dns lookup failed", map[string]interface{}{"hostname": hostname, "recordType": recordType}), nil
	}

	irecords := make([]interface{}, len(records))
	for i, r := range records {
		irecords[i] = r
	}
	out["payload"] = map[string]interface{}{
		"hostname":   hostname,
		"recordType": recordType,
		"records":    irecords,
	}
	return "output", out, nil
}

func (s *Service) executePing(node *CanvasNode, msg Msg) (string, Msg, error) {
	host, _ := node.Config["host"].(string)
	timeout := configFloat(node.Config, "timeout", 5)

	if host == "" {
		return "", nil, fmt.Errorf("host is required")
	}

	// TCP ping as a portable alternative to ICMP (no raw socket needed)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host+":80", time.Duration(timeout)*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		// Try port 443 as fallback
		start = time.Now()
		conn, err = net.DialTimeout("tcp", host+":443", time.Duration(timeout)*time.Second)
		elapsed = time.Since(start)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	if err != nil {
		out["payload"] = map[string]interface{}{
			"host":      host,
			"reachable": false,
			"error":     "host unreachable",
		}
		return "error", out, nil
	}
	conn.Close()

	out["payload"] = map[string]interface{}{
		"host":      host,
		"reachable": true,
		"latencyMs": elapsed.Milliseconds(),
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Advanced Docker node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeContainerStats(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	snapshot, err := FetchContainerMetric(ctx, cli, containerName)
	if err != nil {
		s.logger.Error("workflows: container stats failed", "error", err, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container stats failed", "container": containerName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"container":  containerName,
		"cpuPercent": snapshot.CPUPercent,
		"memUsage":   snapshot.MemUsage,
		"memLimit":   snapshot.MemLimit,
		"memPercent": snapshot.MemPercent,
		"netRx":      snapshot.NetRx,
		"netTx":      snapshot.NetTx,
		"blockRead":  snapshot.BlockRead,
		"blockWrite": snapshot.BlockWrite,
		"pids":       snapshot.PIDs,
	}
	return "output", out, nil
}

func (s *Service) executeContainerRename(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	newName, _ := node.Config["new_name"].(string)
	if containerName == "" || newName == "" {
		return "", nil, fmt.Errorf("container and new_name are required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if renameErr := cli.ContainerRename(opCtx, containerName, newName); renameErr != nil {
		s.logger.Error("workflows: container rename failed", "error", renameErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "rename failed", "container": containerName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"container": containerName, "newName": newName, "status": "renamed"}
	return "output", out, nil
}

func (s *Service) executeContainerWait(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}
	condition, _ := node.Config["condition"].(string)
	if condition == "" {
		condition = "not-running"
	}
	timeout := configFloat(node.Config, "timeout", 60)

	opCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var waitCond container.WaitCondition
	switch condition {
	case "next-exit":
		waitCond = container.WaitConditionNextExit
	case "removed":
		waitCond = container.WaitConditionRemoved
	default:
		waitCond = container.WaitConditionNotRunning
	}

	statusCh, errCh := cli.ContainerWait(opCtx, containerName, waitCond)
	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	select {
	case result := <-statusCh:
		out["payload"] = map[string]interface{}{
			"container": containerName,
			"exitCode":  result.StatusCode,
			"condition": condition,
		}
		if result.StatusCode != 0 {
			return "error", out, nil
		}
		return "output", out, nil
	case waitErr := <-errCh:
		if waitErr != nil {
			s.logger.Error("workflows: container wait failed", "error", waitErr, "container", containerName)
			out["payload"] = map[string]interface{}{"error": "container wait failed", "container": containerName}
			return "error", out, nil
		}
		return "output", out, nil
	case <-opCtx.Done():
		out["payload"] = map[string]interface{}{"error": "timeout waiting for container", "container": containerName}
		return "error", out, nil
	}
}

func (s *Service) executeImageBuild(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	// Legacy fallback retained for compatibility. Runtime execution uses executeImageBuildRuntime.
	tag, _ := node.Config["tag"].(string)
	dockerfile, _ := node.Config["dockerfile"].(string)
	contextPath, _ := node.Config["context_path"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"tag":         tag,
		"dockerfile":  dockerfile,
		"contextPath": contextPath,
		"status":      "stub",
		"note":        "image build requires tar context streaming implementation",
	}
	return "output", out, nil
}

func (s *Service) executeImagePush(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	pushCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	reader, pushErr := cli.ImagePush(pushCtx, imageName, image.PushOptions{})
	if pushErr != nil {
		s.logger.Error("workflows: image push failed", "error", pushErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image push failed", "image": imageName}
		return "error", out, nil
	}
	defer reader.Close()
	if _, copyErr := io.Copy(io.Discard, reader); copyErr != nil {
		s.logger.Warn("workflows: image push stream drain failed", "error", copyErr, "image", imageName, "envID", envID)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"image": imageName, "status": "pushed"}
	return "output", out, nil
}

func (s *Service) executeRegistrySearch(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	term, _ := node.Config["term"].(string)
	if term == "" {
		return "", nil, fmt.Errorf("search term is required")
	}
	limit := int(configFloat(node.Config, "limit", 25))

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, searchErr := cli.ImageSearch(opCtx, term, registry.SearchOptions{Limit: limit})
	if searchErr != nil {
		s.logger.Error("workflows: registry search failed", "error", searchErr, "term", term, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "registry search failed", "term": term}
		return "error", out, nil
	}

	items := make([]interface{}, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]interface{}{
			"name":        r.Name,
			"description": r.Description,
			"stars":       r.StarCount,
			"official":    r.IsOfficial,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

func (s *Service) executeComposeUp(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// List containers in this stack to report status
	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+stackName)
	containers, _ := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})

	// Start any stopped containers in the stack
	started := 0
	for _, c := range containers {
		if c.State != "running" {
			if startErr := cli.ContainerStart(opCtx, c.ID, container.StartOptions{}); startErr == nil {
				started++
			}
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"stack":   stackName,
		"total":   len(containers),
		"started": started,
		"status":  "up",
	}
	return "output", out, nil
}

func (s *Service) executeComposeDown(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	removeVolumes := false
	if v, ok := node.Config["remove_volumes"].(bool); ok {
		removeVolumes = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+stackName)
	containers, listErr := cli.ContainerList(opCtx, container.ListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: compose down list failed", "error", listErr, "stack", stackName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "compose down failed", "stack": stackName}
		return "error", out, nil
	}

	stopped := 0
	removed := 0
	for _, c := range containers {
		if c.State == "running" {
			if stopErr := cli.ContainerStop(opCtx, c.ID, container.StopOptions{}); stopErr == nil {
				stopped++
			}
		}
		if rmErr := cli.ContainerRemove(opCtx, c.ID, container.RemoveOptions{Force: true, RemoveVolumes: removeVolumes}); rmErr == nil {
			removed++
		}
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"stack":   stackName,
		"stopped": stopped,
		"removed": removed,
		"total":   len(containers),
		"status":  "down",
	}
	return "output", out, nil
}
