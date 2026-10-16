// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Scheduled reports: the record, and the cron that drives it.
//
// A schedule is a saved generation request plus a cron and at least one
// delivery target. It keeps exactly one run — its most recent — and that
// run never enters the report library: the library is what a person
// asked for, and a nightly job would bury it in a week.
//
// Everything the run needs to reproduce the request is stored as its own
// column rather than as an opaque blob, so the schedules table can be
// listed and filtered without decoding JSON per row.

// ScheduleTargetKind is where a finished report is delivered.
type ScheduleTargetKind string

const (
	TargetComm    ScheduleTargetKind = "comm"
	TargetStorage ScheduleTargetKind = "storage"
)

// Run statuses. `partial` is its own state on purpose: a report that
// reached two of three targets is neither a success nor a failure, and
// collapsing it into either loses the one fact an operator needs.
const (
	RunRunning = "running"
	RunSuccess = "success"
	RunPartial = "partial"
	RunFailed  = "failed"
)

var (
	errScheduleNotFound = errors.New("reporting: schedule not found")
	errNoTargets        = errors.New("reporting: a schedule must deliver somewhere")
	errBadCron          = errors.New("reporting: unreadable schedule expression")
	errBadTarget        = errors.New("reporting: unusable delivery target")
)

// ScheduleTarget is one delivery destination.
//
// Name and SupportsFiles are resolved from the host at read time rather
// than stored: a channel renamed in settings must not keep showing its
// old name here, and an adapter that gains file support in a rebuilt
// plugin must stop being labelled "link only" without anyone editing a
// schedule.
type ScheduleTarget struct {
	ID    string             `json:"id"`
	Kind  ScheduleTargetKind `json:"kind"`
	RefID string             `json:"ref_id"`

	// Path is the destination folder for a storage target, relative to
	// the location's own configured base path. Empty means the plugin's
	// default (see defaultStoragePath), which is what every schedule
	// created before this field existed keeps doing. Always empty for a
	// comm target — a channel has no folder.
	Path string `json:"path,omitempty"`

	// Filename is the pattern a storage target names its file with,
	// before the format's extension is appended. Empty means the
	// plugin's own dated name (see exportFilename), which is what every
	// schedule created before this field existed keeps doing. Always
	// empty for a comm target, where the recipient sees the attachment
	// name the exporter chose.
	Filename string `json:"filename,omitempty"`

	// Recipient retargets a comm delivery to a destination other than
	// the channel's own - one report to a board member from the channel
	// that mails the ops list every morning. Empty means the channel's
	// configured destination, and is the only possibility for a channel
	// whose adapter declared no overridable field. Always empty for a
	// storage target, which is retargeted with Path instead.
	Recipient string `json:"recipient,omitempty"`

	Name string `json:"name,omitempty"`
	// SupportsFiles is meaningful for comm targets only. False means the
	// host will drop the document and the recipient gets the summary text
	// alone, which the UI says out loud rather than promising an
	// attachment that never arrives.
	SupportsFiles bool `json:"supports_files"`
	// Missing marks a target whose host row has gone — a deleted channel
	// or storage location. The schedule keeps running for its other
	// targets and says so, rather than silently delivering less.
	Missing bool `json:"missing,omitempty"`
	// SupportsRecipient is meaningful for comm targets only, and is
	// resolved from the host at read time for the same reason Name is:
	// a channel type that gains or loses an overridable destination in
	// a rebuilt plugin must change what this schedule offers without
	// anyone editing it. False with a non-empty Recipient means a
	// stored override the adapter no longer honours, which the UI says
	// out loud rather than leaving on screen as if it still worked.
	SupportsRecipient bool `json:"supports_recipient"`
}

// DeliveryOutcome is one target's result from the last run.
type DeliveryOutcome struct {
	Kind  ScheduleTargetKind `json:"kind"`
	RefID string             `json:"ref_id"`
	Name  string             `json:"name"`
	OK    bool               `json:"ok"`
	// Code is an i18n code. A raw transport error can carry a hostname, a
	// bucket path or a token fragment, so the real error is logged
	// server-side and only the code is stored.
	Code string `json:"code,omitempty"`
	// Attached records whether the document itself travelled. False on a
	// comm target whose adapter cannot carry files — the message went,
	// the file did not.
	Attached bool `json:"attached"`
}

// ScheduleRun is the most recent run of one schedule.
type ScheduleRun struct {
	ScheduleID string            `json:"schedule_id"`
	Status     string            `json:"status"`
	Title      string            `json:"title"`
	Code       string            `json:"code,omitempty"`
	Deliveries []DeliveryOutcome `json:"deliveries"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at,omitempty"`

	// Document is populated only by the single-run read, never by the
	// list: a page of schedules would otherwise carry a rendered document
	// each.
	Document *Document `json:"document,omitempty"`
}

// Schedule is one saved, recurring report.
type Schedule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ReportType string `json:"report_type"`

	SubjectKind   SubjectKind `json:"subject_kind,omitempty"`
	SubjectID     string      `json:"subject_id,omitempty"`
	SubjectName   string      `json:"subject_name,omitempty"`
	EnvironmentID string      `json:"environment_id"`
	Environment   string      `json:"environment"`
	PeriodDays    int         `json:"period_days"`
	Levels        []Level     `json:"levels"`
	Options       []string    `json:"options"`
	Format        string      `json:"format"`
	Prompt        string      `json:"prompt,omitempty"`
	// AIConnectionID pins which configured AI writes a scheduled ad-hoc
	// report, so consecutive periods stay comparable instead of drifting
	// to whatever the host's default happens to be that month.
	AIConnectionID string `json:"ai_connection_id,omitempty"`

	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`

	CreatedByID string `json:"-"`
	CreatedBy   string `json:"created_by"`

	LastRunAt time.Time `json:"last_run_at,omitempty"`
	NextRunAt time.Time `json:"next_run_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Targets []ScheduleTarget `json:"targets"`
	LastRun *ScheduleRun     `json:"last_run,omitempty"`
}

// --- schema self-heal -------------------------------------------------

// scheduleSchema is the DDL ensureSchema replays on every boot.
//
// It duplicates migrations/0004_schedules.sql on purpose, for the same
// reason ensureSchema already re-adds the report columns: the host only
// auto-applies a plugin's migrations on a *fresh install*, so an
// existing install upgrading to a build that carries a new migration
// file never runs it without an operator opting in. Every statement is
// `IF NOT EXISTS`, so a migrated install costs three no-ops.
var scheduleSchema = []string{
	`CREATE TABLE IF NOT EXISTS plugin_reporting_schedules (
		id               TEXT    PRIMARY KEY,
		name             TEXT    NOT NULL,
		report_type      TEXT    NOT NULL,
		subject_kind     TEXT    NOT NULL DEFAULT '',
		subject_id       TEXT    NOT NULL DEFAULT '',
		subject_name     TEXT    NOT NULL DEFAULT '',
		environment_id   TEXT    NOT NULL DEFAULT '',
		environment      TEXT    NOT NULL DEFAULT '',
		period_days      INTEGER NOT NULL DEFAULT 30,
		levels           TEXT    NOT NULL DEFAULT '',
		options_json     TEXT    NOT NULL DEFAULT '[]',
		format           TEXT    NOT NULL DEFAULT 'pdf',
		prompt           TEXT    NOT NULL DEFAULT '',
		ai_connection_id TEXT    NOT NULL DEFAULT '',
		cron             TEXT    NOT NULL,
		enabled          INTEGER NOT NULL DEFAULT 1,
		created_by_id    TEXT    NOT NULL DEFAULT '',
		created_by       TEXT    NOT NULL DEFAULT '',
		last_run_at      TEXT    NOT NULL DEFAULT '',
		next_run_at      TEXT    NOT NULL DEFAULT '',
		created_at       TEXT    NOT NULL,
		updated_at       TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_reporting_schedules_due
		ON plugin_reporting_schedules(enabled, next_run_at)`,
	`CREATE TABLE IF NOT EXISTS plugin_reporting_schedule_targets (
		id          TEXT NOT NULL PRIMARY KEY,
		schedule_id TEXT NOT NULL,
		kind        TEXT NOT NULL,
		ref_id      TEXT NOT NULL,
		created_at  TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_reporting_schedule_targets_schedule
		ON plugin_reporting_schedule_targets(schedule_id)`,
	`CREATE TABLE IF NOT EXISTS plugin_reporting_schedule_runs (
		schedule_id   TEXT PRIMARY KEY,
		status        TEXT NOT NULL,
		title         TEXT NOT NULL DEFAULT '',
		document_json TEXT NOT NULL DEFAULT '',
		error         TEXT NOT NULL DEFAULT '',
		delivery_json TEXT NOT NULL DEFAULT '[]',
		started_at    TEXT NOT NULL,
		finished_at   TEXT NOT NULL DEFAULT ''
	)`,
}

// scheduleTargetColumns are the columns added to
// plugin_reporting_schedule_targets after it was first created.
//
// `CREATE TABLE IF NOT EXISTS` above does nothing once the table exists,
// so a column added later needs its own ALTER — the same pattern, and the
// same reason, as reportSchemaColumns in store.go. Each entry has a
// matching migration file; this is a second, idempotent route to the same
// end state, not a replacement for it.
var scheduleTargetColumns = []reportSchemaColumn{
	// migrations/0005_schedule_target_path.sql
	{name: "path", ddl: `ALTER TABLE plugin_reporting_schedule_targets ADD COLUMN path TEXT NOT NULL DEFAULT ''`},
	// migrations/0006_schedule_target_recipient.sql
	{name: "filename", ddl: `ALTER TABLE plugin_reporting_schedule_targets ADD COLUMN filename TEXT NOT NULL DEFAULT ''`},
	{name: "recipient", ddl: `ALTER TABLE plugin_reporting_schedule_targets ADD COLUMN recipient TEXT NOT NULL DEFAULT ''`},
}

func (s *store) ensureScheduleSchema(ctx context.Context) error {
	for _, ddl := range scheduleSchema {
		if _, err := s.db.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("reporting: ensure schedule schema: %w", err)
		}
	}
	return s.ensureColumns(ctx, "plugin_reporting_schedule_targets", scheduleTargetColumns)
}

// ensureColumns adds whichever of `want` the table is missing.
//
// SQLite's ALTER TABLE ADD COLUMN is not idempotent — it errors when the
// column exists — so this checks PRAGMA table_info first and only adds
// what is genuinely absent. A table that does not exist yet returns zero
// rows rather than an error, and is left to whoever creates it.
func (s *store) ensureColumns(ctx context.Context, table string, want []reportSchemaColumn) error {
	rows, err := s.db.Query(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return fmt.Errorf("reporting: inspect %s: %w", table, err)
	}
	existing := map[string]bool{}
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("reporting: scan %s column: %w", table, err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("reporting: read %s: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("reporting: close %s query: %w", table, err)
	}
	if len(existing) == 0 {
		return nil
	}
	for _, col := range want {
		if existing[col.name] {
			continue
		}
		if _, err := s.db.Exec(ctx, col.ddl); err != nil {
			return fmt.Errorf("reporting: add column %s.%s: %w", table, col.name, err)
		}
	}
	return nil
}

// --- persistence ------------------------------------------------------

const scheduleColumns = `id, name, report_type, subject_kind, subject_id, subject_name,
	environment_id, environment, period_days, levels, options_json, format, prompt,
	ai_connection_id, cron, enabled, created_by_id, created_by,
	last_run_at, next_run_at, created_at, updated_at`

func scanSchedule(rows plugin.Rows) (Schedule, error) {
	var (
		sc                                 Schedule
		kind, levels, options              string
		enabled                            int
		lastRun, nextRun, created, updated string
	)
	if err := rows.Scan(&sc.ID, &sc.Name, &sc.ReportType, &kind, &sc.SubjectID, &sc.SubjectName,
		&sc.EnvironmentID, &sc.Environment, &sc.PeriodDays, &levels, &options, &sc.Format,
		&sc.Prompt, &sc.AIConnectionID, &sc.Cron, &enabled, &sc.CreatedByID, &sc.CreatedBy,
		&lastRun, &nextRun, &created, &updated); err != nil {
		return Schedule{}, err
	}
	sc.SubjectKind = SubjectKind(kind)
	sc.Levels = parseLevels(levels)
	sc.Enabled = enabled != 0
	sc.LastRunAt = parseTS(lastRun)
	sc.NextRunAt = parseTS(nextRun)
	sc.CreatedAt = parseTS(created)
	sc.UpdatedAt = parseTS(updated)
	if options != "" {
		// A malformed options blob degrades to "the type's defaults"
		// rather than failing the listing: the schedule is still real and
		// still needs to be visible so an operator can fix or delete it.
		_ = json.Unmarshal([]byte(options), &sc.Options)
	}
	if sc.Options == nil {
		sc.Options = []string{}
	}
	return sc, nil
}

func (s *store) listSchedules(ctx context.Context, limit, offset int) ([]Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+scheduleColumns+` FROM plugin_reporting_schedules
		 ORDER BY enabled DESC, next_run_at ASC, name ASC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Schedule{}
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *store) countSchedules(ctx context.Context) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM plugin_reporting_schedules`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	total := 0
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Err()
}

func (s *store) getSchedule(ctx context.Context, id string) (Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+scheduleColumns+` FROM plugin_reporting_schedules WHERE id = ?`, id)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Schedule{}, err
		}
		return Schedule{}, errScheduleNotFound
	}
	return scanSchedule(rows)
}

// dueSchedules returns the enabled schedules whose next run has passed.
//
// A schedule with an empty next_run_at is deliberately excluded rather
// than treated as "due now": an empty value means the cron never
// resolved, and firing it every minute would be the worst possible
// reading of a broken expression.
func (s *store) dueSchedules(ctx context.Context, now time.Time) ([]Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+scheduleColumns+` FROM plugin_reporting_schedules
		 WHERE enabled = 1 AND next_run_at != '' AND next_run_at <= ?
		 ORDER BY next_run_at ASC LIMIT 50`,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *store) insertSchedule(ctx context.Context, sc Schedule) error {
	now := time.Now().UTC().Format(time.RFC3339)
	options, err := json.Marshal(sc.Options)
	if err != nil {
		return fmt.Errorf("reporting: encode schedule options: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO plugin_reporting_schedules
			(id, name, report_type, subject_kind, subject_id, subject_name,
			 environment_id, environment, period_days, levels, options_json, format,
			 prompt, ai_connection_id, cron, enabled, created_by_id, created_by,
			 last_run_at, next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		sc.ID, sc.Name, sc.ReportType, string(sc.SubjectKind), sc.SubjectID, sc.SubjectName,
		sc.EnvironmentID, sc.Environment, sc.PeriodDays, levelsCSV(sc.Levels), string(options),
		sc.Format, sc.Prompt, sc.AIConnectionID, sc.Cron, boolInt(sc.Enabled),
		sc.CreatedByID, sc.CreatedBy, tsOrEmpty(sc.NextRunAt), now, now)
	return err
}

func (s *store) updateSchedule(ctx context.Context, sc Schedule) (bool, error) {
	options, err := json.Marshal(sc.Options)
	if err != nil {
		return false, fmt.Errorf("reporting: encode schedule options: %w", err)
	}
	n, err := s.db.Exec(ctx, `
		UPDATE plugin_reporting_schedules SET
			name = ?, report_type = ?, subject_kind = ?, subject_id = ?, subject_name = ?,
			environment_id = ?, environment = ?, period_days = ?, levels = ?, options_json = ?,
			format = ?, prompt = ?, ai_connection_id = ?, cron = ?, enabled = ?,
			next_run_at = ?, updated_at = ?
		WHERE id = ?`,
		sc.Name, sc.ReportType, string(sc.SubjectKind), sc.SubjectID, sc.SubjectName,
		sc.EnvironmentID, sc.Environment, sc.PeriodDays, levelsCSV(sc.Levels), string(options),
		sc.Format, sc.Prompt, sc.AIConnectionID, sc.Cron, boolInt(sc.Enabled),
		tsOrEmpty(sc.NextRunAt), time.Now().UTC().Format(time.RFC3339), sc.ID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// deleteSchedule removes the schedule and everything hanging off it.
//
// The three statements are not wrapped in a transaction because the
// plugin DB handle exposes none. The order is therefore chosen so a
// failure halfway leaves no schedule pointing at rows that are gone:
// children first, parent last.
func (s *store) deleteSchedule(ctx context.Context, id string) (bool, error) {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_reporting_schedule_runs WHERE schedule_id = ?`, id); err != nil {
		return false, err
	}
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_reporting_schedule_targets WHERE schedule_id = ?`, id); err != nil {
		return false, err
	}
	n, err := s.db.Exec(ctx, `DELETE FROM plugin_reporting_schedules WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// markRan records when a schedule last fired and when it fires next.
// Written before the run starts, not after: a generation that takes two
// minutes must not be picked up again by the next tick.
func (s *store) markRan(ctx context.Context, id string, ranAt, next time.Time) error {
	_, err := s.db.Exec(ctx,
		`UPDATE plugin_reporting_schedules SET last_run_at = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ?`,
		ranAt.UTC().Format(time.RFC3339), tsOrEmpty(next),
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// --- targets ----------------------------------------------------------

func (s *store) scheduleTargets(ctx context.Context, scheduleID string) ([]ScheduleTarget, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, kind, ref_id, path, filename, recipient
		   FROM plugin_reporting_schedule_targets
		 WHERE schedule_id = ? ORDER BY kind ASC, created_at ASC LIMIT 100`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []ScheduleTarget{}
	for rows.Next() {
		var t ScheduleTarget
		var kind string
		if err := rows.Scan(&t.ID, &kind, &t.RefID, &t.Path, &t.Filename, &t.Recipient); err != nil {
			return nil, err
		}
		t.Kind = ScheduleTargetKind(kind)
		out = append(out, t)
	}
	return out, rows.Err()
}

// replaceTargets swaps a schedule's whole target set. Editing targets in
// place would need a diff for no benefit — the set is small and always
// submitted whole by the form.
func (s *store) replaceTargets(ctx context.Context, scheduleID string, targets []ScheduleTarget) error {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_reporting_schedule_targets WHERE schedule_id = ?`, scheduleID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO plugin_reporting_schedule_targets
				(id, schedule_id, kind, ref_id, path, filename, recipient, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), scheduleID, string(t.Kind), t.RefID, t.Path, t.Filename,
			t.Recipient, now); err != nil {
			return err
		}
	}
	return nil
}

// --- runs -------------------------------------------------------------

// upsertRun writes the one run row a schedule keeps. The second run of a
// schedule replaces the first; the table can never hold two rows for one
// schedule, because schedule_id is its primary key.
func (s *store) upsertRun(ctx context.Context, run ScheduleRun, doc *Document) error {
	payload := ""
	if doc != nil {
		raw, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("reporting: encode scheduled document: %w", err)
		}
		payload = string(raw)
	}
	deliveries, err := json.Marshal(run.Deliveries)
	if err != nil {
		return fmt.Errorf("reporting: encode delivery outcomes: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO plugin_reporting_schedule_runs
			(schedule_id, status, title, document_json, error, delivery_json, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(schedule_id) DO UPDATE SET
			status = excluded.status,
			title = excluded.title,
			document_json = excluded.document_json,
			error = excluded.error,
			delivery_json = excluded.delivery_json,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at`,
		run.ScheduleID, run.Status, run.Title, payload, run.Code, string(deliveries),
		run.StartedAt.UTC().Format(time.RFC3339), tsOrEmpty(run.FinishedAt))
	return err
}

// lastRun reads a schedule's stored run. withDocument is false for the
// list, where a rendered document per row would make a page of schedules
// a multi-megabyte response.
func (s *store) lastRun(ctx context.Context, scheduleID string, withDocument bool) (*ScheduleRun, error) {
	rows, err := s.db.Query(ctx, `
		SELECT status, title, error, delivery_json, started_at, finished_at, document_json
		FROM plugin_reporting_schedule_runs WHERE schedule_id = ?`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}

	run := ScheduleRun{ScheduleID: scheduleID}
	var deliveries, started, finished, payload string
	if err := rows.Scan(&run.Status, &run.Title, &run.Code, &deliveries,
		&started, &finished, &payload); err != nil {
		return nil, err
	}
	run.StartedAt = parseTS(started)
	run.FinishedAt = parseTS(finished)
	if deliveries != "" {
		_ = json.Unmarshal([]byte(deliveries), &run.Deliveries)
	}
	if run.Deliveries == nil {
		run.Deliveries = []DeliveryOutcome{}
	}
	if withDocument && payload != "" {
		var doc Document
		if err := json.Unmarshal([]byte(payload), &doc); err != nil {
			return nil, fmt.Errorf("reporting: decode scheduled document: %w", err)
		}
		run.Document = &doc
	}
	return &run, nil
}

// --- storage paths ----------------------------------------------------

var errBadPath = errors.New("reporting: unusable storage path")

// maxStoragePathLen bounds what an operator can type into a key that will
// be joined onto a location's base path. Generous for a folder tree,
// short enough that no adapter meets a filename limit because of us.
const maxStoragePathLen = 200

// defaultStoragePath is where a storage target writes when the operator
// named no folder. It is what every schedule did before the path existed,
// so an empty value is a real default rather than a missing one.
func defaultStoragePath(scheduleName string) string {
	return "mcharbor-reports/" + slugify(scheduleName)
}

// sanitiseStoragePath normalises what is typing and rejects what is not.
//
// The split matters. Backslashes, doubled slashes and a trailing slash
// are how people type a folder, and silently fixing them is a kindness.
// A leading slash, a `..` segment, a drive letter or a control character
// change *which directory is written to* — the key is joined onto the
// base path an operator configured on the storage location, and quietly
// rewriting a traversal attempt into some other folder would be worse
// than refusing it. Those are an error the caller sees.
//
// An empty input is not an error: it means "use the default".
func sanitiseStoragePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", nil
	}
	// Windows-style separators are typing, not intent.
	p = strings.ReplaceAll(p, "\\", "/")
	for _, r := range p {
		if r < 0x20 || r == 0x7F {
			return "", errBadPath
		}
	}
	// `C:/reports` is an absolute path wearing a disguise, and so is
	// `/reports`: both say "ignore the location's base path", which is
	// not a decision a schedule gets to make.
	if strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return "", errBadPath
	}
	if len(p) > maxStoragePathLen {
		return "", errBadPath
	}

	segments := []string{}
	for _, seg := range strings.Split(p, "/") {
		seg = strings.TrimSpace(seg)
		switch seg {
		case "":
			// A doubled or trailing slash. Drop it.
			continue
		case ".":
			continue
		case "..":
			// Traversal. Refused rather than resolved — see above.
			return "", errBadPath
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 {
		return "", nil
	}
	return strings.Join(segments, "/"), nil
}

// --- filenames --------------------------------------------------------

var errBadFilename = errors.New("reporting: unusable filename pattern")

// maxFilenamePatternLen bounds the pattern before substitution. Every
// token expands to something short, so this leaves the rendered name
// well inside the 255 bytes every filesystem and object store allows.
const maxFilenamePatternLen = 120

// filenameTokens are what a pattern may contain, and the only braces
// sanitiseFilenamePattern accepts.
//
// A typo is refused rather than passed through as a literal: a folder
// full of files called `report-{dat}.pdf` is a mistake nobody notices
// until they go looking for a report by date.
var filenameTokens = map[string]bool{
	"{name}": true, "{type}": true, "{env}": true,
	"{date}": true, "{time}": true,
}

// sanitiseFilenamePattern normalises what is typing and rejects what
// would change where the file lands or what it claims to be.
//
// The split mirrors sanitiseStoragePath. A separator is not typing here
// but a second attempt at choosing a directory - the folder is the Path
// field, and a name that carries a slash would write outside it - so it
// is refused rather than stripped. So is an extension: the plugin
// appends the one that matches the bytes it rendered, and a pattern
// ending in `.pdf` on a DOCX schedule would produce a file that lies
// about its own format.
//
// An empty input is not an error: it means "the plugin's own name".
func sanitiseFilenamePattern(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", nil
	}
	if len(p) > maxFilenamePatternLen {
		return "", errBadFilename
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7F {
			return "", errBadFilename
		}
	}
	if strings.ContainsAny(p, `/\:`) {
		return "", errBadFilename
	}
	if strings.Contains(p, "..") {
		return "", errBadFilename
	}
	if strings.HasPrefix(p, ".") {
		// A leading dot hides the file on every unix-like store and
		// wins nothing anywhere else.
		return "", errBadFilename
	}
	if err := checkFilenameTokens(p); err != nil {
		return "", err
	}
	// The extension is the plugin's to append, so a pattern that
	// already carries one would produce `report.pdf.pdf` at best and a
	// mislabelled file at worst. A dot elsewhere in the name is fine -
	// `capacity.v2` is a name somebody meant.
	if i := strings.LastIndex(p, "."); i >= 0 {
		switch strings.ToLower(p[i+1:]) {
		case "pdf", "docx":
			return "", errBadFilename
		}
	}
	return p, nil
}

// checkFilenameTokens refuses any brace that is not a known token, and
// any brace that is never closed.
func checkFilenameTokens(p string) error {
	rest := p
	for {
		open := strings.Index(rest, "{")
		if open < 0 {
			// A closing brace with no opener is as much a typo as the
			// other direction.
			if strings.Contains(rest, "}") {
				return errBadFilename
			}
			return nil
		}
		if strings.Contains(rest[:open], "}") {
			return errBadFilename
		}
		shut := strings.Index(rest[open:], "}")
		if shut < 0 {
			return errBadFilename
		}
		token := rest[open : open+shut+1]
		if !filenameTokens[token] {
			return errBadFilename
		}
		rest = rest[open+shut+1:]
	}
}

// applyFilenamePattern renders a stored pattern for one run.
//
// Returns fallback unchanged when no pattern is set, so a schedule that
// named none keeps the exporter's dated name. Every substituted value
// is slugified: an environment called "Prod / EU" must not put a slash
// into a name that has just been refused one.
func applyFilenamePattern(pattern, fallback string, sc Schedule, rec StoredReport, at time.Time) string {
	if strings.TrimSpace(pattern) == "" {
		return fallback
	}
	ext := ""
	if i := strings.LastIndex(fallback, "."); i >= 0 {
		ext = fallback[i:]
	}
	r := strings.NewReplacer(
		"{name}", slugify(sc.Name),
		"{type}", slugify(rec.ReportType),
		"{env}", slugify(rec.Environment),
		"{date}", rec.PeriodEnd.Format("2006-01-02"),
		"{time}", at.UTC().Format("1504"),
	)
	name := strings.Trim(r.Replace(pattern), "-_ ")
	if name == "" {
		// Every token was empty - an unnamed schedule on an
		// environment-less report. The exporter's name is still a real
		// one, so fall back rather than writing a file called ".pdf".
		return fallback
	}
	return name + ext
}

// --- recipients -------------------------------------------------------

var errBadRecipient = errors.New("reporting: unusable delivery recipient")

// maxRecipientLen matches the host dispatcher's own bound, so a value
// this plugin accepts is never refused one layer down.
const maxRecipientLen = 512

// sanitiseRecipient normalises an alternative destination and refuses
// what must never reach a provider.
//
// A newline is the whole risk: several notification adapters compose a
// request by writing this value into a header - an SMTP `To:` above all
// - and a value carrying CR or LF could add headers of its own. The
// host validates again at dispatch, and both checks are deliberate: the
// one here turns it into an error the operator sees while editing,
// rather than a delivery that fails at 3am.
//
// The value is otherwise passed through verbatim. Only the adapter
// knows whether its provider wants an address, a numeric chat id or a
// topic, and guessing a format here would refuse valid destinations.
func sanitiseRecipient(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	if len(v) > maxRecipientLen {
		return "", errBadRecipient
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7F {
			return "", errBadRecipient
		}
	}
	return v, nil
}

// --- small helpers ----------------------------------------------------

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// tsOrEmpty stores the empty string rather than a zero timestamp, so
// "never" is distinguishable from "the first second of year one" in
// every query that compares next_run_at.
func tsOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// --- cron -------------------------------------------------------------
//
// A five-field cron, evaluated in UTC, implemented here rather than
// pulled in as a dependency: this plugin has no third-party modules at
// all (see go.mod), and every plugin build failure in this repository so
// far has been a dependency problem. The subset is the standard one —
// `*`, `*/step`, `a-b`, `a,b`, and plain numbers — which covers every
// schedule the studio can produce and the hand-written ones an operator
// is likely to type.
//
// UTC rather than the operator's timezone is a deliberate simplification:
// a report window is a date range, not a wall-clock appointment, and a
// schedule that silently shifts by an hour twice a year would make two
// consecutive monthly reports overlap or gap.

type cronSpec struct {
	minute [60]bool
	hour   [24]bool
	dom    [32]bool
	month  [13]bool
	dow    [7]bool

	// Standard cron's oddest rule: when *both* day-of-month and
	// day-of-week are restricted, a date matches if *either* does. When
	// one is `*`, the other simply applies.
	domRestricted bool
	dowRestricted bool
}

func parseCron(expr string) (cronSpec, error) {
	var spec cronSpec
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return spec, errBadCron
	}
	type target struct {
		set        []bool
		lo, hi     int
		restricted *bool
	}
	targets := []target{
		{set: spec.minute[:], lo: 0, hi: 59},
		{set: spec.hour[:], lo: 0, hi: 23},
		{set: spec.dom[:], lo: 1, hi: 31, restricted: &spec.domRestricted},
		{set: spec.month[:], lo: 1, hi: 12},
		{set: spec.dow[:], lo: 0, hi: 6, restricted: &spec.dowRestricted},
	}
	for i, f := range fields {
		t := targets[i]
		if t.restricted != nil {
			*t.restricted = f != "*"
		}
		if err := cronField(f, t.set, t.lo, t.hi); err != nil {
			return cronSpec{}, err
		}
	}
	return spec, nil
}

// cronField fills one field's bitset. `set` is a slice over the spec's
// array, so writing through it writes the spec.
func cronField(field string, set []bool, lo, hi int) error {
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return errBadCron
		}
		step := 1
		if slash := strings.Index(part, "/"); slash >= 0 {
			n, err := atoiRange(part[slash+1:], 1, hi-lo+1)
			if err != nil {
				return err
			}
			step = n
			part = part[:slash]
		}
		from, to := lo, hi
		switch {
		case part == "*":
			// full range, already set
		case strings.Contains(part, "-"):
			bounds := strings.SplitN(part, "-", 2)
			a, err := atoiRange(bounds[0], lo, hi)
			if err != nil {
				return err
			}
			bv, err := atoiRange(bounds[1], lo, hi)
			if err != nil {
				return err
			}
			if bv < a {
				return errBadCron
			}
			from, to = a, bv
		default:
			v, err := atoiRange(part, lo, hi)
			if err != nil {
				return err
			}
			// A bare number with a step means "from here to the end of
			// the range", which is how `5/15` is read everywhere else.
			from = v
			to = v
			if step > 1 {
				to = hi
			}
		}
		for v := from; v <= to; v += step {
			if v < 0 || v >= len(set) {
				return errBadCron
			}
			set[v] = true
		}
	}
	return nil
}

func atoiRange(s string, lo, hi int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errBadCron
	}
	v := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBadCron
		}
		v = v*10 + int(r-'0')
		if v > 9999 {
			return 0, errBadCron
		}
	}
	if v < lo || v > hi {
		return 0, errBadCron
	}
	return v, nil
}

// matchesDate reports whether a day satisfies the month/day fields.
func (c cronSpec) matchesDate(t time.Time) bool {
	if !c.month[int(t.Month())] {
		return false
	}
	dom := c.dom[t.Day()]
	dow := c.dow[int(t.Weekday())]
	if c.domRestricted && c.dowRestricted {
		return dom || dow
	}
	return dom && dow
}

// next returns the first firing strictly after `after`, in UTC.
//
// The search skips whole days and whole hours rather than stepping a
// minute at a time, so the worst case (`0 0 29 2 *` — the 29th of
// February) costs a few thousand comparisons rather than two million.
// Nothing beyond four years matches a five-field cron, so a spec that
// finds nothing in that window is treated as never firing rather than
// looping forever.
func (c cronSpec) next(after time.Time) (time.Time, bool) {
	t := after.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(4, 0, 0)
	for t.Before(limit) {
		if !c.matchesDate(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
			continue
		}
		if !c.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC).Add(time.Hour)
			continue
		}
		if !c.minute[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// nextCronRun is the one entry point the rest of the plugin uses.
func nextCronRun(expr string, after time.Time) (time.Time, error) {
	spec, err := parseCron(expr)
	if err != nil {
		return time.Time{}, err
	}
	next, ok := spec.next(after)
	if !ok {
		return time.Time{}, errBadCron
	}
	return next, nil
}
