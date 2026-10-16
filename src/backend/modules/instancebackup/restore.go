// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/xid"

	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// UploadDir holds artifacts uploaded for a restore, under ExportDir.
const UploadDir = "uploads"

const statusFile = "restore-status.json"

// Restore states, as the status endpoint reports them.
const (
	RestoreRunning     = "running"
	RestoreRestarting  = "restarting"
	RestoreReconciling = "reconciling"
	RestoreDone        = "done"
	RestoreFailed      = "failed"
)

// ErrRestoreRunning is returned while a restore is in progress.
var ErrRestoreRunning = errors.New("a restore is already running")

// PluginOutcome is what reinstalling one plugin after a restore did.
type PluginOutcome struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

// PluginReconciler makes the installed plugins match an export's list, through
// the same install path the Plugin Store uses.
type PluginReconciler interface {
	ReconcileInstalled(ctx context.Context, want []core.PluginRecord) []PluginOutcome
}

// RestoreHooks are the pieces of the running server a restore needs.
type RestoreHooks struct {
	Restart func(ctx context.Context) error
	Plugins PluginReconciler
	// Emit records the intent in the live audit chain and pushes it to the
	// external sinks before the chain is replaced.
	Emit         func(ctx context.Context, in core.Intent) error
	KeyPinned    bool
	SecretPinned bool
}

// RestoreStatus is persisted in the data directory, because a restore ends in
// a restart and its outcome must still be readable afterwards.
type RestoreStatus struct {
	ID         string          `json:"id"`
	State      string          `json:"state"`
	Actor      string          `json:"actor"`
	Mode       string          `json:"mode"`
	StartedAt  string          `json:"startedAt"`
	FinishedAt string          `json:"finishedAt,omitempty"`
	Error      string          `json:"error,omitempty"`
	Refusals   []string        `json:"refusals,omitempty"`
	Result     *core.Result    `json:"result,omitempty"`
	Plugins    []PluginOutcome `json:"plugins,omitempty"`
	Progress   *core.Progress  `json:"progress,omitempty"`
}

// RestoreSource names the artifact: one of this instance's exports, or an upload.
type RestoreSource struct {
	ExportID string `json:"exportId,omitempty"`
	UploadID string `json:"uploadId,omitempty"`
}

// RestoreInput is a plan or restore request.
type RestoreInput struct {
	Source       RestoreSource `json:"source"`
	Passphrase   string        `json:"passphrase,omitempty"`
	KeyFile      string        `json:"keyFile,omitempty"`
	Mode         string        `json:"mode"`
	Categories   []string      `json:"categories,omitempty"`
	IncludeAudit bool          `json:"includeAudit"`
	Conflict     string        `json:"conflict,omitempty"`
}

// UploadResult is a stored upload and its unauthenticated manifest.
type UploadResult struct {
	UploadID string        `json:"uploadId"`
	Manifest core.Manifest `json:"manifest"`
}

// SetRestoreHooks connects the restore to the server; without it restores are refused.
func (s *Service) SetRestoreHooks(h RestoreHooks) { s.hooks = h }

func (s *Service) target() core.Target {
	return core.Target{DB: s.db, Driver: s.driver, DataDir: s.dataDir, EncryptionKey: s.enc.KeyMaterial(),
		Secret: s.secret, KeyPinned: s.hooks.KeyPinned, SecretPinned: s.hooks.SecretPinned}
}

// SaveUpload stores an uploaded artifact and reads its plaintext manifest.
func (s *Service) SaveUpload(ctx context.Context, body io.Reader) (*UploadResult, error) {
	dir := filepath.Join(s.dataDir, ExportDir, UploadDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("preparing the upload directory: %w", err)
	}
	id := xid.New().String()
	p := filepath.Join(dir, id+core.FileExtension)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("storing the upload: %w", err)
	}
	if _, err := io.Copy(f, body); err != nil {
		s.closeLogged(f, "export file")
		s.removeLogged(p)
		return nil, fmt.Errorf("storing the upload: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("storing the upload: %w", err)
	}
	rf, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer s.closeLogged(rf, "uploaded export")
	m, err := core.ReadManifest(rf)
	if err != nil {
		s.removeLogged(p)
		return nil, &core.ArtifactError{Err: err}
	}
	return &UploadResult{UploadID: id, Manifest: m}, nil
}

// sourcePath resolves a restore source to a file on disk.
func (s *Service) sourcePath(ctx context.Context, src RestoreSource) (string, error) {
	switch {
	case src.ExportID != "" && src.UploadID == "":
		p, _, err := s.FilePath(ctx, src.ExportID)
		return p, err
	case src.UploadID != "" && src.ExportID == "":
		if strings.ContainsAny(src.UploadID, `/\.`) {
			return "", ErrNotFound
		}
		p := filepath.Join(s.dataDir, ExportDir, UploadDir, src.UploadID+core.FileExtension)
		if _, err := os.Stat(p); err != nil {
			return "", ErrNotFound
		}
		return p, nil
	default:
		return "", ErrNotFound
	}
}

func (in RestoreInput) options(actor string) core.RestoreOptions {
	return core.RestoreOptions{
		Secret: core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}, Mode: in.Mode, Categories: in.Categories,
		IncludeAudit: in.IncludeAudit, Conflict: in.Conflict, Actor: actor, Via: "web",
	}
}

// Plan verifies the artifact end to end and states what a restore would change.
func (s *Service) Plan(ctx context.Context, in RestoreInput) (core.Plan, error) {
	p, err := s.sourcePath(ctx, in.Source)
	if err != nil {
		return core.Plan{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return core.Plan{}, fmt.Errorf("opening the export: %w", err)
	}
	defer s.closeLogged(f, "export file")
	plan, _, err := core.PlanRestore(ctx, s.target(), f, in.options(""))
	return plan, err
}

// StartRestore runs a restore in the background: maintenance mode, the
// restore, then a restart, because the key and secret it may replace are
// read once at boot.
func (s *Service) StartRestore(ctx context.Context, actor string, in RestoreInput) (*RestoreStatus, error) {
	if s.hooks.Restart == nil || s.hooks.Emit == nil {
		return nil, fmt.Errorf("restore is not available on this server")
	}
	path, err := s.sourcePath(ctx, in.Source)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.restoring {
		s.mu.Unlock()
		return nil, ErrRestoreRunning
	}
	s.restoring = true
	s.mu.Unlock()

	st := &RestoreStatus{ID: xid.New().String(), State: RestoreRunning, Actor: actor, Mode: in.Mode, StartedAt: now()}
	if err := s.writeStatus(st); err != nil {
		s.mu.Lock()
		s.restoring = false
		s.mu.Unlock()
		return nil, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runRestore(ctx, st, path, in)
	}()
	return st, nil
}

func (s *Service) runRestore(ctx context.Context, st *RestoreStatus, path string, in RestoreInput) {
	ctx, cancel := s.jobContext(ctx, 6*time.Hour)
	defer cancel()
	fail := func(err error) {
		s.logger.Error("instance restore failed", "id", st.ID, "error", err)
		st.State, st.Error, st.FinishedAt = RestoreFailed, err.Error(), now()
		var refused *core.RefusedError
		if errors.As(err, &refused) {
			st.Refusals = refused.Reasons
		}
		if werr := s.writeStatus(st); werr != nil {
			s.logger.Error("instance restore: recording the failure", "error", werr)
		}
		s.mu.Lock()
		s.restoring = false
		s.mu.Unlock()
	}

	middleware.BeginMaintenance("instance-restore")
	drainCtx, drainCancel := context.WithTimeout(ctx, 30*time.Second)
	if err := quiesce.Drain(drainCtx); err != nil {
		s.logger.Warn("instance restore: requests still running after the drain window", "error", err)
	}
	drainCancel()

	opts := in.options(st.Actor)
	opts.Emit = s.hooks.Emit
	opts.Progress = func(p core.Progress) {
		s.mu.Lock()
		s.restoreProgress = &p
		s.mu.Unlock()
	}
	res, err := core.Restore(ctx, s.target(), func() (io.ReadCloser, error) { return os.Open(path) }, opts)
	if err != nil {
		middleware.EndMaintenance()
		fail(err)
		return
	}
	st.Result, st.State = &res, RestoreRestarting
	if err := s.writeStatus(st); err != nil {
		s.logger.Error("instance restore: recording the result", "error", err)
	}
	if in.Source.UploadID != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("instance restore: removing the upload", "error", err)
		}
	}
	s.logger.Info("instance restore committed; restarting", "id", st.ID)
	if err := s.hooks.Restart(ctx); err != nil {
		middleware.EndMaintenance()
		fail(fmt.Errorf("the restore is complete but McHarbor could not restart itself (%w); restart it to finish", err))
	}
}

// RestoreStatus returns the last restore's status, with live progress while it runs.
func (s *Service) RestoreStatus() (*RestoreStatus, error) {
	st, err := s.readStatus()
	if err != nil || st == nil {
		return st, err
	}
	s.mu.Lock()
	if s.restoring && s.restoreProgress != nil {
		p := *s.restoreProgress
		st.Progress = &p
	}
	s.mu.Unlock()
	return st, nil
}

// ResumeAfterRestart runs once at boot. A restore that restarted the server
// finishes here: its plugins are reinstalled at the recorded versions, and the
// server restarts once more when any plugin binary changed, so it loads.
func (s *Service) ResumeAfterRestart(ctx context.Context) {
	st, err := s.readStatus()
	if err != nil || st == nil || st.State != RestoreRestarting || st.Result == nil {
		if err != nil {
			s.logger.Warn("instance restore: reading the last status", "error", err)
		}
		return
	}
	st.State = RestoreReconciling
	if err := s.writeStatus(st); err != nil {
		s.logger.Error("instance restore: recording the plugin step", "error", err)
		return
	}
	changed := false
	if s.hooks.Plugins != nil && len(st.Result.Plugins) > 0 {
		st.Plugins = s.hooks.Plugins.ReconcileInstalled(ctx, st.Result.Plugins)
		for _, o := range st.Plugins {
			if o.Action == "installed" || o.Action == "updated" {
				changed = true
			}
		}
	}
	st.State, st.FinishedAt = RestoreDone, now()
	if err := s.writeStatus(st); err != nil {
		s.logger.Error("instance restore: recording the outcome", "error", err)
	}
	if changed && s.hooks.Restart != nil {
		s.logger.Info("instance restore: plugins reinstalled; restarting to load them", "id", st.ID)
		if err := s.hooks.Restart(ctx); err != nil {
			s.logger.Error("instance restore: restarting after plugin reinstall", "error", err)
		}
	}
}

func (s *Service) writeStatus(st *RestoreStatus) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	p := filepath.Join(s.dataDir, statusFile)
	if err := os.WriteFile(p+".tmp", raw, 0o600); err != nil {
		return fmt.Errorf("writing the restore status: %w", err)
	}
	return os.Rename(p+".tmp", p)
}

func (s *Service) readStatus() (*RestoreStatus, error) {
	raw, err := os.ReadFile(filepath.Join(s.dataDir, statusFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the restore status: %w", err)
	}
	var st RestoreStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("the restore status is damaged: %w", err)
	}
	return &st, nil
}

// HasActiveElevation reports whether userID holds perm through an approved,
// unexpired elevation. Restoring audit history takes an approval at the
// moment of use, so a standing role grant is not enough.
func (s *Service) HasActiveElevation(ctx context.Context, userID, perm string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM elevation_requests
		WHERE requester_id = ? AND permission_key = ? AND status = 'approved'
		  AND approved_until IS NOT NULL AND approved_until > ?`, userID, perm, now()).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking elevations: %w", err)
	}
	return n > 0, nil
}

// ResumeInBackground runs ResumeAfterRestart under the service's lifetime.
func (s *Service) ResumeInBackground() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.ResumeAfterRestart(s.life)
	}()
}

// RecordLocalRestore writes the status a command-line restore leaves behind,
// so the next server start reinstalls the export's plugins exactly as it does
// after a restore from the web interface.
func RecordLocalRestore(dataDir, actor, mode string, res core.Result) error {
	s := &Service{dataDir: dataDir}
	return s.writeStatus(&RestoreStatus{
		ID: res.ID, State: RestoreRestarting, Actor: actor, Mode: mode, StartedAt: now(), Result: &res,
	})
}
