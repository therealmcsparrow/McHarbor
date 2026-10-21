// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package workflows stores visual workflows and executes their node graphs, from manual runs, webhooks and background triggers.
package workflows

import (
	"bytes"
	"context"
	"crypto/hmac"
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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/registry"
	dockerclient "github.com/moby/moby/client"
	"gopkg.in/yaml.v3"

	"github.com/rs/xid"
	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/db"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	coreemail "github.com/therealmcsparrow/mcharbor/core/email"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	coresettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// nodeLongOpTimeout bounds a workflow node whose work is an image build or a
// storage round-trip rather than a single API call.
const nodeLongOpTimeout = 2 * time.Minute

func unusedImagePruneFilters() dockerclient.Filters {
	return make(dockerclient.Filters).Add("dangling", "false")
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

// PluginNodeExecutor runs canvas nodes whose action is
// `plugin:<slug>:<key>` — nodes an installed plugin registered through
// its NodeRegistry. Implemented by internal/bootstrap over the plugin
// runtime so this package never imports it.
type PluginNodeExecutor interface {
	IsPluginNode(actionKey string) bool
	ExecutePluginNode(ctx context.Context, actionKey string, in PluginNodeInput) (port string, msg map[string]interface{}, err error)
}

// PluginNodeInput is the engine-side view of what a plugin node
// receives. Mirrors plugin.NodeExecInput field for field; the bridge
// copies it across so the two packages stay decoupled.
type PluginNodeInput struct {
	Key           string
	Config        map[string]interface{}
	Msg           map[string]interface{}
	EnvironmentID string
	UserID        string
	WorkflowID    string
}

// Service handles workflow persistence and business logic.
type Service struct {
	db             *sql.DB
	pool           *docker.ClientPool
	logger         *slog.Logger
	enc            *encryption.Service
	dataDir        string
	backupCrypto   *backupcrypto.Service
	onLinkOut      LinkOutCallback
	customExecutor interface {
		ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error)
		IsCustomNode(key string) bool
	}
	pluginNodes PluginNodeExecutor
	// nodeAlias resolves a bare stored action a loaded plugin claims through a
	// workflow-node-alias capability. Defaults to the live capability registry.
	nodeAlias NodeAliasResolver
	// channelAlias and emailChannels answer the comm plugins' channel claims; nil
	// means the live capability registry (see channel_caps.go).
	channelAlias  ChannelAliasResolver
	emailChannels func() []string
	notifier      *corenotify.Dispatcher
	imageScanner  ImageScanner
	// Runtime adapters for cross-module integration. Injected from
	// main.go via the Set*Runtime methods below so this package does
	// not import its sibling modules.
	// secretResolver substitutes ${secret:NAME} in node configs at execution
	// time. Built in NewService rather than injected — see resolveNodeSecrets.
	secretResolver SecretResolver
	backupRunner   ContainerBackupRunner
	stackLinker    StackContainerLinker
	storageReader  StorageLocationReader
	storageObjects StorageObjectOperator
}

// NewService creates a new workflow service.
func NewService(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger, enc *encryption.Service, notifier *corenotify.Dispatcher, dataDir string, backupCrypto *backupcrypto.Service) *Service {
	svc := &Service{db: db, pool: pool, logger: logger, enc: enc, notifier: notifier, dataDir: dataDir, backupCrypto: backupCrypto,
		nodeAlias: plugincaps.Default.NodeAliasAction}
	if db != nil && enc != nil {
		svc.secretResolver = secrets.NewService(db, enc, nil)
	}
	return svc
}

// SetCustomExecutor registers the custom node executor for handling third-party nodes.
func (s *Service) SetCustomExecutor(executor interface {
	ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error)
	IsCustomNode(key string) bool
}) {
	s.customExecutor = executor
}

// SetPluginNodeExecutor registers the bridge that runs plugin-contributed
// workflow nodes (`plugin:<slug>:<key>` actions).
func (s *Service) SetPluginNodeExecutor(executor PluginNodeExecutor) {
	s.pluginNodes = executor
}

// NodeAliasResolver maps a bare canvas action to the `plugin:<slug>:<key>` action of
// the loaded plugin that claims it, and reports false when nothing claims it.
type NodeAliasResolver func(action string) (string, bool)

// SetNodeAliasResolver replaces the capability-registry resolver; tests use it.
func (s *Service) SetNodeAliasResolver(r NodeAliasResolver) {
	s.nodeAlias = r
}

// resolveAction returns the action a node runs as: a claimed alias becomes its
// plugin's composite action, anything else is returned unchanged. Built-in actions
// never reach it from ExecuteNode, whose switch matches them first.
func (s *Service) resolveAction(action string) string {
	if s == nil || s.nodeAlias == nil {
		return action
	}
	if resolved, ok := s.nodeAlias(action); ok {
		return resolved
	}
	return action
}

// SetImageScanner registers the vulnerability scanner runtime adapter.
func (s *Service) SetImageScanner(scanner ImageScanner) {
	s.imageScanner = scanner
}

// SetLinkOutCallback registers a callback invoked when a link-out node fires.
func (s *Service) SetLinkOutCallback(cb LinkOutCallback) {
	s.onLinkOut = cb
}

// SetContainerBackupRuntime registers the runtime adapter that
// fulfils container-backup workflow nodes.
func (s *Service) SetContainerBackupRuntime(r ContainerBackupRunner) {
	s.backupRunner = r
}

// SetStackContainerLinkerRuntime registers the runtime adapter
// that fulfils stack-link / stack-unlink workflow nodes.
func (s *Service) SetStackContainerLinkerRuntime(r StackContainerLinker) {
	s.stackLinker = r
}

// SetStorageLocationRuntime registers the runtime adapter that
// fulfils storage-location workflow nodes.
func (s *Service) SetStorageLocationRuntime(r StorageLocationReader) {
	s.storageReader = r
}

// SetStorageObjectRuntime registers the runtime adapter that fulfils
// storage-object and storage-location-test workflow nodes.
func (s *Service) SetStorageObjectRuntime(r StorageObjectOperator) {
	s.storageObjects = r
}

// List returns paginated workflows, optionally filtered by status.
func (s *Service) List(ctx context.Context, status string, page, perPage int) ([]Workflow, int64, error) {
	offset := (page - 1) * perPage

	var total int64
	var rows *sql.Rows
	var err error

	if status != "" {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflows WHERE status = ?", status).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflows: %w", err)
		}
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at, self_host_authorized FROM workflows WHERE status = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?",
			status, perPage, offset,
		)
	} else {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflows").Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflows: %w", err)
		}
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at, self_host_authorized FROM workflows ORDER BY updated_at DESC LIMIT ? OFFSET ?",
			perPage, offset,
		)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("listing workflows: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var items []Workflow
	for rows.Next() {
		var wf Workflow
		if err := rows.Scan(&wf.ID, &wf.Name, &wf.Description, &wf.Status, &wf.CanvasData, &wf.Variables, &wf.CreatedBy, &wf.UpdatedBy, &wf.LastRunAt, &wf.CreatedAt, &wf.UpdatedAt, &wf.SelfHostAuthorized); err != nil {
			return nil, 0, fmt.Errorf("scanning workflow: %w", err)
		}
		if err := s.decryptWorkflowPayload(&wf); err != nil {
			return nil, 0, err
		}
		items = append(items, wf)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating workflows: %w", err)
	}
	if items == nil {
		items = []Workflow{}
	}

	return items, total, nil
}

// Get returns a single workflow by ID.
func (s *Service) Get(ctx context.Context, id string) (*Workflow, error) {
	var wf Workflow
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at, self_host_authorized FROM workflows WHERE id = ?", id,
	).Scan(&wf.ID, &wf.Name, &wf.Description, &wf.Status, &wf.CanvasData, &wf.Variables, &wf.CreatedBy, &wf.UpdatedBy, &wf.LastRunAt, &wf.CreatedAt, &wf.UpdatedAt, &wf.SelfHostAuthorized)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow: %w", err)
	}
	if err := s.decryptWorkflowPayload(&wf); err != nil {
		return nil, err
	}

	return &wf, nil
}

func (s *Service) encryptWorkflowField(value, label string) (string, error) {
	if value == "" {
		return value, nil
	}
	if s.enc == nil {
		return "", fmt.Errorf("encrypting workflow %s: %w", label, ErrWorkflowEncryptionUnavailable)
	}
	enc, err := s.enc.Encrypt(value)
	if err != nil {
		return "", fmt.Errorf("encrypting workflow %s: %w", label, err)
	}
	return enc, nil
}

func (s *Service) decryptWorkflowField(value, label string) (string, error) {
	if value == "" || !encryption.IsEncrypted(value) {
		return value, nil
	}
	if s.enc == nil {
		return "", fmt.Errorf("decrypting workflow %s: %w", label, ErrWorkflowEncryptionUnavailable)
	}
	plain, err := s.enc.Decrypt(value)
	if err != nil {
		return "", fmt.Errorf("decrypting workflow %s: %w", label, err)
	}
	return plain, nil
}

func (s *Service) decryptWorkflowPayload(wf *Workflow) error {
	canvasData, err := s.decryptWorkflowField(wf.CanvasData, "canvas data")
	if err != nil {
		return err
	}
	variables, err := s.decryptWorkflowField(wf.Variables, "variables")
	if err != nil {
		return err
	}
	wf.CanvasData = canvasData
	wf.Variables = variables
	return nil
}

// Create inserts a new workflow and returns it.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Workflow, error) {
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

	storedCanvas, err := s.encryptWorkflowField(wf.CanvasData, "canvas data")
	if err != nil {
		return nil, err
	}
	storedVariables, err := s.encryptWorkflowField(wf.Variables, "variables")
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		wf.ID, wf.Name, wf.Description, wf.Status, storedCanvas, storedVariables, wf.CreatedBy, wf.UpdatedBy, wf.LastRunAt, wf.CreatedAt, wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating workflow: %w", err)
	}

	return &wf, nil
}

// Export returns a portable workflow export payload.
func (s *Service) Export(ctx context.Context, id string) (*WorkflowExport, error) {
	wf, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, nil
	}

	return &WorkflowExport{
		Kind:       "mcharbor.workflow",
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Workflow: WorkflowExportRecord{
			OriginalID:        wf.ID,
			Name:              wf.Name,
			Description:       wf.Description,
			Status:            wf.Status,
			CanvasData:        wf.CanvasData,
			Variables:         wf.Variables,
			OriginalCreatedAt: wf.CreatedAt,
			OriginalUpdatedAt: wf.UpdatedAt,
		},
	}, nil
}

// Import creates a draft workflow from an exported workflow payload.
func (s *Service) Import(ctx context.Context, input WorkflowExport) (*Workflow, error) {
	if input.Kind != "mcharbor.workflow" || input.Version < 1 {
		return nil, fmt.Errorf("%w: unsupported format", ErrInvalidWorkflowExport)
	}

	record := input.Workflow
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: workflow name is required", ErrInvalidWorkflowExport)
	}
	canvasData := strings.TrimSpace(record.CanvasData)
	if canvasData == "" {
		canvasData = `{"nodes":[],"edges":[],"groups":[],"viewport":{"x":0,"y":0,"zoom":1}}`
	}
	if err := validateJSONObject(canvasData); err != nil {
		return nil, fmt.Errorf("%w: validating workflow canvas data", ErrInvalidWorkflowExport)
	}
	variables := strings.TrimSpace(record.Variables)
	if variables == "" {
		variables = "{}"
	}
	if err := validateJSONObject(variables); err != nil {
		return nil, fmt.Errorf("%w: validating workflow variables", ErrInvalidWorkflowExport)
	}
	authorized, err := s.evaluateSelfHost(ctx, canvasData)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	wf := Workflow{
		ID:          xid.New().String(),
		Name:        name,
		Description: record.Description,
		Status:      "draft",
		CanvasData:  canvasData,
		Variables:   variables,
		CreatedAt:   now,
		UpdatedAt:   now,

		SelfHostAuthorized: authorized,
	}

	storedCanvas, err := s.encryptWorkflowField(wf.CanvasData, "canvas data")
	if err != nil {
		return nil, err
	}
	storedVariables, err := s.encryptWorkflowField(wf.Variables, "variables")
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at, self_host_authorized) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		wf.ID, wf.Name, wf.Description, wf.Status, storedCanvas, storedVariables, wf.CreatedBy, wf.UpdatedBy, wf.LastRunAt, wf.CreatedAt, wf.UpdatedAt, wf.SelfHostAuthorized,
	)
	if err != nil {
		return nil, fmt.Errorf("importing workflow: %w", err)
	}

	return &wf, nil
}

func validateJSONObject(raw string) error {
	var value map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value == nil {
		return fmt.Errorf("json object is required")
	}
	if decoder.Decode(&value) != io.EOF {
		return fmt.Errorf("multiple JSON documents")
	}
	return nil
}

// Update applies partial updates to an existing workflow and returns the updated record.
// Every edit re-decides the host_self record before anything is written, so an edit by
// someone without host_self.manage clears it, and one that adds a static McHarbor target
// is refused with a *selfguard.Refusal.
func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*Workflow, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM workflows WHERE id = ?", id).Scan(&exists); err != nil {
		return nil, nil //nolint:nilerr // any lookup failure reads as not found; the handler maps a nil workflow to 404
	}

	var canvas string
	if input.CanvasData != nil {
		canvas = *input.CanvasData
	} else {
		current, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
		canvas = current.CanvasData
	}
	authorized, err := s.evaluateSelfHost(ctx, canvas)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET self_host_authorized = ?, updated_at = ? WHERE id = ?", authorized, now, id); err != nil {
		return nil, fmt.Errorf("recording workflow host_self decision: %w", err)
	}

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow description: %w", err)
		}
	}
	if input.Status != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET status = ?, updated_at = ? WHERE id = ?", *input.Status, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow status: %w", err)
		}
	}
	if input.CanvasData != nil {
		stored, err := s.encryptWorkflowField(*input.CanvasData, "canvas data")
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET canvas_data = ?, updated_at = ? WHERE id = ?", stored, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow canvas data: %w", err)
		}
	}
	if input.Variables != nil {
		stored, err := s.encryptWorkflowField(*input.Variables, "variables")
		if err != nil {
			return nil, err
		}
		// variables_secret_id is cleared here for the same reason every other
		// adopted module clears its pointer: the vault copy must never outlive
		// the value it copied.
		if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET variables = ?, variables_secret_id = NULL, updated_at = ? WHERE id = ?", stored, now, id); err != nil {
			return nil, fmt.Errorf("updating workflow variables: %w", err)
		}
	}

	return s.Get(ctx, id)
}

// Delete removes a workflow by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting workflow: %w", err)
	}

	return db.RowsAffected(result) > 0, nil
}

// ListRuns returns paginated workflow runs, optionally filtered by workflow ID.
func (s *Service) ListRuns(ctx context.Context, workflowID string, page, perPage int) ([]WorkflowRun, int64, error) {
	offset := (page - 1) * perPage

	var total int64
	var rows *sql.Rows
	var err error

	if workflowID != "" {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflow_runs WHERE workflow_id = ?", workflowID).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflow runs: %w", err)
		}
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at FROM workflow_runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?",
			workflowID, perPage, offset,
		)
	} else {
		err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflow_runs").Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting workflow runs: %w", err)
		}
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at FROM workflow_runs ORDER BY started_at DESC LIMIT ? OFFSET ?",
			perPage, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing workflow runs: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var runs []WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		if err := rows.Scan(&run.ID, &run.WorkflowID, &run.Status, &run.Trigger, &run.DurationMs, &run.NodeCount, &run.Error, &run.StartedAt, &run.FinishedAt); err != nil {
			s.logger.Error("workflows: scan run error", "error", err)
			continue
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating workflow runs: %w", err)
	}
	if runs == nil {
		runs = []WorkflowRun{}
	}

	return runs, total, nil
}

// RecordRun inserts a workflow run record into the database.
func (s *Service) RecordRun(ctx context.Context, workflowID, trigger, status string, durationMs int64, nodeCount int, errMsg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	finishedAt := ""
	if status != "running" {
		finishedAt = now
		telemetry.WorkflowRun(trigger, status, time.Duration(durationMs)*time.Millisecond)
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO workflow_runs (id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		xid.New().String(), workflowID, status, trigger, durationMs, nodeCount, errMsg, now, finishedAt,
	)
	if err != nil {
		s.logger.Error("workflows: record run error", "error", err)
	}
}

// UpdateLastRunAt sets the last_run_at and updated_at timestamps on a workflow.
func (s *Service) UpdateLastRunAt(ctx context.Context, id string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, "UPDATE workflows SET last_run_at = ?, updated_at = ? WHERE id = ?", now, now, id); err != nil {
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
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var result []struct{ ID, CanvasData string }
	for rows.Next() {
		var item struct{ ID, CanvasData string }
		if err := rows.Scan(&item.ID, &item.CanvasData); err != nil {
			continue
		}
		canvasData, err := s.decryptWorkflowField(item.CanvasData, "canvas data")
		if err != nil {
			return nil, err
		}
		item.CanvasData = canvasData
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating active workflows: %w", err)
	}
	return result, nil
}

// ListActiveDockerEnvIDs returns IDs of all active Docker environments.
func (s *Service) ListActiveDockerEnvIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM environments WHERE is_active = 1 LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("querying docker environments: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating docker environments: %w", err)
	}
	return ids, nil
}

// StoreLinkMessage upserts a link message for a link-out node.
func (s *Service) StoreLinkMessage(ctx context.Context, workflowID, nodeID, name string, msg Msg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling link message: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
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
func (s *Service) FetchLinkMessage(ctx context.Context, workflowID, nodeID string) (Msg, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
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
func (s *Service) ListLinkOutputs(ctx context.Context) ([]LinkOutputInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, canvas_data FROM workflows LIMIT 1000",
	)
	if err != nil {
		return nil, fmt.Errorf("querying workflows for link outputs: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var results []LinkOutputInfo
	for rows.Next() {
		var wfID, wfName, canvasRaw string
		if err := rows.Scan(&wfID, &wfName, &canvasRaw); err != nil {
			continue
		}
		decryptedCanvas, err := s.decryptWorkflowField(canvasRaw, "canvas data")
		if err != nil {
			continue
		}
		var canvas CanvasData
		if err := json.Unmarshal([]byte(decryptedCanvas), &canvas); err != nil {
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating link outputs: %w", err)
	}
	if results == nil {
		results = []LinkOutputInfo{}
	}
	return results, nil
}

// executePluginNode dispatches a `plugin:<slug>:<key>` action to the
// plugin runtime. A missing bridge or an unregistered key is reported
// on the error port rather than as a hard failure, so a workflow that
// references a plugin which was later uninstalled degrades the same
// way a custom node with a broken script does.
func (s *Service) executePluginNode(ctx context.Context, node *CanvasNode, msg Msg, flowCtx *FlowContext, envID string) (string, Msg, error) {
	fail := func(reason string) (string, Msg, error) {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": reason, "action": node.Action}
		return "error", out, nil
	}
	if s.pluginNodes == nil || !s.pluginNodes.IsPluginNode(node.Action) {
		s.logger.Warn("workflows: plugin node has no executor", "action", node.Action)
		return fail("plugin node unavailable")
	}
	key := node.Action[strings.LastIndex(node.Action, ":")+1:]
	if msg == nil {
		// A trigger node run by hand from the editor has no event to
		// carry; give it the empty msg every built-in trigger starts
		// with, so the plugin and the downstream nodes see one shape.
		msg = NewMsg(nil)
		msg["topic"] = key
	}
	in := PluginNodeInput{
		Key:           key,
		Config:        node.Config,
		Msg:           msg,
		EnvironmentID: envID,
	}
	if flowCtx != nil {
		in.UserID = flowCtx.UserID
		if id, ok := flowCtx.FlowVars["_workflowId"].(string); ok {
			in.WorkflowID = id
		}
	}
	port, outMsg, err := s.pluginNodes.ExecutePluginNode(ctx, node.Action, in)
	if err != nil {
		s.logger.Error("workflows: plugin node execution failed", "error", err, "action", node.Action)
		return fail("plugin node execution failed")
	}
	if outMsg == nil {
		outMsg = CloneMsg(msg)
	}
	outMsg = EnsureMsgID(outMsg)
	if port == "" {
		port = "output"
	}
	return port, outMsg, nil
}

// ExecuteNode executes a single workflow node and returns the output port, output msg, and error.
// Trigger nodes (msg == nil) create the initial msg; all other nodes receive and transform the msg.
func (s *Service) ExecuteNode(ctx context.Context, node *CanvasNode, msg Msg, flowCtx *FlowContext, fallbackEnvID string) (string, Msg, error) {
	applyLegacyNodeAlias(node)
	s.applyChannelAlias(node)
	nodeEnvID, _ := node.Config["environment"].(string)
	if nodeEnvID == "" {
		nodeEnvID = fallbackEnvID
	}

	// One call, above the switch, covers every node type. Resolution produces
	// a copy — the plaintext must not reach the stored canvas — and a missing
	// reference fails the node rather than running it with an empty
	// credential.
	resolvedNode, err := s.resolveNodeSecrets(ctx, node, nodeEnvID)
	if err != nil {
		return "error", msg, err
	}
	node = resolvedNode

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
		tz, _ := node.Config["timezone"].(string)
		if tz == "" {
			tz = "UTC"
		}
		out := NewMsg(map[string]interface{}{
			"trigger":   "schedule",
			"cron":      cron,
			"timezone":  tz,
			"timestamp": now.Format(time.RFC3339),
			"datetime":  now.Format("2006-01-02 15:04:05 UTC"),
			"unix":      now.Unix(),
		})
		out["topic"] = "schedule"
		return "output", out, nil

	case "docker-event-trigger":
		return s.executeDockerEventTrigger(ctx, node, nodeEnvID)

	case "registry-tag-trigger":
		return s.executeRegistryTagTrigger(ctx, node, nodeEnvID)

	case "metric-trigger":
		return s.executeMetricTrigger(ctx, node, nodeEnvID)

	case "http-request":
		return s.executeHTTPRequest(ctx, node, msg)

	case "send-email":
		return s.executeSendConfiguredEmail(ctx, node, msg)

	case "send-notification":
		return s.executeSendConfiguredNotification(ctx, node, msg)

	case "condition":
		return s.executeConditionNode(node, msg)

	case "approval":
		return s.executeApproval(node, msg)

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
			switch actionType {
			case "set":
				SetPath(target, property, value)
			case "delete":
				DeletePath(target, property)
			}
		}

		return "output", out, nil

	case "debug":
		time.Sleep(100 * time.Millisecond)
		// Pass msg through unchanged — debug output is a side-effect via SSE
		return "output", msg, nil

	case "docker-prune":
		return s.executeDockerPrune(ctx, node, msg, nodeEnvID)

	case "image-vulnerability-scan":
		return s.executeImageVulnerabilityScan(ctx, node, msg, nodeEnvID)

	case "host-exec":
		return s.executeHostExec(ctx, node, msg, nodeEnvID)

	case "join":
		return s.executeJoinRuntime(node, msg, flowCtx)

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
	case "container-logs":
		return s.executeContainerLogs(ctx, node, msg, nodeEnvID)
	case "container-exec":
		return s.executeContainerExec(ctx, node, msg, nodeEnvID)
	case "container-create":
		return s.executeContainerCreate(ctx, node, msg, nodeEnvID)
	case "container-backup-run":
		return s.executeContainerBackupRun(ctx, node, msg, nodeEnvID)
	case "container-backup-plan-run":
		return s.executeContainerBackupPlanRun(ctx, node, msg)
	case "container-backup-download":
		return s.executeContainerBackupDownload(ctx, node, msg)
	case "container-backup-restore":
		return s.executeContainerBackupRestore(ctx, node, msg)
	case "container-backup-manage":
		return s.executeContainerBackupManage(ctx, node, msg, nodeEnvID)
	case "container-stack-link":
		return s.executeContainerStackLink(ctx, node, msg, nodeEnvID)
	case "container-stack-unlink":
		return s.executeContainerStackUnlink(ctx, node, msg, nodeEnvID)

	// Stack nodes
	case "stack-backup":
		return s.executeStackBackup(ctx, node, msg, nodeEnvID)

	// Image nodes

	// Volume nodes

	// Network nodes

	// Advanced Docker nodes
	case "container-stats":
		return s.executeContainerStats(ctx, node, msg, nodeEnvID)
	case "image-build":
		return s.executeImageBuildRuntime(ctx, node, msg, nodeEnvID)
	case "registry-search":
		return s.executeRegistrySearch(ctx, node, msg, nodeEnvID)

	// Storage nodes
	case "storage-location-list":
		return s.executeStorageLocationList(ctx, node, msg)
	case "storage-location-get":
		return s.executeStorageLocationGet(ctx, node, msg)
	case "storage-object":
		return s.executeStorageObject(ctx, node, msg)
	case "storage-location-test":
		return s.executeStorageLocationTest(ctx, node, msg)

	// Trigger nodes
	case "file-watch-trigger":
		return s.executeFileWatchTrigger(ctx, node)

	// Logic / Flow nodes
	case "range":
		return s.executeRangeRuntime(node, msg, flowCtx)
	case "aggregate":
		return s.executeAggregateRuntime(node, msg, flowCtx)
	case "rate-limit":
		return s.executeRateLimitRuntime(ctx, node, msg, flowCtx)
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
	case "merge-objects":
		return s.executeMergeObjects(node, msg)
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
	case "send-communication":
		return s.executeSendCommunication(ctx, node, msg)
	case NodeITSMOpen, NodeITSMUpdate, NodeITSMResolve:
		return s.executeITSM(ctx, node, msg)
	case NodeITSMStatus:
		out := NewMsg(map[string]interface{}{})
		out["topic"] = NodeITSMStatus
		return "output", out, nil

	// Consolidated nodes (routers live in merged_nodes.go)
	case "parse":
		return s.executeParse(node, msg)
	case "container-action":
		return s.executeContainerAction(ctx, node, msg, nodeEnvID)
	case "inspect":
		return s.executeInspect(ctx, node, msg, nodeEnvID)
	case "list":
		return s.executeList(ctx, node, msg, nodeEnvID)
	case "kv":
		return s.executeKV(ctx, node, msg)
	case "environment-info":
		return s.executeEnvironmentInfo(ctx, node, msg, nodeEnvID)
	case "encode":
		return s.executeEncode(node, msg)
	case "object":
		return s.executeObject(node, msg)
	case "file":
		return s.executeFile(node, msg)
	case "image-manage":
		return s.executeImageManage(ctx, node, msg, nodeEnvID)
	case "network-manage":
		return s.executeNetworkManage(ctx, node, msg, nodeEnvID)
	case "volume-manage":
		return s.executeVolumeManage(ctx, node, msg, nodeEnvID)
	case "stack-manage":
		return s.executeStackManage(ctx, node, msg, nodeEnvID)
	case "container-trigger":
		return s.executeContainerTrigger(ctx, node, nodeEnvID)
	case "send-webhook":
		return s.executeSendOutboundWebhook(ctx, node, msg)

	// Storage / State nodes
	case "sql-query":
		return s.executeSQLQuery(ctx, node, msg)

	// Monitoring nodes
	case "assert":
		return s.executeAssert(node, msg)
	case "metric-record":
		return s.executeMetricRecord(ctx, node, msg, flowCtx)
	case "health-check":
		return s.executeHealthCheck(ctx, node, msg)

	// External service nodes
	case "ssh-exec":
		return s.executeSSHExecRuntime(ctx, node, msg)
	case "ftp-upload":
		return s.executeFTPUploadRuntime(ctx, node, msg)
	case "dns-lookup":
		return s.executeDNSLookup(ctx, node, msg)
	case "ping":
		return s.executePing(ctx, node, msg)

	case "link-out":
		name, _ := node.Config["name"].(string)
		wfID, _ := flowCtx.FlowVars["_workflowId"].(string)
		if wfID != "" {
			if err := s.StoreLinkMessage(ctx, wfID, node.ID, name, msg); err != nil {
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
			stored, err := s.FetchLinkMessage(ctx, parts[0], parts[1])
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
		// Plugin-contributed node: the SPA writes `plugin:<slug>:<key>`
		// as the action; the loaded plugin owns the executor.
		if strings.HasPrefix(node.Action, "plugin:") {
			return s.executePluginNode(ctx, node, msg, flowCtx, nodeEnvID)
		}
		// A bare action a loaded plugin claims (a node the host once shipped):
		// run it as that plugin's node, on a copy so the canvas keeps its id.
		if resolved := s.resolveAction(node.Action); resolved != node.Action {
			aliased := *node
			aliased.Action = resolved
			return s.executePluginNode(ctx, &aliased, msg, flowCtx, nodeEnvID)
		}

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

	cli, err := s.pool.GetContext(ctx, envID)
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
		infoRes, inspectErr := cli.ContainerInspect(inspectCtx, containerName, dockerclient.ContainerInspectOptions{})
		info := infoRes.Container
		if inspectErr == nil {
			payload["currentState"] = string(info.State.Status)
			payload["running"] = info.State.Running
			payload["startedAt"] = info.State.StartedAt
			payload["finishedAt"] = info.State.FinishedAt
			payload["exitCode"] = info.State.ExitCode
			payload["image"] = info.Config.Image
			if info.State.Health != nil {
				payload["health"] = string(info.State.Health.Status)
			}
		} else {
			payload["inspectError"] = inspectErr.Error()
		}

		filterArgs := make(dockerclient.Filters)
		filterArgs.Add("type", "container")
		if containerName != "" {
			filterArgs.Add("container", containerName)
		}
		if statusEvent != "any" {
			filterArgs.Add("event", statusEvent)
		}

		eventCtx, eventCancel := context.WithTimeout(ctx, 15*time.Second)
		defer eventCancel()
		eventStream := cli.Events(eventCtx, dockerclient.EventsListOptions{Filters: filterArgs})
		eventsCh, errCh := eventStream.Messages, eventStream.Err

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

func (s *Service) executeContainerHealthTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, err := cli.ContainerInspect(opCtx, containerName, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("inspecting container health: %w", err)
	}
	info := infoRes.Container

	health := "none"
	if info.State != nil && info.State.Health != nil {
		health = string(info.State.Health.Status)
	}
	want, _ := node.Config["health"].(string)
	if want == "" {
		want = "any"
	}

	out := NewMsg(map[string]interface{}{
		"trigger":   "container-health",
		"container": containerName,
		"health":    health,
		"matched":   want == "any" || want == health,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	out["topic"] = "container-health"
	return "output", out, nil
}

func (s *Service) executeDockerEventTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	filterArgs := make(dockerclient.Filters)
	if eventType, _ := node.Config["type"].(string); eventType != "" && eventType != "any" {
		filterArgs.Add("type", eventType)
	}
	for _, action := range configStringSlice(node.Config["action"]) {
		filterArgs.Add("event", action)
	}
	if label, _ := node.Config["label"].(string); strings.TrimSpace(label) != "" {
		filterArgs.Add("label", strings.TrimSpace(label))
	}

	eventCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	eventStream := cli.Events(eventCtx, dockerclient.EventsListOptions{Filters: filterArgs})
	eventsCh, errCh := eventStream.Messages, eventStream.Err
	for {
		select {
		case evt := <-eventsCh:
			if !matchesDockerEventNode(node, evt) {
				continue
			}
			return "output", dockerEventMsg(evt, false), nil
		case err := <-errCh:
			if err != nil && !dockerEventStreamClosed(eventCtx, err) {
				return "", nil, fmt.Errorf("watching docker events: %w", err)
			}
			return "output", dockerEventMsg(events.Message{}, false), nil
		case <-eventCtx.Done():
			return "output", dockerEventMsg(events.Message{}, false), nil
		}
	}
}

func (s *Service) executeRegistryTagTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	digest, payload, err := s.inspectRegistryTagDigest(ctx, node, envID)
	if err != nil {
		return "", nil, err
	}
	payload["digest"] = digest
	payload["changed"] = false
	out := NewMsg(payload)
	out["topic"] = "registry-tag"
	return "output", out, nil
}

func (s *Service) executeApproval(node *CanvasNode, msg Msg) (string, Msg, error) {
	decision, _ := node.Config["decision"].(string)
	if decision == "" {
		decision = "pending"
	}
	if decision != "approved" && decision != "rejected" {
		decision = "pending"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	payload := map[string]interface{}{
		"decision": decision,
	}
	if reason, _ := node.Config["reason"].(string); reason != "" {
		payload["reason"] = reason
	}
	out["payload"] = payload
	return decision, out, nil
}

// executeMetricTrigger handles the metric-trigger node action.
func (s *Service) executeMetricTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container name or ID is required")
	}

	cli, err := s.pool.GetContext(ctx, envID)
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

	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "error", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ref, guardErr := s.guardSelfContainer(ctx, envID, containerName, false)
	if guardErr != nil {
		return "", nil, guardErr
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	// Check current state before acting
	if action != "restart" && action != "remove" {
		infoRes, inspectErr := cli.ContainerInspect(opCtx, ref, dockerclient.ContainerInspectOptions{})
		info := infoRes.Container
		if inspectErr == nil {
			alreadyRunning := info.State.Running
			if action == "start" && alreadyRunning {
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{
					"action":    action,
					"container": containerName,
					"error":     "Container is already running",
					"state":     string(info.State.Status),
				}
				return "error", out, nil
			}
			if action == "stop" && !alreadyRunning {
				out := CloneMsg(msg)
				out = EnsureMsgID(out)
				out["payload"] = map[string]interface{}{
					"action":    action,
					"container": containerName,
					"error":     fmt.Sprintf("Container is already in state %s", string(info.State.Status)),
					"state":     string(info.State.Status),
				}
				return "error", out, nil
			}
		}
	}

	var opErr error
	switch action {
	case "start":
		_, opErr = cli.ContainerStart(opCtx, ref, dockerclient.ContainerStartOptions{})
	case "stop", "restart":
		stopOpts := dockerclient.ContainerStopOptions{}
		if t := int(configFloat(node.Config, "timeout", 0)); t > 0 {
			stopOpts.Timeout = &t
		}
		if action == "stop" {
			_, opErr = cli.ContainerStop(opCtx, ref, stopOpts)
		} else {
			_, opErr = cli.ContainerRestart(opCtx, ref, dockerclient.ContainerRestartOptions(stopOpts))
		}
	case "remove":
		_, opErr = cli.ContainerRemove(opCtx, ref, dockerclient.ContainerRemoveOptions{Force: true})
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
		infoRes, inspectErr := cli.ContainerInspect(opCtx, ref, dockerclient.ContainerInspectOptions{})
		info := infoRes.Container
		if inspectErr == nil {
			result["state"] = string(info.State.Status)
			result["running"] = info.State.Running
		}
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

	resp, err := cli.ContainerStats(statsCtx, containerID, dockerclient.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
	if err != nil {
		return nil, err
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

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

func configBool(config map[string]interface{}, key string, fallback bool) bool {
	v, ok := config[key]
	if !ok {
		return fallback
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func configStringSlice(value interface{}) []string {
	switch raw := value.(type) {
	case []string:
		return cleanWorkflowStringSlice(raw)
	case []interface{}:
		items := make([]string, 0, len(raw))
		for _, item := range raw {
			items = append(items, fmt.Sprintf("%v", item))
		}
		return cleanWorkflowStringSlice(items)
	case string:
		text := strings.TrimSpace(raw)
		if text == "" {
			return []string{}
		}
		var parsed []string
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return cleanWorkflowStringSlice(parsed)
		}
		return cleanWorkflowStringSlice(strings.Split(text, ","))
	default:
		return []string{}
	}
}

func cleanWorkflowStringSlice(input []string) []string {
	out := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

// executeDockerInfo retrieves Docker system information.
func (s *Service) executeDockerInfo(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	infoCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, err := cli.Info(infoCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
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

func (s *Service) executeEnvironmentStatus(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"reachable": false, "error": "docker connection failed"}
		return "error", out, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	ping, err := cli.Ping(opCtx, dockerclient.PingOptions{})
	if err != nil {
		s.logger.Error("workflows: environment ping failed", "error", err, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"reachable": false, "error": "environment ping failed"}
		return "error", out, nil
	}

	infoRes, infoErr := cli.Info(opCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
	payload := map[string]interface{}{
		"reachable":      true,
		"environmentId":  envID,
		"apiVersion":     ping.APIVersion,
		"osType":         ping.OSType,
		"experimental":   ping.Experimental,
		"builderVersion": ping.BuilderVersion,
	}
	if infoErr == nil {
		payload["serverVersion"] = info.ServerVersion
		payload["name"] = info.Name
		payload["operatingSystem"] = info.OperatingSystem
		payload["containers"] = info.Containers
		payload["images"] = info.Images
		payload["cpus"] = info.NCPU
		payload["memoryTotal"] = info.MemTotal
	} else {
		s.logger.Warn("workflows: environment info failed after ping", "error", infoErr, "envID", envID)
		payload["infoError"] = "environment info failed"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = payload
	return "output", out, nil
}

// executeDockerPrune removes unused Docker resources.
func (s *Service) executeDockerPrune(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
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
		reportRes, pruneErr := cli.ContainerPrune(opCtx, dockerclient.ContainerPruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: container prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "container prune failed"}
			return "error", out, nil
		}
		report := reportRes.Report
		result["containersDeleted"] = report.ContainersDeleted
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "images":
		reportRes, pruneErr := cli.ImagePrune(opCtx, dockerclient.ImagePruneOptions{Filters: unusedImagePruneFilters()})
		if pruneErr != nil {
			s.logger.Error("workflows: image prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "image prune failed"}
			return "error", out, nil
		}
		report := reportRes.Report
		result["imagesDeleted"] = len(report.ImagesDeleted)
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "volumes":
		reportRes, pruneErr := cli.VolumePrune(opCtx, dockerclient.VolumePruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: volume prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "volume prune failed"}
			return "error", out, nil
		}
		report := reportRes.Report
		result["volumesDeleted"] = report.VolumesDeleted
		result["spaceReclaimed"] = report.SpaceReclaimed

	case "networks":
		reportRes, pruneErr := cli.NetworkPrune(opCtx, dockerclient.NetworkPruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: network prune failed", "error", pruneErr, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "network prune failed"}
			return "error", out, nil
		}
		report := reportRes.Report
		result["networksDeleted"] = report.NetworksDeleted

	case "all":
		var totalReclaimed uint64
		crRes, pruneErr := cli.ContainerPrune(opCtx, dockerclient.ContainerPruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: containers prune failed during full prune", "error", pruneErr, "envID", envID)
		}
		cr := crRes.Report
		result["containersDeleted"] = cr.ContainersDeleted
		totalReclaimed += cr.SpaceReclaimed

		irRes, pruneErr := cli.ImagePrune(opCtx, dockerclient.ImagePruneOptions{Filters: unusedImagePruneFilters()})
		if pruneErr != nil {
			s.logger.Error("workflows: images prune failed during full prune", "error", pruneErr, "envID", envID)
		}
		ir := irRes.Report
		result["imagesDeleted"] = len(ir.ImagesDeleted)
		totalReclaimed += ir.SpaceReclaimed

		vrRes, pruneErr := cli.VolumePrune(opCtx, dockerclient.VolumePruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: volumes prune failed during full prune", "error", pruneErr, "envID", envID)
		}
		vr := vrRes.Report
		result["volumesDeleted"] = vr.VolumesDeleted
		totalReclaimed += vr.SpaceReclaimed

		nrRes, pruneErr := cli.NetworkPrune(opCtx, dockerclient.NetworkPruneOptions{})
		if pruneErr != nil {
			s.logger.Error("workflows: networks prune failed during full prune", "error", pruneErr, "envID", envID)
		}
		nr := nrRes.Report
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image name is required")
	}

	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	registryAuth, registryName, err := s.registryAuthForNode(pullCtx, node)
	if err != nil {
		return "", nil, err
	}

	reader, err := cli.ImagePull(pullCtx, imageName, dockerclient.ImagePullOptions{RegistryAuth: registryAuth})
	if err != nil {
		s.logger.Error("workflows: image pull failed", "error", err, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image pull failed", "image": imageName}
		return "error", out, nil
	}
	defer func() { closeutil.Report(reader.Close(), "reader") }()

	// Drain the pull output to completion
	if _, copyErr := io.Copy(io.Discard, reader); copyErr != nil {
		s.logger.Warn("workflows: image pull stream drain failed", "error", copyErr, "image", imageName, "envID", envID)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"image":    imageName,
		"registry": registryName,
		"status":   "pulled",
	}
	return "output", out, nil
}

func (s *Service) executeImageVulnerabilityScan(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	imageRef, _ := node.Config["image"].(string)
	if strings.TrimSpace(imageRef) == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	// The node's `scanner` is passed through untouched — empty
	// included. Resolution moved into the scans service when the
	// engines became plugins: it accepts a scanner_configs id (what the
	// node's scanner picker writes), an engine type (what nodes saved
	// before the picker carry), or empty for the estate default, and it
	// is the only layer that can tell those apart.
	//
	// The enabled-check that used to live here is gone with it. It read
	// three booleans out of the settings table and hard-coded the three
	// built-in engine names, so a fourth scanner would have been
	// rejected by a node that had no way to know about it. A disabled
	// or missing scanner now surfaces as an error from StartScan below,
	// on the same error output, with a message that names the real
	// cause.
	scannerName, _ := node.Config["scanner"].(string)

	if s.imageScanner == nil {
		s.logger.Error("workflows: image vulnerability scanner is not configured", "image", imageRef, "scanner", scannerName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"image": imageRef, "scanner": scannerName, "error": "scanner is unavailable"}
		return "error", out, nil
	}

	input := ImageScanInput{ImageRef: imageRef, Scanner: scannerName, EnvironmentID: envID}

	wait := false
	if v, ok := node.Config["wait"].(bool); ok {
		wait = v
	}

	scanCtx := ctx
	cancel := func() {}
	if wait {
		// ScanTimeout is one of the three scanner settings that stayed
		// in core: it bounds how long the *workflow run* blocks, which
		// is the host's concern and must not vary per engine.
		timeout := time.Duration(coresettings.ReadScannerSettingsContext(ctx, s.db).ScanTimeout) * time.Second
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}
		scanCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	var scan *ImageScanResult
	var err error
	if wait {
		scan, err = s.imageScanner.StartScanSync(scanCtx, input)
	} else {
		scan, err = s.imageScanner.StartScan(scanCtx, input)
	}
	if err != nil {
		s.logger.Error("workflows: image vulnerability scan failed", "error", err, "image", imageRef, "scanner", scannerName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"image": imageRef, "scanner": scannerName, "error": "scan failed"}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":            scan.ID,
		"image":         scan.ImageRef,
		"scanner":       scan.Scanner,
		"status":        scan.Status,
		"severity":      scan.Severity,
		"totalVulns":    scan.TotalVulns,
		"criticalCount": scan.CriticalCount,
		"highCount":     scan.HighCount,
		"mediumCount":   scan.MediumCount,
		"lowCount":      scan.LowCount,
	}

	if !wait || scan.Status != "completed" {
		return "output", out, nil
	}
	switch scan.Severity {
	case "critical", "high", "medium", "low":
		return scan.Severity, out, nil
	default:
		return "clean", out, nil
	}
}

// executeHostInfo returns system information from the Docker host via Docker info.
func (s *Service) executeHostInfo(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	infoCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, err := cli.Info(infoCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
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
	cli, err := s.pool.GetContext(ctx, envID)
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
	hostConfig := hostExecHostConfig()
	if guardErr := s.guardSelfCreate(ctx, envID, hostConfig); guardErr != nil {
		return "", nil, guardErr
	}
	resp, err := cli.ContainerCreate(execCtx, dockerclient.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:3.20",
		Cmd:   []string{"sh", "-c", command},
	}, HostConfig: hostConfig})
	if err != nil {
		s.logger.Error("workflows: host exec create failed", "error", err, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to create exec container", "command": command}
		return "error", out, nil
	}

	if _, startErr := cli.ContainerStart(execCtx, resp.ID, dockerclient.ContainerStartOptions{}); startErr != nil {
		s.logger.Error("workflows: host exec start failed", "error", startErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "failed to start exec container", "command": command}
		return "error", out, nil
	}

	// Wait for completion
	wait := cli.ContainerWait(execCtx, resp.ID, dockerclient.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	statusCh, errCh := wait.Result, wait.Error
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
	logReader, logErr := cli.ContainerLogs(execCtx, resp.ID, dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	var output string
	if logErr == nil {
		defer func() { closeutil.Report(logReader.Close(), "logReader") }()
		logBytes, readErr := io.ReadAll(logReader)
		if readErr != nil {
			s.logger.Warn("workflows: reading container logs", "error", readErr)
		}
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
			if errors.Is(err, io.EOF) {
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	showAll := true
	if v, ok := node.Config["show_all"].(bool); ok {
		showAll = v
	}
	nameFilter, _ := node.Config["name_filter"].(string)

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	opts := dockerclient.ContainerListOptions{All: showAll}
	if nameFilter != "" {
		f := make(dockerclient.Filters)
		f.Add("name", nameFilter)
		opts.Filters = f
	}

	containersRes, listErr := cli.ContainerList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: container list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container list failed"}
		return "error", out, nil
	}
	containers := containersRes.Items

	items := make([]interface{}, 0, len(containers))
	for _, c := range containers {
		items = append(items, map[string]interface{}{
			"id":      c.ID[:12],
			"names":   c.Names,
			"image":   c.Image,
			"state":   string(c.State),
			"status":  c.Status,
			"created": c.Created,
			"ports":   wirePorts(c.Ports),
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = items
	return "output", out, nil
}

// executeContainerInspect returns detailed information about a container.
func (s *Service) executeContainerInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	if containerName == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, inspectErr := cli.ContainerInspect(opCtx, containerName, dockerclient.ContainerInspectOptions{})
	if inspectErr != nil {
		s.logger.Error("workflows: container inspect failed", "error", inspectErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container inspect failed", "container": containerName}
		return "error", out, nil
	}
	info := infoRes.Container
	// A run's output is stored and shown to anyone who can read the workflow, which is not
	// the audience a container's credentials are for; there is no reveal path here.
	envredact.Policy{McHarbor: docker.IsMcHarborInstance(info)}.Container(&info)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"id":         info.ID[:12],
		"name":       strings.TrimPrefix(info.Name, "/"),
		"image":      info.Config.Image,
		"state":      string(info.State.Status),
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
	cli, err := s.pool.GetContext(ctx, envID)
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

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	opts := dockerclient.ContainerLogsOptions{
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
	defer func() { closeutil.Report(reader.Close(), "reader") }()

	logBytes, readErr := io.ReadAll(reader)
	if readErr != nil {
		s.logger.Warn("workflows: reading container logs", "container", containerName, "error", readErr)
	}

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
	cli, err := s.pool.GetContext(ctx, envID)
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

	ref, guardErr := s.guardSelfContainer(ctx, envID, containerName, true)
	if guardErr != nil {
		return "", nil, guardErr
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	execConfig := dockerclient.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
	}
	if workdir != "" {
		execConfig.WorkingDir = workdir
	}

	execResp, execErr := cli.ExecCreate(opCtx, ref, execConfig)
	if execErr != nil {
		s.logger.Error("workflows: container exec create failed", "error", execErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "exec create failed", "container": containerName}
		return "error", out, nil
	}

	attachResp, attachErr := cli.ExecAttach(opCtx, execResp.ID, dockerclient.ExecAttachOptions{})
	if attachErr != nil {
		s.logger.Error("workflows: container exec attach failed", "error", attachErr, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "exec attach failed", "container": containerName}
		return "error", out, nil
	}
	defer attachResp.Close()

	outputBytes, readErr := io.ReadAll(attachResp.Reader)
	if readErr != nil {
		s.logger.Warn("workflows: reading exec output", "container", containerName, "error", readErr)
	}

	// Get exit code
	inspectResp, inspectErr := cli.ExecInspect(opCtx, execResp.ID, dockerclient.ExecInspectOptions{})
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}
	containerName, _ := node.Config["name"].(string)
	portsStr := strings.Join(configStringSlice(node.Config["ports"]), ",")
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

	opCtx, cancel := context.WithTimeout(ctx, nodeLongOpTimeout)
	defer cancel()

	config := &container.Config{
		Image: imageName,
		Env:   envVars,
	}

	hostConfig := containerCreateHostConfig(node)
	if guardErr := s.guardSelfCreate(ctx, envID, hostConfig); guardErr != nil {
		return "", nil, guardErr
	}

	resp, createErr := cli.ContainerCreate(opCtx, dockerclient.ContainerCreateOptions{Config: config, HostConfig: hostConfig, Name: containerName})
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
		if _, startErr := cli.ContainerStart(opCtx, resp.ID, dockerclient.ContainerStartOptions{}); startErr != nil {
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

func (s *Service) executeContainerBackupRun(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	if s.backupRunner == nil {
		return "", nil, ErrContainerBackupsUnavailable
	}
	containerID, _ := node.Config["container"].(string)
	if strings.TrimSpace(containerID) == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	storageLocationID, _ := node.Config["storage_location_id"].(string)
	input := ContainerBackupInput{
		Name:              "Workflow container backup",
		StorageLocationID: storageLocationID,
		IncludeConfig:     true,
		IncludeLogs:       configBool(node.Config, "include_logs", false),
		IncludeFilesystem: configBool(node.Config, "include_filesystem", false),
		IncludeImage:      configBool(node.Config, "include_image", false),
		SelectedMounts:    configStringSlice(node.Config["selected_mounts"]),
	}

	opCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()

	run, err := s.backupRunner.RunAdhoc(opCtx, envID, containerID, input)
	if err != nil {
		s.logger.Error("workflows: container backup failed", "error", err, "container", containerID, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container backup failed", "container": containerID}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = run
	return "output", out, nil
}

func (s *Service) executeContainerBackupPlanRun(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.backupRunner == nil {
		return "", nil, ErrContainerBackupsUnavailable
	}
	planID, _ := node.Config["plan_id"].(string)
	if strings.TrimSpace(planID) == "" {
		return "", nil, fmt.Errorf("plan_id is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()

	run, err := s.backupRunner.RunPlan(opCtx, planID)
	if err != nil {
		s.logger.Error("workflows: container backup plan failed", "error", err, "plan", planID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container backup plan failed", "planId": planID}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = run
	return "output", out, nil
}

func (s *Service) executeContainerBackupDownload(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.backupRunner == nil {
		return "", nil, ErrContainerBackupsUnavailable
	}
	runID, _ := node.Config["run_id"].(string)
	if strings.TrimSpace(runID) == "" {
		return "", nil, fmt.Errorf("run_id is required")
	}

	download, err := s.backupRunner.Download(ctx, runID)
	if err != nil {
		s.logger.Error("workflows: container backup download info failed", "error", err, "run", runID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container backup download info failed", "runId": runID}
		return "error", out, nil
	}
	if download == nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container backup not downloadable", "runId": runID}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"runId":       download.RunID,
		"path":        download.Path,
		"fileName":    download.FileName,
		"contentType": download.ContentType,
		"size":        download.Size,
		"modTime":     download.ModTime,
	}
	return "output", out, nil
}

func (s *Service) executeContainerStackLink(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	if s.stackLinker == nil {
		return "", nil, ErrStackLinkerUnavailable
	}
	containerID, _ := node.Config["container"].(string)
	stackName, _ := node.Config["stack_name"].(string)
	serviceName, _ := node.Config["service_name"].(string)
	if strings.TrimSpace(containerID) == "" || strings.TrimSpace(stackName) == "" {
		return "", nil, fmt.Errorf("container and stack_name are required")
	}

	link, err := s.stackLinker.LinkContainer(ctx, envID, LinkContainerRequest{
		ContainerID: containerID,
		StackName:   stackName,
		ServiceName: serviceName,
	})
	if err != nil {
		s.logger.Error("workflows: container stack link failed", "error", err, "container", containerID, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container stack link failed", "container": containerID, "stack": stackName}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = link
	return "output", out, nil
}

func (s *Service) executeContainerStackUnlink(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	if s.stackLinker == nil {
		return "", nil, ErrStackLinkerUnavailable
	}
	containerID, _ := node.Config["container"].(string)
	if strings.TrimSpace(containerID) == "" {
		return "", nil, fmt.Errorf("container is required")
	}

	if err := s.stackLinker.UnlinkContainer(envID, containerID); err != nil {
		s.logger.Error("workflows: container stack unlink failed", "error", err, "container", containerID, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "container stack unlink failed", "container": containerID}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"container": containerID, "status": "unlinked"}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Stack node implementations
// ---------------------------------------------------------------------------

// executeStackList lists compose stacks by grouping containers by their compose project label.
func (s *Service) executeStackList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project")
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack list failed"}
		return "error", out, nil
	}
	containers := containersRes.Items

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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+stackName)
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack status failed", "error", listErr, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack status failed", "stack": stackName}
		return "error", out, nil
	}
	containers := containersRes.Items

	services := make([]interface{}, 0, len(containers))
	running := 0
	for _, c := range containers {
		svcName := c.Labels["com.docker.compose.service"]
		services = append(services, map[string]interface{}{
			"id":      c.ID[:12],
			"service": svcName,
			"image":   c.Image,
			"state":   string(c.State),
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	if guardErr := s.guardSelfStack(ctx, envID, stackName); guardErr != nil {
		return "", nil, guardErr
	}
	removeVolumes := false
	if v, ok := node.Config["remove_volumes"].(bool); ok {
		removeVolumes = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+stackName)
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack remove list failed", "error", listErr, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack remove failed", "stack": stackName}
		return "error", out, nil
	}
	containers := containersRes.Items

	removed := 0
	for _, c := range containers {
		_, rmErr := cli.ContainerRemove(opCtx, c.ID, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: removeVolumes})
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

func (s *Service) executeStackBackup(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+stackName)
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: stack backup list failed", "error", listErr, "stack", stackName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack backup failed", "stack": stackName}
		return "error", out, nil
	}
	containers := containersRes.Items

	items := make([]interface{}, 0, len(containers))
	for _, c := range containers {
		items = append(items, map[string]interface{}{
			"id":          c.ID[:12],
			"names":       c.Names,
			"image":       c.Image,
			"imageID":     c.ImageID,
			"command":     c.Command,
			"state":       string(c.State),
			"status":      c.Status,
			"created":     c.Created,
			"labels":      c.Labels,
			"ports":       wirePorts(c.Ports),
			"mounts":      c.Mounts,
			"networkMode": c.HostConfig.NetworkMode,
		})
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"kind":        "mcharbor.stack-backup",
		"version":     1,
		"stack":       stackName,
		"environment": envID,
		"exportedAt":  time.Now().UTC().Format(time.RFC3339),
		"containers":  items,
	}
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Image node implementations
// ---------------------------------------------------------------------------

// executeImageList lists Docker images.
func (s *Service) executeImageList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
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

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	opts := dockerclient.ImageListOptions{All: showAll}
	if filterDangling {
		f := make(dockerclient.Filters)
		f.Add("dangling", "true")
		opts.Filters = f
	}

	imagesRes, listErr := cli.ImageList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: image list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image list failed"}
		return "error", out, nil
	}
	images := imagesRes.Items

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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	info, inspectErr := cli.ImageInspect(opCtx, imageName)
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
		"env":          imageEnvForRun(info.Config.Env),
		"cmd":          info.Config.Cmd,
		"entrypoint":   info.Config.Entrypoint,
		"labels":       info.Config.Labels,
		"exposedPorts": info.Config.ExposedPorts,
	}
	return "output", out, nil
}

// imageEnvForRun withholds credential-looking values from an image's env: a run's output is
// shown to anyone who can read the workflow, and there is no reveal path here.
func imageEnvForRun(env []string) []string {
	masked, _ := envredact.Policy{}.EnvList(env)
	return masked
}

// executeImageRemove removes a Docker image.
func (s *Service) executeImageRemove(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
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

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	deletedRes, rmErr := cli.ImageRemove(opCtx, imageName, dockerclient.ImageRemoveOptions{Force: force, PruneChildren: pruneChildren})
	if rmErr != nil {
		s.logger.Error("workflows: image remove failed", "error", rmErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image remove failed", "image": imageName}
		return "error", out, nil
	}
	deleted := deletedRes.Items

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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	source, _ := node.Config["source"].(string)
	target, _ := node.Config["target"].(string)
	if source == "" || target == "" {
		return "", nil, fmt.Errorf("source and target are required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, tagErr := cli.ImageTag(opCtx, dockerclient.ImageTagOptions{Source: source, Target: target}); tagErr != nil {
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	filterDangling := false
	if v, ok := node.Config["filter_dangling"].(bool); ok {
		filterDangling = v
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	opts := dockerclient.VolumeListOptions{}
	if filterDangling {
		f := make(dockerclient.Filters)
		f.Add("dangling", "true")
		opts.Filters = f
	}

	respRes, listErr := cli.VolumeList(opCtx, opts)
	if listErr != nil {
		s.logger.Error("workflows: volume list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume list failed"}
		return "error", out, nil
	}
	resp := respRes.Items

	items := make([]interface{}, 0, len(resp))
	for _, vol := range resp {
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
	cli, err := s.pool.GetContext(ctx, envID)
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

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	volRes, createErr := cli.VolumeCreate(opCtx, dockerclient.VolumeCreateOptions{
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
	vol := volRes.Volume

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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name := nodeConfigString(node, "volume", nodeConfigString(node, "name", ""))
	if name == "" {
		return "", nil, fmt.Errorf("volume name is required")
	}
	if guardErr := s.guardSelfVolume(ctx, envID, name); guardErr != nil {
		return "", nil, guardErr
	}
	force := false
	if v, ok := node.Config["force"].(bool); ok {
		force = v
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, rmErr := cli.VolumeRemove(opCtx, name, dockerclient.VolumeRemoveOptions{Force: force}); rmErr != nil {
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name := nodeConfigString(node, "volume", nodeConfigString(node, "name", ""))
	if name == "" {
		return "", nil, fmt.Errorf("volume name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	volRes, inspectErr := cli.VolumeInspect(opCtx, name, dockerclient.VolumeInspectOptions{})
	if inspectErr != nil {
		s.logger.Error("workflows: volume inspect failed", "error", inspectErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "volume inspect failed", "name": name}
		return "error", out, nil
	}
	vol := volRes.Volume

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

func (s *Service) executeVolumeBackup(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	volumeName, _ := node.Config["volume"].(string)
	backupPath, _ := node.Config["backup_path"].(string)
	if strings.TrimSpace(volumeName) == "" {
		return "", nil, fmt.Errorf("volume is required")
	}
	if strings.TrimSpace(backupPath) == "" {
		return "", nil, fmt.Errorf("backup path is required")
	}

	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	helperImage := workflowHelperImage(node)
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := s.runVolumeArchiveHelper(opCtx, cli, helperImage, volumeName, backupPath, false); err != nil {
		s.logger.Error("workflows: volume backup failed", "error", err, "volume", volumeName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"volume": volumeName, "backupPath": backupPath, "error": "volume backup failed"}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"volume":      volumeName,
		"backupPath":  backupPath,
		"helperImage": helperImage,
		"status":      "backed_up",
	}
	return "output", out, nil
}

func (s *Service) executeVolumeRestore(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	volumeName, _ := node.Config["volume"].(string)
	backupPath, _ := node.Config["backup_path"].(string)
	if strings.TrimSpace(volumeName) == "" {
		return "", nil, fmt.Errorf("volume is required")
	}
	if strings.TrimSpace(backupPath) == "" {
		return "", nil, fmt.Errorf("backup path is required")
	}
	if guardErr := s.guardSelfVolume(ctx, envID, volumeName); guardErr != nil {
		return "", nil, guardErr
	}

	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	overwrite := false
	if v, ok := node.Config["overwrite"].(bool); ok {
		overwrite = v
	}
	helperImage := workflowHelperImage(node)
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := s.runVolumeRestoreHelper(opCtx, cli, helperImage, volumeName, backupPath, overwrite); err != nil {
		s.logger.Error("workflows: volume restore failed", "error", err, "volume", volumeName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"volume": volumeName, "backupPath": backupPath, "error": "volume restore failed"}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"volume":      volumeName,
		"backupPath":  backupPath,
		"helperImage": helperImage,
		"overwrite":   overwrite,
		"status":      "restored",
	}
	return "output", out, nil
}

func (s *Service) executeStorageLocationList(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.storageReader == nil {
		return "", nil, ErrStorageLocationsUnavailable
	}
	items, err := s.storageReader.List(ctx)
	if err != nil {
		s.logger.Error("workflows: storage location list failed", "error", err)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "storage location list failed"}
		return "error", out, nil
	}

	if configBool(node.Config, "enabled_only", true) {
		filtered := make([]StorageLocationSummary, 0, len(items))
		for _, item := range items {
			if item.Enabled {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"items": items, "count": len(items)}
	return "output", out, nil
}

func (s *Service) executeStorageLocationGet(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if s.storageReader == nil {
		return "", nil, ErrStorageLocationsUnavailable
	}
	locationID, _ := node.Config["storage_location_id"].(string)
	if strings.TrimSpace(locationID) == "" {
		return "", nil, fmt.Errorf("storage_location_id is required")
	}

	item, err := s.storageReader.ByID(ctx, locationID)
	if err != nil {
		s.logger.Error("workflows: storage location get failed", "error", err, "location", locationID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "storage location get failed", "storageLocationId": locationID}
		return "error", out, nil
	}
	if item == nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "storage location not found", "storageLocationId": locationID}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = item
	return "output", out, nil
}

// ---------------------------------------------------------------------------
// Network node implementations
// ---------------------------------------------------------------------------

// executeNetworkList lists Docker networks.
func (s *Service) executeNetworkList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	networksRes, listErr := cli.NetworkList(opCtx, dockerclient.NetworkListOptions{})
	if listErr != nil {
		s.logger.Error("workflows: network list failed", "error", listErr, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network list failed"}
		return "error", out, nil
	}
	networks := networksRes.Items

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
	cli, err := s.pool.GetContext(ctx, envID)
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

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	opts := dockerclient.NetworkCreateOptions{
		Driver:   driver,
		Internal: internal,
	}
	if subnet != "" {
		prefix, parseErr := netip.ParsePrefix(subnet)
		if parseErr != nil {
			s.logger.Error("workflows: network create failed", "error", parseErr, "name", name, "envID", envID)
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "network create failed", "name": name}
			return "error", out, nil
		}
		opts.IPAM = &network.IPAM{
			Config: []network.IPAMConfig{{Subnet: prefix}},
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name := nodeConfigString(node, "network", nodeConfigString(node, "name", ""))
	if name == "" {
		return "", nil, fmt.Errorf("network name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, rmErr := cli.NetworkRemove(opCtx, name, dockerclient.NetworkRemoveOptions{}); rmErr != nil {
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	name := nodeConfigString(node, "network", nodeConfigString(node, "name", ""))
	if name == "" {
		return "", nil, fmt.Errorf("network name is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, inspectErr := cli.NetworkInspect(opCtx, name, dockerclient.NetworkInspectOptions{})
	if inspectErr != nil {
		s.logger.Error("workflows: network inspect failed", "error", inspectErr, "name", name, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "network inspect failed", "name": name}
		return "error", out, nil
	}
	info := infoRes.Network

	containers := make(map[string]interface{}, len(info.Containers))
	for id, ep := range info.Containers {
		containers[id[:12]] = map[string]interface{}{
			"name":        ep.Name,
			"ipv4Address": wireText(ep.IPv4Address),
			"ipv6Address": wireText(ep.IPv6Address),
			"macAddress":  wireText(ep.MacAddress),
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
	cli, err := s.pool.GetContext(ctx, envID)
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

	ref, guardErr := s.guardSelfContainer(ctx, envID, containerName, false)
	if guardErr != nil {
		return "", nil, guardErr
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	var opErr error
	if action == "disconnect" {
		_, opErr = cli.NetworkDisconnect(opCtx, networkName, dockerclient.NetworkDisconnectOptions{Container: ref})
	} else {
		_, opErr = cli.NetworkConnect(opCtx, networkName, dockerclient.NetworkConnectOptions{Container: ref})
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

// executeConditionNode routes a message on the node's own condition first, then on
// each extra condition in order, matching the port set the canvas draws.
//
// Without extras the node is a two-port branch: "true" / "false". With extras the
// canvas draws "true", "condition_0".."condition_N-1", "else"
// (getEffectiveOutputPorts in src/frontend/modules/workflows/types.ts), and the
// first match wins — so an extra condition is only reached when the primary one
// and every earlier extra have failed.
func (s *Service) executeConditionNode(node *CanvasNode, msg Msg) (string, Msg, error) {
	time.Sleep(100 * time.Millisecond)

	field, _ := node.Config["field"].(string)
	operator, _ := node.Config["operator"].(string)
	value, _ := node.Config["value"].(string)
	primary := evaluateCondition(field, operator, value, msg)

	extras := parseExtraConditions(node.Config["extra_conditions"])

	// Pass msg through unchanged in every branch — a condition node decides
	// routing and never rewrites the message.
	if len(extras) == 0 {
		if primary {
			return "true", msg, nil
		}
		return "false", msg, nil
	}

	if primary {
		return "true", msg, nil
	}
	for i, extra := range extras {
		// A malformed entry holds its index and never matches. Evaluating it would
		// match everything, because evaluateCondition treats an empty field as true.
		if extra.Invalid {
			continue
		}
		if evaluateCondition(extra.Field, extra.Operator, extra.Value, msg) {
			return fmt.Sprintf("condition_%d", i), msg, nil
		}
	}
	return "else", msg, nil
}

// extraCondition is one additional branch on a condition node, matching the
// frontend's ExtraCondition type. Label is carried for parity and is not read
// here: the canvas shows it, the engine routes by index.
type extraCondition struct {
	Field    string
	Operator string
	Value    string
	// Invalid marks an entry that could not be read. It keeps its index so the
	// canvas's edges stay aligned, and is never evaluated.
	Invalid bool
}

// parseExtraConditions reads node.Config["extra_conditions"], which arrives as
// []interface{} of map[string]interface{} from the stored canvas JSON.
//
// A malformed entry is kept rather than skipped, because dropping it would shift
// every later condition onto the wrong output port — the edges the canvas drew
// are bound to the index, so the count has to survive intact.
func parseExtraConditions(raw interface{}) []extraCondition {
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([]extraCondition, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, extraCondition{Invalid: true})
			continue
		}
		field, _ := entry["field"].(string)
		operator, _ := entry["operator"].(string)
		value, _ := entry["value"].(string)
		out = append(out, extraCondition{Field: field, Operator: operator, Value: value})
	}
	return out
}

// evaluateCondition checks a simple condition against the msg using dot-notation paths.
// Fields like "payload.status" resolve to msg["payload"]["status"].
//
// An unknown operator returns true, so the operator lists the UI offers must stay
// within this set or a condition silently always passes. PREDICATE_OPERATORS in
// src/frontend/modules/workflows/node-predicates.ts is that set, and
// node-predicates.contract.test.ts parses this switch to keep the two in step.
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
	case "starts_with":
		return strings.HasPrefix(fieldValue, value)
	case "is_empty":
		return fieldValue == ""
	case "is_not_empty":
		return fieldValue != ""
	case ">", "<", ">=", "<=":
		return compareOrdered(fieldValue, operator, value)
	case "regex":
		if re, err := regexp.Compile(value); err == nil {
			return re.MatchString(fieldValue)
		}
		return false
	default:
		return true
	}
}

// compareOrdered evaluates the four ordering operators, numerically when both
// sides parse as numbers and lexicographically otherwise.
//
// The string fallback matters because a msg path resolves through
// fmt.Sprintf("%v", …): a timestamp or a semver-ish string is the common case for
// an ordering test, and refusing it would make the operator useless there.
func compareOrdered(fieldValue, operator, value string) bool {
	left, leftErr := strconv.ParseFloat(strings.TrimSpace(fieldValue), 64)
	right, rightErr := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if leftErr == nil && rightErr == nil {
		switch operator {
		case ">":
			return left > right
		case "<":
			return left < right
		case ">=":
			return left >= right
		case "<=":
			return left <= right
		}
		return false
	}
	switch operator {
	case ">":
		return fieldValue > value
	case "<":
		return fieldValue < value
	case ">=":
		return fieldValue >= value
	case "<=":
		return fieldValue <= value
	}
	return false
}

// ---------------------------------------------------------------------------
// Trigger node implementations
// ---------------------------------------------------------------------------

func (s *Service) executeFileWatchTrigger(ctx context.Context, node *CanvasNode) (string, Msg, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	watchPath, _ := node.Config["path"].(string)
	eventTypes := configStringSlice(node.Config["event_types"])
	storageLocationID, _ := node.Config["storage_location_id"].(string)
	now := time.Now().UTC()
	out := NewMsg(map[string]interface{}{
		"trigger":         "file-watch",
		"path":            watchPath,
		"eventTypes":      eventTypes,
		"storageLocation": s.fileWatchStorageLocationPayload(ctx, storageLocationID),
		"timestamp":       now.Format(time.RFC3339),
	})
	out["topic"] = "file-watch"
	return "output", out, nil
}

func (s *Service) fileWatchStorageLocationPayload(ctx context.Context, id string) map[string]interface{} {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if s.storageReader == nil {
		return nil
	}
	location, err := s.storageReader.ByID(ctx, id)
	if err != nil || location == nil {
		return nil
	}
	return map[string]interface{}{
		"id":           location.ID,
		"name":         location.Name,
		"locationType": location.LocationType,
		"basePath":     location.BasePath,
	}
}

// ---------------------------------------------------------------------------
// Logic / Flow node implementations
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Data transformation node implementations
// ---------------------------------------------------------------------------

var msgTemplatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// renderMsgTemplate replaces {{path}} placeholders with the value found at
// that path in msg. Unknown paths are left as-is so the user can see them.
func renderMsgTemplate(tmpl string, msg Msg) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}
	return msgTemplatePattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if v, ok := GetPath(msg, key); ok {
			return fmt.Sprintf("%v", v)
		}
		return match
	})
}

func (s *Service) executeTemplate(node *CanvasNode, msg Msg) (string, Msg, error) {
	tmpl, _ := node.Config["template"].(string)
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}
	if tmpl == "" {
		return "error", msg, nil
	}

	result := renderMsgTemplate(tmpl, msg)

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
	fields := configStringSlice(node.Config["fields"])

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
	fields := configStringSlice(node.Config["fields"])
	omitSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		omitSet[f] = true
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
	sources := configStringSlice(node.Config["sources"])
	outputProp, _ := node.Config["output_property"].(string)
	if outputProp == "" {
		outputProp = "payload"
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	merged := make(map[string]interface{})
	for _, src := range sources {
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
			s.logger.Warn("workflows: base64 decode failed", "error", err)
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
	outputFormat = resolveDateFormatPreset(outputFormat)
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
		return "error", out, nil //nolint:nilerr // an unparseable date is reported on the error port, not as a hard node failure
	}

	loc, locErr := time.LoadLocation(tz)
	if locErr != nil {
		loc = time.UTC
	}
	if outputFormat == dateFormatUnix {
		SetPath(out, property, strconv.FormatInt(parsed.In(loc).Unix(), 10))
		return "output", out, nil
	}
	SetPath(out, property, parsed.In(loc).Format(outputFormat))
	return "output", out, nil
}

// dateFormatUnix is the sentinel the "unix" preset resolves to; it is not
// a Go layout, so executeDateFormat branches on it before calling Format.
const dateFormatUnix = "\x00unix"

// resolveDateFormatPreset maps the preset keys the date-format node offers
// in its picker to Go layouts. Anything else is treated as a literal
// layout, which keeps workflows saved with a hand-typed format working.
func resolveDateFormatPreset(value string) string {
	switch strings.TrimSpace(value) {
	case "", "rfc3339":
		return time.RFC3339
	case "rfc1123":
		return time.RFC1123
	case "date":
		return "2006-01-02"
	case "datetime":
		return "2006-01-02 15:04:05"
	case "time":
		return "15:04:05"
	case "date_eu":
		return "02-01-2006"
	case "date_us":
		return "01/02/2006"
	case "unix":
		return dateFormatUnix
	default:
		return value
	}
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
			return "error", out, nil //nolint:nilerr // a non-numeric input is reported on the error port, not as a hard node failure
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
		start := atoiOrZero(arg1)
		end := atoiOrZero(arg2)
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
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("encoding graphql request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
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
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		s.logger.Warn("workflows: reading response body", "error", readErr)
	}
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

// configuredEmailRequest addresses the mail to the node's own recipients when it names any;
// an empty to leaves Recipient empty, so the channel's saved recipients are the default.
func configuredEmailRequest(channelID string, emailTypes []string, to, subject, body string) corenotify.ChannelRequest {
	return corenotify.ChannelRequest{
		ChannelID:    channelID,
		ChannelTypes: emailTypes,
		Title:        subject,
		Message:      body,
		Recipient:    to,
	}
}

func (s *Service) executeSendConfiguredEmail(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if deliveryMode, _ := node.Config["delivery_mode"].(string); deliveryMode == "custom" {
		return s.executeSendCustomEmail(ctx, node, msg)
	}

	if s.notifier == nil {
		return "", nil, fmt.Errorf("notification dispatcher is not configured")
	}

	to := strings.Join(configStringSlice(node.Config["to"]), ",")
	subject, _ := node.Config["subject"].(string)
	body, _ := node.Config["body"].(string)
	channelID, _ := node.Config["channel_id"].(string)
	if channelID == "" {
		channelID, _ = node.Config["server_id"].(string)
	}
	// An empty "To" is not an error here: it leaves Recipient empty, so the dispatcher
	// delivers to the recipients saved on the channel, the same default reports use.
	if strings.TrimSpace(body) == "" {
		return "", nil, fmt.Errorf("body is required")
	}

	// With no server picked the default email channel is wanted, and only a plugin that
	// declares its channel type an email transport can supply one; with none loaded the
	// node fails rather than handing the mail to whichever channel is the default.
	var emailTypes []string
	if channelID == "" {
		emailTypes = s.emailChannelTypes()
		if len(emailTypes) == 0 {
			return "", nil, fmt.Errorf("no installed communication plugin can send email")
		}
	}

	delivery, err := s.notifier.SendChannel(ctx, configuredEmailRequest(channelID, emailTypes, to, subject, body))
	if err != nil {
		s.logger.Error("workflows: configured email send failed", "error", err, "channelID", channelID)
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
		"sent":        true,
		"to":          to,
		"subject":     subject,
		"channelId":   delivery.TargetID,
		"channelName": delivery.TargetName,
	}
	return "output", out, nil
}

func (s *Service) executeSendCustomEmail(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	to := strings.Join(configStringSlice(node.Config["to"]), ",")
	subject, _ := node.Config["subject"].(string)
	body, _ := node.Config["body"].(string)
	host, _ := node.Config["smtp_host"].(string)
	encryptionMode, _ := node.Config["smtp_encryption"].(string)
	authMethod, _ := node.Config["smtp_auth_method"].(string)
	username, _ := node.Config["smtp_username"].(string)
	password, _ := node.Config["smtp_password"].(string)
	fromAddress, _ := node.Config["from_address"].(string)
	fromName, _ := node.Config["from_name"].(string)
	port := int(configFloat(node.Config, "smtp_port", 587))

	if strings.TrimSpace(to) == "" {
		return "", nil, fmt.Errorf("recipient is required")
	}
	if strings.TrimSpace(body) == "" {
		return "", nil, fmt.Errorf("body is required")
	}
	if strings.TrimSpace(host) == "" {
		return "", nil, fmt.Errorf("smtp host is required")
	}
	if port <= 0 {
		return "", nil, fmt.Errorf("smtp port is required")
	}
	if strings.TrimSpace(fromAddress) == "" {
		return "", nil, fmt.Errorf("from address is required")
	}
	if encryptionMode == "" {
		encryptionMode = "starttls"
	}
	if authMethod == "" {
		authMethod = "plain"
	}

	err := coreemail.SendSMTP(ctx, coreemail.SMTPConfig{
		Host:        host,
		Port:        port,
		Encryption:  encryptionMode,
		AuthMethod:  authMethod,
		Username:    username,
		Password:    password,
		FromAddress: fromAddress,
		FromName:    fromName,
	}, to, subject, body)
	if err != nil {
		s.logger.Error("workflows: custom email send failed", "error", err, "host", host)
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
		"sent":    true,
		"to":      to,
		"subject": subject,
		"server":  host,
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

func (s *Service) executeSendOutboundWebhook(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	if deliveryMode, _ := node.Config["delivery_mode"].(string); deliveryMode == "configured" {
		return s.executeSendConfiguredWebhook(ctx, node, msg)
	}

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
	bodyBytes, err := json.Marshal(bodyData)
	if err != nil {
		return "", nil, fmt.Errorf("encoding webhook body: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
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
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		s.logger.Warn("workflows: reading response body", "error", readErr)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"statusCode": resp.StatusCode,
		"body":       string(respBody),
	}
	return "output", out, nil
}

type configuredWebhook struct {
	ID     string
	Name   string
	URL    string
	Secret string
}

func (s *Service) executeSendConfiguredWebhook(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	webhookID, _ := node.Config["webhook_id"].(string)
	if strings.TrimSpace(webhookID) == "" {
		return "", nil, fmt.Errorf("webhook is required")
	}

	wh, err := s.resolveConfiguredWebhook(ctx, webhookID)
	if err != nil {
		return "", nil, err
	}
	if wh == nil {
		return "", nil, fmt.Errorf("webhook not found")
	}

	bodySource, _ := node.Config["body_source"].(string)
	if bodySource == "" {
		bodySource = "payload"
	}

	var bodyData interface{}
	if raw, ok := GetPath(msg, bodySource); ok {
		bodyData = raw
	}
	bodyBytes, err := json.Marshal(bodyData)
	if err != nil {
		return "", nil, fmt.Errorf("encoding webhook body: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, wh.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(bodyBytes)
		req.Header.Set("X-McHarbor-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	duration := int(time.Since(start).Milliseconds())
	if err != nil {
		s.logger.Error("workflows: configured webhook failed", "error", err, "webhookID", wh.ID)
		if recErr := s.recordWebhookDelivery(ctx, wh.ID, "workflow", string(bodyBytes), 0, "request failed", false, duration); recErr != nil {
			s.logger.Warn("workflows: recording webhook delivery failed", "error", recErr, "webhookID", wh.ID)
		}
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "webhook request failed", "webhookId": wh.ID}
		return "error", out, nil
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		s.logger.Warn("workflows: reading response body", "error", readErr)
	}
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if recErr := s.recordWebhookDelivery(ctx, wh.ID, "workflow", string(bodyBytes), resp.StatusCode, string(respBody), success, duration); recErr != nil {
		s.logger.Warn("workflows: recording webhook delivery failed", "error", recErr, "webhookID", wh.ID)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"statusCode":  resp.StatusCode,
		"body":        string(respBody),
		"webhookId":   wh.ID,
		"webhookName": wh.Name,
	}
	return "output", out, nil
}

func (s *Service) resolveConfiguredWebhook(ctx context.Context, id string) (*configuredWebhook, error) {
	var wh configuredWebhook
	var secret sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, secret
		 FROM webhooks
		 WHERE id = ? AND is_active = 1
		 LIMIT 1`, id,
	).Scan(&wh.ID, &wh.Name, &wh.URL, &secret)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up configured webhook: %w", err)
	}
	if secret.String != "" {
		if s.enc == nil {
			return nil, fmt.Errorf("encryption service is not configured")
		}
		plain, decErr := s.enc.Decrypt(secret.String)
		if decErr != nil {
			return nil, fmt.Errorf("decrypting webhook secret: %w", decErr)
		}
		wh.Secret = plain
	}
	return &wh, nil
}

func (s *Service) recordWebhookDelivery(ctx context.Context, webhookID, event, payload string, respStatus int, respBody string, success bool, duration int) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO webhook_deliveries (id, webhook_id, event, payload, response_status, response_body, success, duration, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, webhookID, event, payload, respStatus, respBody, success, duration, now,
	)
	if err != nil {
		return fmt.Errorf("recording webhook delivery: %w", err)
	}
	return nil
}

// executeSendCommunication is the merged "Send Message" node. Delivery
// always goes through a channel configured under Settings → Communication.
// The node itself has no channel_type field; it is only present on nodes
// rewritten from the legacy send-<provider> keys, where it selects the
// default channel of that provider when channel_id is empty. Recipient,
// title and message accept {{path}} placeholders resolved against msg.
func (s *Service) executeSendCommunication(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	channelType, _ := node.Config["channel_type"].(string)
	if channelType == "any" {
		channelType = ""
	}
	return s.executeSendConfiguredCommunication(ctx, node, msg, channelType)
}

func (s *Service) executeSendConfiguredCommunication(ctx context.Context, node *CanvasNode, msg Msg, channelType string) (string, Msg, error) {
	if s.notifier == nil {
		return "", nil, fmt.Errorf("notification dispatcher is not configured")
	}

	channelID, _ := node.Config["channel_id"].(string)
	rawRecipient, _ := node.Config["recipient"].(string)
	rawTitle, _ := node.Config["title"].(string)
	rawMessage, _ := node.Config["message"].(string)
	if strings.TrimSpace(rawMessage) == "" {
		return "", nil, fmt.Errorf("message is required")
	}
	recipient := strings.TrimSpace(renderMsgTemplate(rawRecipient, msg))
	title := renderMsgTemplate(rawTitle, msg)
	message := renderMsgTemplate(rawMessage, msg)

	delivery, err := s.notifier.SendChannel(ctx, corenotify.ChannelRequest{
		ChannelID:   channelID,
		ChannelType: channelType,
		Title:       title,
		Message:     message,
		Recipient:   recipient,
	})
	if err != nil {
		s.logger.Error("workflows: configured communication send failed", "error", err, "channelID", channelID, "channelType", channelType)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"error":       "communication send failed",
			"channelId":   channelID,
			"channelType": channelType,
			"recipient":   recipient,
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
		"recipient":   recipient,
		"title":       title,
		"message":     message,
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

	dataDir := apppaths.DataDir()
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

	dataDir := apppaths.DataDir()
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
			s.logger.Warn("workflows: base64 decode failed", "error", err)
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
			closeutil.Report(f.Close(), "f")
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

func (s *Service) executeKVGet(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
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
	err := s.db.QueryRowContext(ctx, "SELECT value FROM workflow_kv WHERE key = ? AND (expires_at IS NULL OR expires_at > datetime('now')) LIMIT 1", key).Scan(&value)
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

func (s *Service) executeKVSet(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
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
	valueBytes, err := json.Marshal(raw)
	if err != nil {
		return "", nil, fmt.Errorf("encoding kv value: %w", err)
	}
	valueStr := string(valueBytes)

	var expiresAt *string
	if ttl > 0 {
		t := time.Now().UTC().Add(time.Duration(ttl) * time.Second).Format("2006-01-02 15:04:05")
		expiresAt = &t
	}

	_, err = s.db.ExecContext(ctx,
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

func (s *Service) executeKVDelete(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	key, _ := node.Config["key"].(string)
	if key == "" {
		return "", nil, fmt.Errorf("key is required")
	}

	_, err := s.db.ExecContext(ctx, "DELETE FROM workflow_kv WHERE key = ?", key)
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
	if validationErr != nil {
		return "error", workflowErrorMsg(msg, validationErr.Error(), nil), nil
	}

	rows, cleanup, err := openReadOnlyWorkflowSQLRows(ctx, s.db, s.logger, normalizedQuery)
	if err != nil {
		if isWorkflowSQLRefusal(err) {
			// The wrapped detail names opcodes and root pages; the workflow sees only the
			// sentinel's own text, the log keeps the rest.
			s.logger.Warn("workflows: sql query refused", "error", err)
			return "error", workflowErrorMsg(msg, workflowSQLRefusalMessage(err), nil), nil
		}
		s.logger.Error("workflows: sql query failed", "error", err)
		return "error", workflowErrorMsg(msg, "sql query failed", nil), nil
	}
	defer cleanup()
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	cols, colErr := rows.Columns()
	if colErr != nil {
		s.logger.Error("workflows: sql query columns failed", "error", colErr)
		return "error", workflowErrorMsg(msg, "sql query failed", nil), nil
	}
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

func (s *Service) executeMetricRecord(ctx context.Context, node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
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

	recordedID, recordedAt, numericValue, err := s.recordWorkflowMetric(ctx, workflowID, node.ID, metricName, metricType, property, unit, raw, labels)
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

func (s *Service) recordWorkflowMetric(ctx context.Context, workflowID, nodeID, metricName, metricType, sourceProperty, unit string, raw interface{}, labels map[string]interface{}) (string, string, interface{}, error) {
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
		valueJSON, err = json.Marshal(fmt.Sprintf("%v", raw))
		if err != nil {
			return "", "", nil, fmt.Errorf("marshalling metric value: %w", err)
		}
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

	if _, err := s.db.ExecContext(ctx,
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
		closeutil.Report(resp.Body.Close(), "resp.Body")
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

func (s *Service) executeDNSLookup(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
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
		ips, lookupErr := net.DefaultResolver.LookupHost(ctx, hostname)
		err = lookupErr
		records = ips
	case "MX":
		mxs, lookupErr := net.DefaultResolver.LookupMX(ctx, hostname)
		err = lookupErr
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%s (priority %d)", mx.Host, mx.Pref))
		}
	case "TXT":
		txts, lookupErr := net.DefaultResolver.LookupTXT(ctx, hostname)
		err = lookupErr
		records = txts
	case "CNAME":
		cname, lookupErr := net.DefaultResolver.LookupCNAME(ctx, hostname)
		err = lookupErr
		records = []string{cname}
	case "NS":
		nss, lookupErr := net.DefaultResolver.LookupNS(ctx, hostname)
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

func (s *Service) executePing(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	host, _ := node.Config["host"].(string)
	timeout := configFloat(node.Config, "timeout", 5)

	if host == "" {
		return "", nil, fmt.Errorf("host is required")
	}

	// TCP ping as a portable alternative to ICMP (no raw socket needed)
	start := time.Now()
	dialer := &net.Dialer{Timeout: time.Duration(timeout) * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", host+":80")
	elapsed := time.Since(start)
	if err != nil {
		// Try port 443 as fallback
		start = time.Now()
		conn, err = dialer.DialContext(ctx, "tcp", host+":443")
		elapsed = time.Since(start)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	if err != nil {
		s.logger.Warn("workflows: ping failed", "host", host, "error", err)
		out["payload"] = map[string]interface{}{
			"host":      host,
			"reachable": false,
			"error":     "host unreachable",
		}
		return "error", out, nil
	}
	closeutil.Report(conn.Close(), "conn")

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
	cli, err := s.pool.GetContext(ctx, envID)
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	containerName, _ := node.Config["container"].(string)
	newName, _ := node.Config["new_name"].(string)
	if containerName == "" || newName == "" {
		return "", nil, fmt.Errorf("container and new_name are required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	ref, guardErr := s.guardSelfContainer(ctx, envID, containerName, false)
	if guardErr != nil {
		return "", nil, guardErr
	}

	unlockProtected := configBool(node.Config, "unlock_protected", false)
	if err := ensureWorkflowContainerMutable(opCtx, cli, ref, unlockProtected); err != nil {
		s.logger.Error("workflows: container rename blocked", "error", err, "container", containerName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "rename blocked", "container": containerName}
		return "error", out, nil
	}

	if _, renameErr := cli.ContainerRename(opCtx, ref, dockerclient.ContainerRenameOptions{NewName: newName}); renameErr != nil {
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

func ensureWorkflowContainerMutable(ctx context.Context, cli *dockerclient.Client, id string, unlockProtected bool) error {
	if !unlockProtected {
		return docker.EnsureContainerMutable(ctx, cli, id)
	}

	infoRes, err := cli.ContainerInspect(ctx, id, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	info := infoRes.Container

	imageName := ""
	labels := map[string]string{}
	if info.Config != nil {
		imageName = info.Config.Image
		labels = info.Config.Labels
	}
	if docker.IsMcHarborContainer([]string{info.Name}, imageName, labels) {
		return nil
	}
	if docker.IsProtectedContainer([]string{info.Name}, imageName, labels) {
		return docker.ErrProtectedResource
	}
	return nil
}

func (s *Service) executeContainerWait(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
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

	waitResult := cli.ContainerWait(opCtx, containerName, dockerclient.ContainerWaitOptions{Condition: waitCond})
	statusCh, errCh := waitResult.Result, waitResult.Error
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

func (s *Service) executeImagePush(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageName, _ := node.Config["image"].(string)
	if imageName == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	pushCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	registryAuth, registryName, err := s.registryAuthForNode(pushCtx, node)
	if err != nil {
		return "", nil, err
	}

	reader, pushErr := cli.ImagePush(pushCtx, imageName, dockerclient.ImagePushOptions{RegistryAuth: registryAuth})
	if pushErr != nil {
		s.logger.Error("workflows: image push failed", "error", pushErr, "image", imageName, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image push failed", "image": imageName}
		return "error", out, nil
	}
	defer func() { closeutil.Report(reader.Close(), "reader") }()
	if _, copyErr := io.Copy(io.Discard, reader); copyErr != nil {
		s.logger.Warn("workflows: image push stream drain failed", "error", copyErr, "image", imageName, "envID", envID)
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{"image": imageName, "registry": registryName, "status": "pushed"}
	return "output", out, nil
}

func (s *Service) executeRegistrySearch(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	term, _ := node.Config["term"].(string)
	if term == "" {
		return "", nil, fmt.Errorf("search term is required")
	}
	limit := int(configFloat(node.Config, "limit", 25))

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	registryAuth, registryName, err := s.registryAuthForNode(opCtx, node)
	if err != nil {
		return "", nil, err
	}

	resultsRes, searchErr := cli.ImageSearch(opCtx, term, dockerclient.ImageSearchOptions{Limit: limit, RegistryAuth: registryAuth})
	if searchErr != nil {
		s.logger.Error("workflows: registry search failed", "error", searchErr, "term", term, "envID", envID)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "registry search failed", "term": term}
		return "error", out, nil
	}
	results := resultsRes.Items

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
	if registryName != "" {
		out["registry"] = registryName
	}
	return "output", out, nil
}

type configuredRegistry struct {
	ID       string
	Name     string
	URL      string
	Username string
	Password string
}

func (s *Service) registryAuthForNode(ctx context.Context, node *CanvasNode) (string, string, error) {
	mode, _ := node.Config["registry_mode"].(string)
	if mode == "configured" {
		registryID, _ := node.Config["registry_id"].(string)
		return s.configuredRegistryAuth(ctx, registryID)
	}

	regURL, _ := node.Config["registry_url"].(string)
	username, _ := node.Config["registry_username"].(string)
	password, _ := node.Config["registry_password"].(string)
	if strings.TrimSpace(regURL) == "" && strings.TrimSpace(username) == "" && strings.TrimSpace(password) == "" {
		return "", "", nil
	}

	auth, err := authconfig.Encode(registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: strings.TrimRight(regURL, "/"),
	})
	if err != nil {
		return "", "", fmt.Errorf("encoding registry auth: %w", err)
	}
	return auth, strings.TrimRight(regURL, "/"), nil
}

func (s *Service) configuredRegistryAuth(ctx context.Context, registryID string) (string, string, error) {
	reg, err := s.resolveConfiguredRegistry(ctx, registryID)
	if err != nil {
		return "", "", err
	}
	if reg == nil {
		return "", "", fmt.Errorf("registry not found")
	}

	auth, err := authconfig.Encode(registry.AuthConfig{
		Username:      reg.Username,
		Password:      reg.Password,
		ServerAddress: strings.TrimRight(reg.URL, "/"),
	})
	if err != nil {
		return "", "", fmt.Errorf("encoding registry auth: %w", err)
	}
	return auth, reg.Name, nil
}

func (s *Service) resolveConfiguredRegistry(ctx context.Context, registryID string) (*configuredRegistry, error) {
	var row *sql.Row
	if strings.TrimSpace(registryID) != "" {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, name, url, username, password
			 FROM registries
			 WHERE id = ?
			 LIMIT 1`, registryID,
		)
	} else {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, name, url, username, password
			 FROM registries
			 WHERE is_default = 1
			 ORDER BY name ASC
			 LIMIT 1`,
		)
	}

	var reg configuredRegistry
	var username, password sql.NullString
	err := row.Scan(&reg.ID, &reg.Name, &reg.URL, &username, &password)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up configured registry: %w", err)
	}

	reg.Username = username.String
	if password.String != "" {
		if s.enc == nil {
			return nil, fmt.Errorf("encryption service is not configured")
		}
		plain, decErr := s.enc.Decrypt(password.String)
		if decErr != nil {
			return nil, fmt.Errorf("decrypting registry password: %w", decErr)
		}
		reg.Password = plain
	}

	return &reg, nil
}

func (s *Service) inspectRegistryTagDigest(ctx context.Context, node *CanvasNode, envID string) (string, map[string]interface{}, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	imageRef, _ := node.Config["image"].(string)
	if strings.TrimSpace(imageRef) == "" {
		return "", nil, fmt.Errorf("image is required")
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	registryAuth, registryName, err := s.registryAuthForNode(opCtx, node)
	if err != nil {
		return "", nil, err
	}

	info, err := cli.DistributionInspect(opCtx, imageRef, dockerclient.DistributionInspectOptions{EncodedRegistryAuth: registryAuth})
	if err != nil {
		return "", nil, fmt.Errorf("inspecting registry tag: %w", err)
	}

	payload := map[string]interface{}{
		"trigger":     "registry-tag",
		"image":       imageRef,
		"registry":    registryName,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"platforms":   info.Platforms,
		"descriptor":  info.Descriptor,
		"environment": envID,
	}
	return info.Descriptor.Digest.String(), payload, nil
}

func workflowHelperImage(node *CanvasNode) string {
	helperImage, _ := node.Config["helper_image"].(string)
	if strings.TrimSpace(helperImage) == "" {
		return "alpine:3.20"
	}
	return strings.TrimSpace(helperImage)
}

func (s *Service) runVolumeArchiveHelper(ctx context.Context, cli *dockerclient.Client, helperImage, volumeName, backupPath string, restore bool) error {
	backupDir, backupFile, err := splitBackupPath(backupPath)
	if err != nil {
		return err
	}

	if err := pullHelperImage(ctx, cli, helperImage); err != nil {
		s.logger.Warn("workflows: helper image pull failed, trying local image", "error", err, "image", helperImage)
	}

	cmd := []string{"sh", "-c", `mkdir -p /backup && tar -czf "/backup/$BACKUP_FILE" -C /source .`}
	readonly := true
	if restore {
		cmd = []string{"sh", "-c", `tar -xzf "/backup/$BACKUP_FILE" -C /target`}
		readonly = false
	}

	resp, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Config: &container.Config{
		Image: helperImage,
		Cmd:   cmd,
		Env:   []string{"BACKUP_FILE=" + backupFile},
	}, HostConfig: &container.HostConfig{
		AutoRemove: false,
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Source: volumeName, Target: "/source", ReadOnly: readonly},
			{Type: mount.TypeBind, Source: backupDir, Target: "/backup"},
		},
	}, Name: "mcharbor-workflow-volume-backup-" + xid.New().String()})
	if err != nil {
		return fmt.Errorf("creating helper container: %w", err)
	}
	// Detached on purpose: the helper container has to be removed even when
	// ctx is what cancelled the run. Bounded so cleanup cannot hang.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		defer cancel()
		removeHelperContainer(cleanupCtx, cli, resp.ID, s.logger)
	}()

	return runHelperContainer(ctx, cli, resp.ID)
}

func (s *Service) runVolumeRestoreHelper(ctx context.Context, cli *dockerclient.Client, helperImage, volumeName, backupPath string, overwrite bool) error {
	backupDir, backupFile, err := splitBackupPath(backupPath)
	if err != nil {
		return err
	}

	if err := pullHelperImage(ctx, cli, helperImage); err != nil {
		s.logger.Warn("workflows: helper image pull failed, trying local image", "error", err, "image", helperImage)
	}

	overwriteValue := "false"
	if overwrite {
		overwriteValue = "true"
	}

	resp, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Config: &container.Config{
		Image: helperImage,
		Cmd:   []string{"sh", "-c", `if [ "$OVERWRITE" != "true" ] && [ "$(ls -A /target 2>/dev/null)" ]; then exit 3; fi; tar -xzf "/backup/$BACKUP_FILE" -C /target`},
		Env:   []string{"BACKUP_FILE=" + backupFile, "OVERWRITE=" + overwriteValue},
	}, HostConfig: &container.HostConfig{
		AutoRemove: false,
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Source: volumeName, Target: "/target"},
			{Type: mount.TypeBind, Source: backupDir, Target: "/backup", ReadOnly: true},
		},
	}, Name: "mcharbor-workflow-volume-restore-" + xid.New().String()})
	if err != nil {
		return fmt.Errorf("creating helper container: %w", err)
	}
	// Detached on purpose: the helper container has to be removed even when
	// ctx is what cancelled the run. Bounded so cleanup cannot hang.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		defer cancel()
		removeHelperContainer(cleanupCtx, cli, resp.ID, s.logger)
	}()

	return runHelperContainer(ctx, cli, resp.ID)
}

func splitBackupPath(path string) (string, string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	dir := filepath.Dir(cleaned)
	file := filepath.Base(cleaned)
	if dir == "." || dir == "" || file == "." || file == string(filepath.Separator) {
		return "", "", fmt.Errorf("backup path must include a directory and file name")
	}
	return dir, file, nil
}

func pullHelperImage(ctx context.Context, cli *dockerclient.Client, imageName string) error {
	reader, err := cli.ImagePull(ctx, imageName, dockerclient.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer func() { closeutil.Report(reader.Close(), "reader") }()
	_, err = io.Copy(io.Discard, reader)
	return err
}

func runHelperContainer(ctx context.Context, cli *dockerclient.Client, id string) error {
	if _, err := cli.ContainerStart(ctx, id, dockerclient.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting helper container: %w", err)
	}
	waitResult := cli.ContainerWait(ctx, id, dockerclient.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	statusCh, errCh := waitResult.Result, waitResult.Error
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("waiting for helper container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("helper container exited with status %d", status.StatusCode)
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func removeHelperContainer(ctx context.Context, cli *dockerclient.Client, id string, logger *slog.Logger) {
	rmCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	if _, err := cli.ContainerRemove(rmCtx, id, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		logger.Warn("workflows: helper container cleanup failed", "error", err, "container", id)
	}
}

func dockerEventMsg(evt events.Message, autoTriggered bool) Msg {
	if evt.Actor.ID == "" {
		out := NewMsg(map[string]interface{}{
			"trigger":       "docker-event",
			"matched":       false,
			"autoTriggered": autoTriggered,
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
		})
		out["topic"] = "docker-event"
		return out
	}

	action := strings.TrimSpace(string(evt.Action))
	normalized := action
	if idx := strings.Index(normalized, ":"); idx != -1 {
		normalized = strings.TrimSpace(normalized[:idx])
	}

	out := NewMsg(map[string]interface{}{
		"trigger":          "docker-event",
		"type":             string(evt.Type),
		"action":           action,
		"normalizedAction": normalized,
		"id":               evt.Actor.ID,
		"actorId":          evt.Actor.ID,
		"actorAttributes":  evt.Actor.Attributes,
		"scope":            string(evt.Scope),
		"time":             evt.Time,
		"timeNano":         evt.TimeNano,
		"matched":          true,
		"autoTriggered":    autoTriggered,
	})
	out["topic"] = "docker-event"
	return out
}

func matchesDockerEventNode(node *CanvasNode, evt events.Message) bool {
	eventType, _ := node.Config["type"].(string)
	if eventType != "" && eventType != "any" && eventType != string(evt.Type) {
		return false
	}

	if wanted := configStringSlice(node.Config["action"]); len(wanted) > 0 {
		action := strings.TrimSpace(string(evt.Action))
		normalized := action
		if idx := strings.Index(normalized, ":"); idx != -1 {
			normalized = strings.TrimSpace(normalized[:idx])
		}
		matched := false
		for _, cfgAction := range wanted {
			if cfgAction == action || cfgAction == normalized {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	name, _ := node.Config["name"].(string)
	name = strings.TrimSpace(name)
	if name != "" && name != evt.Actor.ID && name != evt.Actor.Attributes["name"] {
		return false
	}

	labelFilter, _ := node.Config["label"].(string)
	labelFilter = strings.TrimSpace(labelFilter)
	if labelFilter == "" {
		return true
	}
	if strings.Contains(labelFilter, "=") {
		parts := strings.SplitN(labelFilter, "=", 2)
		return evt.Actor.Attributes[parts[0]] == parts[1]
	}
	_, ok := evt.Actor.Attributes[labelFilter]
	return ok
}

func containerHealthFromEvent(evt events.Message) string {
	action := strings.TrimSpace(string(evt.Action))
	if !strings.HasPrefix(action, "health_status") {
		return ""
	}
	parts := strings.SplitN(action, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (s *Service) executeComposeUp(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	if guardErr := s.guardSelfStack(ctx, envID, stackName); guardErr != nil {
		return "", nil, guardErr
	}

	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	// List containers in this stack to report status
	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+stackName)
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	containers := containersRes.Items
	if listErr != nil {
		s.logger.Warn("workflows: listing stack containers", "stack", stackName, "error", listErr)
	}

	// Start any stopped containers in the stack
	started := 0
	for _, c := range containers {
		if c.State != "running" {
			if _, startErr := cli.ContainerStart(opCtx, c.ID, dockerclient.ContainerStartOptions{}); startErr == nil {
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
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	stackName, _ := node.Config["stack_name"].(string)
	if stackName == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	if guardErr := s.guardSelfStack(ctx, envID, stackName); guardErr != nil {
		return "", nil, guardErr
	}
	removeVolumes := false
	if v, ok := node.Config["remove_volumes"].(bool); ok {
		removeVolumes = v
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+stackName)
	containersRes, listErr := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if listErr != nil {
		s.logger.Error("workflows: compose down list failed", "error", listErr, "stack", stackName)
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "compose down failed", "stack": stackName}
		return "error", out, nil
	}
	containers := containersRes.Items

	stopped := 0
	removed := 0
	for _, c := range containers {
		if c.State == "running" {
			if _, stopErr := cli.ContainerStop(opCtx, c.ID, dockerclient.ContainerStopOptions{}); stopErr == nil {
				stopped++
			}
		}
		if _, rmErr := cli.ContainerRemove(opCtx, c.ID, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: removeVolumes}); rmErr == nil {
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

// atoiOrZero parses a workflow config integer and treats anything unparseable as zero, which every caller already clamps.
func atoiOrZero(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
