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
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/cronspec"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// Uploader sends a finished export to a storage location and removes old ones.
type Uploader interface {
	Upload(ctx context.Context, locationID, key string, body io.Reader, size int64) (string, error)
	Delete(ctx context.Context, locationID, key string) error
}

// Schedule is a recurring export. Its key is stored encrypted with the
// instance key because an unattended export has nobody to ask; the operator
// must still hold the same key elsewhere, or the exports cannot be restored.
type Schedule struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Cron           string `json:"cron"`
	Enabled        bool   `json:"enabled"`
	KeyKind        string `json:"keyKind"`
	KeyID          string `json:"keyId"`
	IncludeMetrics bool   `json:"includeMetrics"`
	// LocationIDs is every storage location this schedule copies to.
	LocationIDs []string `json:"locationIds"`
	// LocationID is the pre-migration-123 single destination, kept so an older
	// client still reads something truthful. LocationIDs is the truth.
	LocationID     *string `json:"locationId,omitempty"`
	RetentionCount int     `json:"retentionCount"`
	RetentionDays  int     `json:"retentionDays"`
	VerifyAfter    bool    `json:"verifyAfter"`
	LastRunAt      *string `json:"lastRunAt,omitempty"`
	NextRunAt      *string `json:"nextRunAt,omitempty"`
	LastStatus     string  `json:"lastStatus"`
	LastError      string  `json:"lastError"`
	CreatedBy      string  `json:"createdBy"`
	CreatedAt      string  `json:"createdAt"`
}

// ScheduleInput creates or updates a schedule. On update an empty key keeps
// the stored one.
type ScheduleInput struct {
	Name           string `json:"name"`
	Cron           string `json:"cron"`
	Enabled        bool   `json:"enabled"`
	Passphrase     string `json:"passphrase,omitempty"`
	KeyFile        string `json:"keyFile,omitempty"`
	IncludeMetrics bool   `json:"includeMetrics"`
	// LocationIDs replaces the whole set on every save: the editor sends what it
	// has, so a location the operator removed has to disappear.
	LocationIDs    []string `json:"locationIds,omitempty"`
	RetentionCount int      `json:"retentionCount"`
	RetentionDays  int      `json:"retentionDays"`
	VerifyAfter    bool     `json:"verifyAfter"`
}

// Check reports why in cannot be saved, or "" when it can.
func (in ScheduleInput) Check(creating bool) string {
	switch {
	case strings.TrimSpace(in.Name) == "" || len(in.Name) > 100:
		return "bad_name"
	case !cronspec.Valid(in.Cron):
		return "bad_cron"
	case in.RetentionCount < 0 || in.RetentionCount > 1000 || in.RetentionDays < 0 || in.RetentionDays > 3650:
		return "bad_retention"
	case in.RetentionCount == 0 && in.RetentionDays == 0:
		return "no_retention"
	}
	secret := core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}
	if !creating && in.Passphrase == "" && in.KeyFile == "" {
		return ""
	}
	return secret.Check()
}

type storedKey struct {
	Passphrase string `json:"passphrase,omitempty"`
	KeyFile    string `json:"keyFile,omitempty"`
}

const scheduleColumns = `id, name, cron, enabled, key_kind, key_id, include_metrics, location_id, retention_count, retention_days,
	verify_after, last_run_at, next_run_at, last_status, last_error, created_by, created_at`

func scanSchedule(row interface{ Scan(...any) error }) (*Schedule, error) {
	var sc Schedule
	var enabled, metrics, verify int64
	if err := row.Scan(&sc.ID, &sc.Name, &sc.Cron, &enabled, &sc.KeyKind, &sc.KeyID, &metrics, &sc.LocationID,
		&sc.RetentionCount, &sc.RetentionDays, &verify, &sc.LastRunAt, &sc.NextRunAt, &sc.LastStatus, &sc.LastError,
		&sc.CreatedBy, &sc.CreatedAt); err != nil {
		return nil, err
	}
	sc.Enabled, sc.IncludeMetrics, sc.VerifyAfter = enabled != 0, metrics != 0, verify != 0
	return &sc, nil
}

// ListSchedules returns every schedule.
func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM instance_backup_schedules ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing schedules: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing query rows", "error", err)
		}
	}()
	out := []Schedule{}
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("reading a schedule: %w", err)
		}
		out = append(out, *sc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// One small query per schedule. There are at most a handful, and a join would
	// have to de-duplicate the schedule columns across its destination rows.
	for i := range out {
		ids, err := s.scheduleLocationIDs(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].LocationIDs = ids
	}
	return out, nil
}

// Schedule returns one schedule.
func (s *Service) Schedule(ctx context.Context, id string) (*Schedule, error) {
	sc, err := scanSchedule(s.db.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM instance_backup_schedules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading schedule %s: %w", id, err)
	}
	ids, err := s.scheduleLocationIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	sc.LocationIDs = ids
	return sc, nil
}

func (s *Service) sealKey(in ScheduleInput) (kind, enc, id string, err error) {
	key := storedKey{Passphrase: in.Passphrase, KeyFile: in.KeyFile}
	raw, err := json.Marshal(key)
	if err != nil {
		return "", "", "", err
	}
	enc, err = s.enc.Encrypt(string(raw))
	if err != nil {
		return "", "", "", fmt.Errorf("encrypting the schedule key: %w", err)
	}
	kind = core.KeyKindPassphrase
	if in.KeyFile != "" {
		kind = core.KeyKindKeyFile
	}
	return kind, enc, core.SecretKeyID(core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}), nil
}

func nullable(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// CreateSchedule stores a new schedule and its first run time.
func (s *Service) CreateSchedule(ctx context.Context, createdBy string, in ScheduleInput) (*Schedule, error) {
	kind, enc, keyID, err := s.sealKey(in)
	if err != nil {
		return nil, err
	}
	id, ts := xid.New().String(), now()
	next := cronspec.Next(in.Cron, time.Now()).UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO instance_backup_schedules (id, name, cron, enabled, key_kind, key_secret_enc, key_id,
		include_metrics, location_id, retention_count, retention_days, verify_after, next_run_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.Name), in.Cron, boolInt(in.Enabled), kind, enc, keyID, boolInt(in.IncludeMetrics),
		nullable(firstID(in.LocationIDs)), in.RetentionCount, in.RetentionDays, boolInt(in.VerifyAfter), next, createdBy, ts, ts); err != nil {
		return nil, fmt.Errorf("saving the schedule: %w", err)
	}
	if err := s.setScheduleLocations(ctx, id, in.LocationIDs); err != nil {
		return nil, err
	}
	return s.Schedule(ctx, id)
}

// UpdateSchedule changes a schedule; its key is replaced only when a new one is given.
func (s *Service) UpdateSchedule(ctx context.Context, id string, in ScheduleInput) (*Schedule, error) {
	if _, err := s.Schedule(ctx, id); err != nil {
		return nil, err
	}
	next := cronspec.Next(in.Cron, time.Now()).UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_schedules SET name = ?, cron = ?, enabled = ?, include_metrics = ?,
		location_id = ?, retention_count = ?, retention_days = ?, verify_after = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(in.Name), in.Cron, boolInt(in.Enabled), boolInt(in.IncludeMetrics), nullable(firstID(in.LocationIDs)),
		in.RetentionCount, in.RetentionDays, boolInt(in.VerifyAfter), next, now(), id); err != nil {
		return nil, fmt.Errorf("saving the schedule: %w", err)
	}
	if in.Passphrase != "" || in.KeyFile != "" {
		kind, enc, keyID, err := s.sealKey(in)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_schedules SET key_kind = ?, key_secret_enc = ?, key_id = ? WHERE id = ?`,
			kind, enc, keyID, id); err != nil {
			return nil, fmt.Errorf("saving the schedule key: %w", err)
		}
	}
	if err := s.setScheduleLocations(ctx, id, in.LocationIDs); err != nil {
		return nil, err
	}
	return s.Schedule(ctx, id)
}

// DeleteSchedule removes a schedule. Its exports stay, detached from it.
func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM instance_backup_schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting schedule %s: %w", id, err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// RunDueSchedules runs every enabled schedule whose time has come. It is
// called once a minute by the leader; next_run_at moves first, so a slow run
// cannot be started twice.
func (s *Service) RunDueSchedules(ctx context.Context, uploader Uploader) {
	due, err := s.dueSchedules(ctx, now())
	if err != nil {
		s.logger.Error("instance backup: reading due schedules", "error", err)
		return
	}
	for _, sc := range due {
		if ctx.Err() != nil {
			return
		}
		s.RunSchedule(ctx, sc, uploader)
	}
}

// dueSchedules reads the enabled schedules whose next run has come, closing the
// cursor before any of them runs so a run never holds a connection open.
func (s *Service) dueSchedules(ctx context.Context, nowTS string) ([]Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM instance_backup_schedules
		WHERE enabled = 1 AND (next_run_at IS NULL OR next_run_at <= ?) ORDER BY next_run_at LIMIT 100`, nowTS)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing the schedule query", "error", err)
		}
	}()
	var due []Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			s.logger.Error("instance backup: reading a schedule", "error", err)
			continue
		}
		due = append(due, *sc)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("instance backup: iterating due schedules", "error", err)
	}
	return due, nil
}

// RunSchedule runs one schedule now: export, copy to its location, verify if
// asked, then apply retention. Each step's failure is recorded on the schedule.
func (s *Service) RunSchedule(ctx context.Context, sc Schedule, uploader Uploader) {
	next := cronspec.Next(sc.Cron, time.Now()).UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `UPDATE instance_backup_schedules SET next_run_at = ?, last_run_at = ?, updated_at = ? WHERE id = ?`,
		next, now(), now(), sc.ID); err != nil {
		s.logger.Error("instance backup: advancing a schedule", "id", sc.ID, "error", err)
		return
	}
	status, problem := StatusCompleted, ""
	defer func() {
		if _, err := s.db.ExecContext(context.WithoutCancel(ctx), `UPDATE instance_backup_schedules SET last_status = ?, last_error = ?, updated_at = ? WHERE id = ?`,
			status, problem, now(), sc.ID); err != nil {
			s.logger.Error("instance backup: recording a schedule run", "id", sc.ID, "error", err)
		}
	}()
	secret, err := s.scheduleSecret(ctx, sc.ID)
	if err != nil {
		status, problem = StatusFailed, err.Error()
		return
	}
	id := sc.ID
	e, err := s.runExport(ctx, "schedule: "+sc.Name, "schedule", &id, CreateExportInput{
		Passphrase: secret.Passphrase, KeyFile: secret.KeyFile, IncludeMetrics: sc.IncludeMetrics, Note: sc.Name,
	})
	if err != nil {
		status, problem = StatusFailed, err.Error()
		return
	}
	if e.Status != StatusCompleted {
		status, problem = StatusFailed, e.Error
		return
	}
	path := filepath.Join(s.dataDir, ExportDir, e.FileName)
	if ids, err := s.scheduleLocationIDs(ctx, sc.ID); err != nil {
		s.logger.Warn("instance backup: reading a schedule's destinations", "id", sc.ID, "error", err)
	} else if len(ids) > 0 && uploader != nil {
		// A schedule run is only a success if the backup reached somewhere it was
		// asked to reach. Reaching one of two is not a failure — the operator has
		// an offsite copy — so the run fails only when every destination refused.
		if ok, problems := s.copyToLocations(ctx, e, path, ids, uploader); !ok {
			status = StatusFailed
			problem = "no storage location accepted the export: " + strings.Join(problems, "; ")
		}
	}
	if sc.VerifyAfter {
		s.verify(ctx, e.ID, path, secret)
		if v, err := s.Get(ctx, e.ID); err == nil && v.VerifyStatus == VerifyFailed {
			status, problem = StatusFailed, "the export failed its restore rehearsal: "+v.VerifyError
		}
	}
	if err := s.applyRetention(ctx, sc, uploader); err != nil {
		s.logger.Warn("instance backup: applying retention", "schedule", sc.ID, "error", err)
	}
}

func (s *Service) scheduleSecret(ctx context.Context, id string) (core.Secret, error) {
	var enc string
	if err := s.db.QueryRowContext(ctx, `SELECT key_secret_enc FROM instance_backup_schedules WHERE id = ?`, id).Scan(&enc); err != nil {
		return core.Secret{}, fmt.Errorf("reading the schedule key: %w", err)
	}
	raw, err := s.enc.Decrypt(enc)
	if err != nil {
		return core.Secret{}, fmt.Errorf("the schedule key cannot be decrypted; set it again: %w", err)
	}
	var k storedKey
	if err := json.Unmarshal([]byte(raw), &k); err != nil {
		return core.Secret{}, fmt.Errorf("the schedule key is damaged; set it again")
	}
	return core.Secret{Passphrase: k.Passphrase, KeyFile: k.KeyFile}, nil
}

type retentionRow struct {
	id, file, created, key string
	loc                    *string
}

// retentionCandidates reads a schedule's completed exports newest first and
// closes the cursor before any is deleted.
func (s *Service) retentionCandidates(ctx context.Context, scheduleID string) ([]retentionRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, file_name, created_at, location_id, location_key FROM instance_backup_exports
		WHERE schedule_id = ? AND status = ? ORDER BY created_at DESC, id DESC LIMIT 1000`, scheduleID, StatusCompleted)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("instance backup: closing the retention query", "error", err)
		}
	}()
	var all []retentionRow
	for rows.Next() {
		var o retentionRow
		if err := rows.Scan(&o.id, &o.file, &o.created, &o.loc, &o.key); err != nil {
			return nil, err
		}
		all = append(all, o)
	}
	return all, rows.Err()
}

// applyRetention removes a schedule's oldest completed exports beyond its
// count and age limits, here and at the storage location.
func (s *Service) applyRetention(ctx context.Context, sc Schedule, uploader Uploader) error {
	all, err := s.retentionCandidates(ctx, sc.ID)
	if err != nil {
		return err
	}
	cutoff := ""
	if sc.RetentionDays > 0 {
		cutoff = time.Now().Add(-time.Duration(sc.RetentionDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	for i, o := range all {
		expired := (sc.RetentionCount > 0 && i >= sc.RetentionCount) || (cutoff != "" && o.created < cutoff)
		if !expired {
			continue
		}
		if o.loc != nil && o.key != "" && uploader != nil {
			if err := uploader.Delete(ctx, *o.loc, o.key); err != nil {
				s.logger.Warn("instance backup: removing an expired export from storage", "id", o.id, "error", err)
			}
		}
		// nil: the remote object was removed just above, so Delete must not try
		// again and log a spurious failure for an object already gone.
		if _, err := s.Delete(ctx, o.id, nil); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// StartScheduler checks for due schedules once a minute until the service's
// lifetime ends. Only the node leader runs them, so an HA pair exports once.
func (s *Service) StartScheduler(leader func(context.Context) bool, uploader Uploader) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-s.life.Done():
				return
			case <-t.C:
				if leader == nil || leader(s.life) {
					s.RunDueSchedules(s.life, uploader)
				}
			}
		}
	}()
}

// RunScheduleInBackground runs one schedule now, off the request.
func (s *Service) RunScheduleInBackground(sc Schedule, uploader Uploader) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.RunSchedule(s.life, sc, uploader)
	}()
}

// firstID is the legacy single-destination summary: the first location a
// schedule or export names. Kept only so a reader older than migration 123 sees
// something truthful rather than nothing.
func firstID(ids []string) string {
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			return id
		}
	}
	return ""
}
