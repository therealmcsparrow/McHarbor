// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

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

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/version"
)

// ExportDir is where finished exports are kept, under the data directory. It
// is not one of the export's own file roots, so exports never nest.
const ExportDir = "instance-backups"

// ErrNotFound is returned for an unknown export id.
var ErrNotFound = errors.New("instance export not found")

// ErrBusy is returned while another export is running; two at once would
// double the load for no benefit.
var ErrBusy = errors.New("an instance export is already running")

// Service runs exports as background jobs and keeps their history.
type Service struct {
	db      *sql.DB
	driver  coredb.Driver
	enc     *encryption.Service
	dataDir string
	secret  string
	logger  *slog.Logger

	life   context.Context //nolint:containedctx // lifecycle root for backup and restore jobs, outliving the request that starts one
	mu     sync.Mutex
	active map[string]*job
	wg     sync.WaitGroup

	hooks           RestoreHooks
	restoring       bool
	restoreProgress *core.Progress
}

type job struct {
	cancel   context.CancelFunc
	progress core.Progress
}

// NewService returns a Service whose jobs end when life is cancelled.
func NewService(life context.Context, db *sql.DB, driver coredb.Driver, enc *encryption.Service, dataDir, secret string, logger *slog.Logger) *Service {
	return &Service{db: db, driver: driver, enc: enc, dataDir: dataDir, secret: secret, logger: logger, life: life, active: map[string]*job{}}
}

// closeLogged closes c and logs a failure: a Close error on a read path never
// changes the outcome, but it must not vanish either.
func closeLogged(l *slog.Logger, c io.Closer, what string) {
	if err := c.Close(); err != nil {
		l.Warn("instance backup: closing "+what, "error", err)
	}
}

func (s *Service) closeLogged(c io.Closer, what string) { closeLogged(s.logger, c, what) }

// removeLogged deletes a scratch file after a failed step; a file that is
// already gone is the desired state, anything else is logged.
func (s *Service) removeLogged(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warn("instance backup: removing a scratch file", "path", path, "error", err)
	}
}

// jobContext returns a context that keeps ctx's values but not its
// cancellation, bounded by timeout and cancelled when the service's life ends,
// so a background job outlives the request that started it and not the server.
func (s *Service) jobContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	jc, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	stop := context.AfterFunc(s.life, cancel)
	return jc, func() {
		stop()
		cancel()
	}
}

// Wait blocks until every running job has stopped.
func (s *Service) Wait() { s.wg.Wait() }

// FailInterrupted marks exports left running by a previous process as failed;
// nothing resumes them, and a row stuck at running forever would read as live.
func (s *Service) FailInterrupted(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE instance_backup_exports SET status = ?, error = ?, updated_at = ? WHERE status = ?`,
		StatusFailed, "interrupted: McHarbor restarted while this export was running", now(), StatusRunning)
	if err != nil {
		return fmt.Errorf("marking interrupted exports: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE instance_backup_exports SET verify_status = ?, verify_error = ?, updated_at = ? WHERE verify_status = ?`,
		"failed", "interrupted: McHarbor restarted while this export was being verified", now(), "running"); err != nil {
		return fmt.Errorf("marking interrupted verifications: %w", err)
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// Source describes this instance for the exporter.
func (s *Service) Source(ctx context.Context, createdBy string) (core.Source, error) {
	var uploaded int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = 'tls_cert' AND value <> ''`).Scan(&uploaded); err != nil {
		return core.Source{}, fmt.Errorf("reading the certificate setting: %w", err)
	}
	var instanceID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM license_installs ORDER BY created_at LIMIT 1`).Scan(&instanceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return core.Source{}, fmt.Errorf("reading the instance id: %w", err)
	}
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	return core.Source{
		DB: s.db, Driver: s.driver, DataDir: s.dataDir, EncryptionKey: s.enc.KeyMaterial(), Secret: s.secret,
		UploadedCert: uploaded > 0, Version: version.Current(), Build: version.Build(), InstanceID: instanceID,
		CreatedBy: createdBy, Hostname: host,
	}, nil
}

// StartExport records a new export and runs it in the background.
func (s *Service) StartExport(ctx context.Context, createdBy, trigger string, scheduleID *string, in CreateExportInput, uploader Uploader) (*Export, error) {
	id, src, jobCtx, cancel, err := s.createExport(ctx, createdBy, trigger, scheduleID, in)
	if err != nil {
		return nil, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		s.run(jobCtx, id, src, in, uploader)
	}()
	return s.Get(ctx, id)
}

// runExport records an export and runs it to completion on the caller's
// goroutine, for a scheduler that needs the outcome before it continues.
func (s *Service) runExport(ctx context.Context, createdBy, trigger string, scheduleID *string, in CreateExportInput) (*Export, error) {
	id, src, jobCtx, cancel, err := s.createExport(ctx, createdBy, trigger, scheduleID, in)
	if err != nil {
		return nil, err
	}
	defer cancel()
	// nil: the caller (RunSchedule) copies to the location itself, so an
	// uploader here would upload the same export twice.
	s.run(jobCtx, id, src, in, nil)
	return s.Get(ctx, id)
}

func (s *Service) createExport(ctx context.Context, createdBy, trigger string, scheduleID *string, in CreateExportInput) (string, core.Source, context.Context, context.CancelFunc, error) {
	s.mu.Lock()
	busy := len(s.active) > 0
	s.mu.Unlock()
	if busy {
		return "", core.Source{}, nil, nil, ErrBusy
	}
	src, err := s.Source(ctx, createdBy)
	if err != nil {
		return "", core.Source{}, nil, nil, err
	}
	if err := os.MkdirAll(filepath.Join(s.dataDir, ExportDir), 0o700); err != nil {
		return "", core.Source{}, nil, nil, fmt.Errorf("preparing the export directory: %w", err)
	}
	id := xid.New().String()
	kind := core.KeyKindPassphrase
	if in.KeyFile != "" {
		kind = core.KeyKindKeyFile
	}
	created := now()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO instance_backup_exports
		(id, status, trigger_kind, schedule_id, created_by, key_kind, include_metrics, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, StatusRunning, trigger, scheduleID, createdBy, kind, boolInt(in.IncludeMetrics), in.Note, created, created); err != nil {
		return "", core.Source{}, nil, nil, fmt.Errorf("recording the export: %w", err)
	}

	jobCtx, cancel := context.WithCancel(s.life)
	s.mu.Lock()
	s.active[id] = &job{cancel: cancel, progress: core.Progress{Phase: "starting"}}
	s.mu.Unlock()
	return id, src, jobCtx, cancel, nil
}

func (s *Service) run(ctx context.Context, id string, src core.Source, in CreateExportInput, uploader Uploader) {
	defer func() {
		s.mu.Lock()
		delete(s.active, id)
		s.mu.Unlock()
	}()
	name := fmt.Sprintf("mcharbor-%s%s", time.Now().UTC().Format("20060102-150405"), core.FileExtension)
	final := filepath.Join(s.dataDir, ExportDir, id+"-"+name)
	partial := final + ".partial"
	m, size, err := s.writeArtifact(ctx, id, src, in, partial)
	if err == nil {
		err = os.Rename(partial, final)
	}
	if err != nil {
		if rmErr := os.Remove(partial); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			s.logger.Warn("instance backup: removing a partial export", "error", rmErr)
		}
		s.logger.Error("instance backup: export failed", "id", id, "error", err)
		s.finish(ctx, id, StatusFailed, "", 0, nil, err.Error())
		return
	}
	s.finish(ctx, id, StatusCompleted, filepath.Base(final), size, &m, "")

	// The uploads follow the completed local export rather than replacing it, and
	// a failure is recorded against its destination rather than failing the
	// export: the backup exists, and an operator who can see "stored here, not
	// offsite" can act on it. Reading the row back picks up the file name
	// finish() just wrote.
	if len(in.LocationIDs) == 0 || uploader == nil {
		return
	}
	e, err := s.Get(ctx, id)
	if err != nil {
		s.logger.Error("instance backup: reading an export before upload", "id", id, "error", err)
		return
	}
	if ok, problems := s.copyToLocations(ctx, e, final, in.LocationIDs, uploader); !ok {
		s.logger.Error("instance backup: no destination accepted an export", "id", id,
			"destinations", len(in.LocationIDs), "problems", strings.Join(problems, "; "))
	}
}

func (s *Service) writeArtifact(ctx context.Context, id string, src core.Source, in CreateExportInput, path string) (core.Manifest, int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return core.Manifest{}, 0, fmt.Errorf("creating the export file: %w", err)
	}
	m, err := core.Export(ctx, src, f, core.ExportOptions{
		Secret:         core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile},
		IncludeMetrics: in.IncludeMetrics, Note: in.Note,
		Progress: func(p core.Progress) {
			s.mu.Lock()
			if j, ok := s.active[id]; ok {
				j.progress = p
			}
			s.mu.Unlock()
		},
	})
	if err != nil {
		s.closeLogged(f, "export file")
		return core.Manifest{}, 0, err
	}
	if err := f.Sync(); err != nil {
		s.closeLogged(f, "export file")
		return core.Manifest{}, 0, fmt.Errorf("flushing the export file: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		s.closeLogged(f, "export file")
		return core.Manifest{}, 0, fmt.Errorf("sizing the export file: %w", err)
	}
	if err := f.Close(); err != nil {
		return core.Manifest{}, 0, fmt.Errorf("closing the export file: %w", err)
	}
	return m, st.Size(), nil
}

// finish writes a job's outcome. It uses a fresh context: the job's own may
// be the one that was cancelled, and the outcome must still be recorded.
func (s *Service) finish(ctx context.Context, id, status, file string, size int64, m *core.Manifest, errText string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	var manifest, keyKind, keyID string
	if m != nil {
		raw, err := json.Marshal(m)
		if err == nil {
			manifest = string(raw)
		}
		keyKind, keyID = m.Key.Kind, m.Key.KeyID
	}
	completed := now()
	if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_exports SET status = ?, file_name = ?, size_bytes = ?,
		manifest = ?, key_kind = COALESCE(NULLIF(?, ''), key_kind), key_id = ?, error = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		status, file, size, manifest, keyKind, keyID, errText, completed, completed, id); err != nil {
		s.logger.Error("instance backup: recording the outcome", "id", id, "error", err)
	}
}

const exportColumns = `id, status, trigger_kind, schedule_id, created_by, file_name, size_bytes, manifest, key_kind, key_id,
	include_metrics, note, error, location_id, location_key, location_status, location_error, verify_status, verify_error,
	verified_at, completed_at, created_at`

// withDestinations attaches where an export was sent. A read failure is logged
// and the export is still returned: the backup's own row is the important part.
func (s *Service) withDestinations(ctx context.Context, e *Export) *Export {
	if e == nil {
		return nil
	}
	dests, err := s.exportDestinations(ctx, e.ID)
	if err != nil {
		s.logger.Warn("instance backup: reading destinations", "id", e.ID, "error", err)
		return e
	}
	e.Destinations = dests
	return e
}

func (s *Service) scan(row interface{ Scan(...any) error }) (*Export, error) {
	var e Export
	var manifest string
	var metrics int64
	if err := row.Scan(&e.ID, &e.Status, &e.Trigger, &e.ScheduleID, &e.CreatedBy, &e.FileName, &e.SizeBytes, &manifest,
		&e.KeyKind, &e.KeyID, &metrics, &e.Note, &e.Error, &e.LocationID, &e.LocationKey, &e.LocationStatus, &e.LocationError,
		&e.VerifyStatus, &e.VerifyError, &e.VerifiedAt, &e.CompletedAt, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.IncludeMetrics = metrics != 0
	if manifest != "" {
		var m core.Manifest
		if err := json.Unmarshal([]byte(manifest), &m); err == nil {
			e.Manifest = &m
		}
	}
	s.mu.Lock()
	if j, ok := s.active[e.ID]; ok {
		p := j.progress
		e.Progress = &p
	}
	s.mu.Unlock()
	return &e, nil
}

// List returns one page of the export history, newest first.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Export, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instance_backup_exports`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting exports: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+exportColumns+` FROM instance_backup_exports
		ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("listing exports: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing query rows", "error", err)
		}
	}()
	out := []Export{}
	for rows.Next() {
		e, err := s.scan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("reading an export: %w", err)
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// The history table shows each destination's own outcome, so the page needs
	// them here and not only on a detail read. One query per row on a page of 25.
	for i := range out {
		s.withDestinations(ctx, &out[i])
	}
	return out, total, nil
}

// Get returns one export.
func (s *Service) Get(ctx context.Context, id string) (*Export, error) {
	e, err := s.scan(s.db.QueryRowContext(ctx, `SELECT `+exportColumns+` FROM instance_backup_exports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading export %s: %w", id, err)
	}
	return s.withDestinations(ctx, e), nil
}

// FilePath returns the on-disk artifact of a completed export.
func (s *Service) FilePath(ctx context.Context, id string) (string, *Export, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if e.Status != StatusCompleted || e.FileName == "" || strings.ContainsAny(e.FileName, `/\`) {
		return "", nil, ErrNotFound
	}
	p := filepath.Join(s.dataDir, ExportDir, e.FileName)
	if _, err := os.Stat(p); err != nil {
		return "", nil, ErrNotFound
	}
	return p, e, nil
}

// Delete removes an export and its file. A running export is cancelled first.
func (s *Service) Delete(ctx context.Context, id string, uploader Uploader) (*Export, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if j, ok := s.active[id]; ok {
		j.cancel()
	}
	s.mu.Unlock()
	if e.FileName != "" && !strings.ContainsAny(e.FileName, `/\`) {
		if err := os.Remove(filepath.Join(s.dataDir, ExportDir, e.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("deleting the export file: %w", err)
		}
	}
	// The remote copies go too. Deleting the row first would orphan every object
	// permanently: nothing would be left that knows their keys.
	s.removeFromLocations(ctx, id, uploader)
	res, err := s.db.ExecContext(ctx, `DELETE FROM instance_backup_exports WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("deleting export %s: %w", id, err)
	}
	if coredb.RowsAffected(res) == 0 {
		return nil, ErrNotFound
	}
	return e, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Verification statuses on an export row.
const (
	VerifyRunning = "running"
	VerifyPassed  = "passed"
	VerifyFailed  = "failed"
)

// StartVerify rehearses a completed export in the background: it is restored
// into a throwaway database and checked, and the outcome is stored on the row so
// the list can say "last verified" rather than only "last created".
func (s *Service) StartVerify(ctx context.Context, id string, secret core.Secret) (*Export, error) {
	p, _, err := s.FilePath(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_exports SET verify_status = ?, verify_error = '', updated_at = ? WHERE id = ?`,
		VerifyRunning, now(), id); err != nil {
		return nil, fmt.Errorf("recording the verification: %w", err)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.verify(ctx, id, p, secret)
	}()
	return s.Get(ctx, id)
}

func (s *Service) verify(ctx context.Context, id, path string, secret core.Secret) {
	ctx, cancel := s.jobContext(ctx, 6*time.Hour)
	defer cancel()
	scratch := filepath.Join(s.dataDir, ExportDir, ".rehearsals")
	status, problem := VerifyPassed, ""
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		status, problem = VerifyFailed, err.Error()
	} else {
		r, err := core.Rehearse(ctx, scratch, func() (io.ReadCloser, error) { return os.Open(path) }, secret)
		switch {
		case err != nil:
			status, problem = VerifyFailed, err.Error()
		case !r.OK:
			status, problem = VerifyFailed, rehearsalProblem(r)
		}
	}
	done := context.WithoutCancel(ctx)
	if _, err := s.db.ExecContext(done, `UPDATE instance_backup_exports SET verify_status = ?, verify_error = ?, verified_at = ?, updated_at = ? WHERE id = ?`,
		status, problem, now(), now(), id); err != nil {
		s.logger.Error("instance backup: recording the verification", "id", id, "error", err)
	}
	if status == VerifyFailed {
		s.logger.Warn("instance backup: an export failed its restore rehearsal", "id", id, "problem", problem)
	}
}

func rehearsalProblem(r core.Rehearsal) string {
	if r.Problem != "" {
		return r.Problem
	}
	if len(r.Mismatches) > 0 {
		return "rows differ from the manifest: " + strings.Join(r.Mismatches, "; ")
	}
	return "the audit chain did not verify"
}
