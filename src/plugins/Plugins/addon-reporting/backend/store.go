// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// store owns everything the plugin persists.
//
// The host fences a plugin's DB handle to tables prefixed
// `plugin_<slug>_`, and that fence is right: a plugin has no business
// reading the host's tables, and everything a report needs comes through
// the host's own API where RBAC still applies. Nothing here touches a
// table outside plugin_reporting_*.
type store struct {
	db plugin.DBHandle
}

var errNotFound = errors.New("reporting: report not found")

// StoredReport is a row in the library.
type StoredReport struct {
	ID            string    `json:"id"`
	ReportType    string    `json:"report_type"`
	Title         string    `json:"title"`
	EnvironmentID string    `json:"environment_id"`
	Environment   string    `json:"environment"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	Levels        []Level   `json:"levels"`
	Status        string    `json:"status"`
	OverallScore  int       `json:"overall_score"`
	OverallBand   Band      `json:"overall_band"`
	Headline      string    `json:"headline"`
	GeneratedBy   string    `json:"generated_by"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`

	// Prompt is the question that produced an ad-hoc report. Empty for
	// every other type.
	Prompt string `json:"prompt,omitempty"`
	// Plan is the pinned pass-1 result, replayed on regenerate.
	Plan json.RawMessage `json:"plan,omitempty"`
	// AIConnectionID records which configured AI connection generated
	// (ad-hoc) or polished (AINarrative) this report, so the library can
	// show which AI wrote it. Empty when no AI call was made.
	AIConnectionID string `json:"ai_connection_id,omitempty"`

	// Document is only populated by Get; the list view omits it so a
	// library of a hundred reports is not a multi-megabyte response.
	Document *Document `json:"document,omitempty"`
}

// schemaLogger is the minimal logging surface ensureSchema needs. It
// mirrors host.Logger's Info method rather than importing the whole
// plugin.HostContext, so store.go stays decoupled from init wiring —
// any value with an Info(msg string, args ...any) method satisfies it,
// which host.Logger already does.
type schemaLogger interface {
	Info(msg string, args ...any)
}

// reportSchemaColumn is one column ensureSchema knows how to add if it
// is missing, and the exact DDL to add it with. Kept in the same order
// they were introduced so a log line about a missing column reads in a
// stable, meaningful order.
type reportSchemaColumn struct {
	name string
	ddl  string
}

// reportSchemaColumns are the columns added after plugin_reporting_reports
// was first created by migrations/0001_init.sql. Each entry here should
// have a matching ALTER TABLE statement in the migration file that
// introduced it (migrations/0002_adhoc.sql for these two) — ensureSchema
// is a second, idempotent route to the same end state, not a
// replacement for the migration.
var reportSchemaColumns = []reportSchemaColumn{
	{name: "prompt", ddl: `ALTER TABLE plugin_reporting_reports ADD COLUMN prompt TEXT NOT NULL DEFAULT ''`},
	{name: "plan", ddl: `ALTER TABLE plugin_reporting_reports ADD COLUMN plan TEXT NOT NULL DEFAULT ''`},
	// Matching migrations/0003_ai_connection.sql.
	{name: "ai_connection_id", ddl: `ALTER TABLE plugin_reporting_reports ADD COLUMN ai_connection_id TEXT NOT NULL DEFAULT ''`},
}

// ensureSchema self-heals plugin_reporting_reports against the columns
// in reportSchemaColumns, adding whichever ones are missing.
//
// Why this exists: the host only auto-applies a plugin's
// migrations/*.sql on a fresh install (see runMigrations in
// modules/appstore/plugins/installer_default.go); an already-installed
// plugin that upgrades to a build carrying a new migration file does
// not get it re-run unless the operator has opted into
// MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS. store.insert is the single
// write path for every report type, so leaving that timing gap
// unaddressed would take down report generation for the whole plugin,
// not just the ad-hoc type that motivated the new columns. The plugin
// owns its schema and should not assume someone else ran its DDL.
//
// Safe to call on every boot: a fully migrated table costs one PRAGMA
// query and adds nothing. SQLite's ALTER TABLE ADD COLUMN is not
// idempotent (it errors if the column exists), so this checks
// PRAGMA table_info first and only adds what is genuinely missing.
//
// If the table itself does not exist yet (fresh-install ordering,
// before migrations/0001_init.sql has run), PRAGMA table_info returns
// zero rows rather than an error — this is treated as a no-op, since
// table creation belongs to the migration path, not to this repair.
func (s *store) ensureSchema(ctx context.Context, log schemaLogger) error {
	rows, err := s.db.Query(ctx, `PRAGMA table_info(plugin_reporting_reports)`)
	if err != nil {
		return fmt.Errorf("reporting: inspect schema: %w", err)
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
			return fmt.Errorf("reporting: scan schema column: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("reporting: read schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("reporting: close schema query: %w", err)
	}
	if len(existing) == 0 {
		// Table not created yet: a fresh install still mid-migration.
		// migrations/0001_init.sql owns table creation.
		//
		// The schedule tables are still ensured below: they are created
		// with `IF NOT EXISTS` rather than altered, so unlike the columns
		// above they do not depend on the migration having run first.
		return s.ensureScheduleSchema(ctx)
	}
	for _, col := range reportSchemaColumns {
		if existing[col.name] {
			continue
		}
		if _, err := s.db.Exec(ctx, col.ddl); err != nil {
			return fmt.Errorf("reporting: add column %s: %w", col.name, err)
		}
		if log != nil {
			log.Info("reporting: added missing column to plugin_reporting_reports", "column", col.name)
		}
	}
	// Same reasoning one level up: migrations/0004_schedules.sql only runs
	// on a fresh install, and every write path for schedules would fail on
	// an upgraded one without this.
	return s.ensureScheduleSchema(ctx)
}

func (s *store) insert(ctx context.Context, r StoredReport, doc *Document) error {
	payload, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("reporting: encode document: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(ctx, `
		INSERT INTO plugin_reporting_reports
			(id, report_type, title, environment_id, environment, period_start, period_end,
			 levels, status, overall_score, overall_band, headline, document,
			 generated_by, size_bytes, created_at, updated_at, prompt, plan, ai_connection_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ReportType, r.Title, r.EnvironmentID, r.Environment,
		r.PeriodStart.UTC().Format(time.RFC3339), r.PeriodEnd.UTC().Format(time.RFC3339),
		levelsCSV(r.Levels), r.Status, r.OverallScore, string(r.OverallBand), r.Headline,
		string(payload), r.GeneratedBy, int64(len(payload)), now, now, r.Prompt, string(r.Plan),
		r.AIConnectionID,
	)
	return err
}

const listColumns = `id, report_type, title, environment_id, environment, period_start,
	period_end, levels, status, overall_score, overall_band, headline,
	generated_by, size_bytes, created_at, prompt, ai_connection_id`

func (s *store) list(ctx context.Context, reportType string, limit, offset int) ([]StoredReport, error) {
	query := `SELECT ` + listColumns + ` FROM plugin_reporting_reports`
	args := []any{}
	if reportType != "" {
		query += ` WHERE report_type = ?`
		args = append(args, reportType)
	}
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []StoredReport
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *store) count(ctx context.Context, reportType string) (int, error) {
	query := `SELECT COUNT(*) FROM plugin_reporting_reports`
	args := []any{}
	if reportType != "" {
		query += ` WHERE report_type = ?`
		args = append(args, reportType)
	}
	rows, err := s.db.Query(ctx, query, args...)
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

func (s *store) get(ctx context.Context, id string) (StoredReport, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+listColumns+`, document, plan FROM plugin_reporting_reports WHERE id = ?`, id)
	if err != nil {
		return StoredReport{}, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return StoredReport{}, err
		}
		return StoredReport{}, errNotFound
	}

	var (
		r                      StoredReport
		periodStart, periodEnd string
		levels, band, created  string
		payload                string
		plan                   string
	)
	if err := rows.Scan(&r.ID, &r.ReportType, &r.Title, &r.EnvironmentID, &r.Environment,
		&periodStart, &periodEnd, &levels, &r.Status, &r.OverallScore, &band,
		&r.Headline, &r.GeneratedBy, &r.SizeBytes, &created, &r.Prompt, &r.AIConnectionID,
		&payload, &plan); err != nil {
		return StoredReport{}, err
	}
	hydrate(&r, periodStart, periodEnd, levels, band, created)
	if plan != "" {
		r.Plan = json.RawMessage(plan)
	}

	var doc Document
	if err := json.Unmarshal([]byte(payload), &doc); err != nil {
		return StoredReport{}, fmt.Errorf("reporting: decode stored document: %w", err)
	}
	r.Document = &doc
	return r, nil
}

func (s *store) delete(ctx context.Context, id string) (bool, error) {
	n, err := s.db.Exec(ctx, `DELETE FROM plugin_reporting_reports WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// prune keeps the library from growing without bound. Reports are cheap
// to regenerate and expensive to store — each carries its full rendered
// document — so an install that generates one a day does not accumulate
// forever.
func (s *store) prune(ctx context.Context, keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM plugin_reporting_reports
		WHERE id NOT IN (
			SELECT id FROM plugin_reporting_reports ORDER BY created_at DESC LIMIT ?
		)`, keep)
	return err
}

func scanReport(rows plugin.Rows) (StoredReport, error) {
	var (
		r                      StoredReport
		periodStart, periodEnd string
		levels, band, created  string
	)
	if err := rows.Scan(&r.ID, &r.ReportType, &r.Title, &r.EnvironmentID, &r.Environment,
		&periodStart, &periodEnd, &levels, &r.Status, &r.OverallScore, &band,
		&r.Headline, &r.GeneratedBy, &r.SizeBytes, &created, &r.Prompt, &r.AIConnectionID); err != nil {
		return StoredReport{}, err
	}
	hydrate(&r, periodStart, periodEnd, levels, band, created)
	return r, nil
}

func hydrate(r *StoredReport, periodStart, periodEnd, levels, band, created string) {
	r.PeriodStart = parseTS(periodStart)
	r.PeriodEnd = parseTS(periodEnd)
	r.CreatedAt = parseTS(created)
	r.OverallBand = Band(band)
	r.Levels = parseLevels(levels)
}

func levelsCSV(levels []Level) string {
	parts := make([]string, 0, len(levels))
	for _, l := range levels {
		parts = append(parts, string(l))
	}
	return strings.Join(parts, ",")
}

func parseLevels(csv string) []Level {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]Level, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, Level(p))
		}
	}
	return out
}

// --- settings --------------------------------------------------------

// Settings are the operator-controlled defaults every report inherits.
type Settings struct {
	Organisation   string `json:"organisation"`
	Classification string `json:"classification"`
	DefaultDays    int    `json:"default_days"`
	RetainReports  int    `json:"retain_reports"`
	// AINarrative lets the host's AI router improve the wording of the
	// opening paragraph. Off by default: a board report has to be
	// reproducible, and the deterministic text always is.
	AINarrative bool `json:"ai_narrative"`

	// AIReports lets a model author a whole document for the ad-hoc
	// report type. Deliberately separate from AINarrative and off by
	// default: polishing one sentence and authoring a board document are
	// different risk classes, and an operator who consented to the first
	// has not consented to the second. Enabling it sends estate detail
	// to the configured AI provider.
	AIReports bool `json:"ai_reports"`

	// PageSize is the paper every export is laid out on: "a4" or "legal".
	// It belongs to the operator rather than to the person downloading,
	// because a report that leaves the building should leave it on the
	// same sheet every time.
	PageSize string `json:"page_size"`

	// LogoPNG and IconPNG are the operator's own marks, base64-encoded
	// PNG with no data: prefix. Empty means "use the built-in mark" —
	// assets/img/logo-wordmark.svg and logo-mark.svg, compiled in as PNG.
	//
	// PNG rather than SVG because both exporters embed raster images and
	// neither can rasterise vector artwork; the settings page converts an
	// uploaded SVG in the browser before it ever reaches here, so the
	// operator still gets to pick either format.
	LogoPNG string `json:"logo_png"`
	IconPNG string `json:"icon_png"`
}

func defaultSettings() Settings {
	return Settings{
		Organisation:   "McHarbor",
		Classification: "Internal",
		DefaultDays:    30,
		RetainReports:  100,
		AINarrative:    false,
		AIReports:      false,
		PageSize:       pageA4.Key,
	}
}

const settingsKey = "settings"

func (s *store) settings(ctx context.Context, kv plugin.KVStore) Settings {
	out := defaultSettings()
	raw, ok, err := kv.Get(ctx, settingsKey)
	if err != nil || !ok || raw == "" {
		return out
	}
	// A malformed value falls back to defaults rather than failing the
	// request: a report is more useful than an error about its footer.
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return defaultSettings()
	}
	return out.sanitised()
}

func (s *store) saveSettings(ctx context.Context, kv plugin.KVStore, in Settings) (Settings, error) {
	in = in.sanitised()
	raw, err := json.Marshal(in)
	if err != nil {
		return Settings{}, err
	}
	if err := kv.Set(ctx, settingsKey, string(raw)); err != nil {
		return Settings{}, err
	}
	return in, nil
}

// sanitised clamps operator input to sane bounds. These values end up on
// the cover of a document that leaves the building, so they are length
// limited and stripped of control characters.
func (s Settings) sanitised() Settings {
	s.Organisation = clampText(s.Organisation, 80, "McHarbor")
	s.Classification = clampText(s.Classification, 40, "Internal")
	if s.DefaultDays < 1 || s.DefaultDays > 365 {
		s.DefaultDays = 30
	}
	if s.RetainReports < 10 || s.RetainReports > 1000 {
		s.RetainReports = 100
	}
	// An empty or unrecognised key resolves to A4, so a settings row
	// written before this field existed keeps working.
	paper, _ := pageSizeByKey(s.PageSize)
	if paper.Key == "" {
		paper = pageA4
	}
	s.PageSize = paper.Key
	s.LogoPNG = clampMark(s.LogoPNG)
	s.IconPNG = clampMark(s.IconPNG)
	return s
}

// withoutMarks returns the settings with the two brand marks cleared, for
// responses that carry settings as context rather than as the thing being
// edited. They are megabytes of base64 nobody on that path draws.
func (s Settings) withoutMarks() Settings {
	s.LogoPNG, s.IconPNG = "", ""
	return s
}

// Mark limits. A cover logo is decorative, so there is no reason for one
// to be larger than a page at print resolution — and the blob lives in a
// settings row that is read on every catalog request.
const (
	maxMarkBytes = 2 << 20 // 2 MiB decoded
	maxMarkPx    = 4000
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}

// clampMark accepts a base64 PNG or nothing at all.
//
// Anything that fails a check is dropped rather than rejected: the mark is
// the least important thing on the page, and refusing to save an
// organisation name because a logo was malformed would be the wrong trade.
// The settings page validates before it uploads, so a drop here means
// something reached the API that did not come from that form.
func clampMark(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// Tolerate a data: prefix so a hand-made API call behaves.
	if i := strings.Index(v, ";base64,"); i >= 0 {
		v = v[i+len(";base64,"):]
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil || len(raw) > maxMarkBytes || !bytes.HasPrefix(raw, pngMagic) {
		return ""
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > maxMarkPx || cfg.Height > maxMarkPx {
		return ""
	}
	return v
}

// markPNG decodes a stored mark back to PNG bytes plus its pixel size.
// A stored value has already passed clampMark, so a failure here can only
// mean the row was edited outside the API; the caller falls back to the
// built-in mark either way.
func markPNG(v string) (data []byte, w, h int, ok bool) {
	if v == "" {
		return nil, 0, 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, 0, 0, false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, false
	}
	return raw, cfg.Width, cfg.Height, true
}

func clampText(v string, max int, fallback string) string {
	v = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, v))
	if v == "" {
		return fallback
	}
	r := []rune(v)
	if len(r) > max {
		return string(r[:max])
	}
	return v
}
