// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
)

// AuditSettings holds audit-specific runtime settings.
//
// Forwarding is deliberately absent. It used to carry
// SIEMForwardingEnabled / SyslogForwardingEnabled, one typed field per
// hard-coded transport; forwarding is now the `logging-` plugin family,
// so core cannot name the transports without re-coupling to the two it
// just stopped owning.
//
// The `audit.<type>_forwarding.enabled` rows seeded by migration 122
// still work and are still honoured: LogForwarderWorker reads them per
// forwarder type through ReadAuditSettingBool, which takes an arbitrary
// key. A new forwarder gets the same kill switch for free, and the two
// existing rows keep working untouched.
type AuditSettings struct {
	ClockDriftThresholdSeconds int    `json:"clockDriftThresholdSeconds"`
	ClockReferenceURL          string `json:"clockReferenceUrl"`
	HashChainEnabled           bool   `json:"hashChainEnabled"`
	ActionCatalogEnabled       bool   `json:"actionCatalogEnabled"`
}

// DefaultAuditSettings returns default audit runtime settings.
func DefaultAuditSettings() AuditSettings {
	return AuditSettings{ClockDriftThresholdSeconds: 5, ClockReferenceURL: "https://www.cloudflare.com/", HashChainEnabled: true, ActionCatalogEnabled: true}
}

// ReadAuditSettings reads audit settings from the settings table, falling back to defaults.
func ReadAuditSettings(db *sql.DB) AuditSettings {
	return ReadAuditSettingsContext(context.Background(), db)
}

// ReadAuditSettingsContext is ReadAuditSettings bound to the caller's context, so a cancelled request stops the audit settings read.
func ReadAuditSettingsContext(ctx context.Context, db *sql.DB) AuditSettings {
	s := DefaultAuditSettings()
	if db == nil {
		return s
	}

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'audit'")
	if err != nil {
		return s
	}
	defer closeRows(rows)
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			continue
		}
		switch key {
		case "audit.clock.drift_threshold_seconds":
			if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 && v <= 60 {
				s.ClockDriftThresholdSeconds = v
			}
		case "audit.clock.reference_url":
			if validHTTPURL(raw) {
				s.ClockReferenceURL = strings.TrimSpace(raw)
			}
		case "audit.hash_chain.enabled":
			s.HashChainEnabled = parseBool(raw, s.HashChainEnabled)
		case "audit.action_catalog.enabled":
			s.ActionCatalogEnabled = parseBool(raw, s.ActionCatalogEnabled)
		}
	}
	logRowsErr(rows)
	return s
}

func validHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func parseBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}

// ReadAuditSettingBool returns the boolean audit setting under key, or fallback when it is unset or unreadable.
func ReadAuditSettingBool(db *sql.DB, key string, fallback bool) bool {
	return ReadAuditSettingBoolContext(context.Background(), db, key, fallback)
}

// ReadAuditSettingBoolContext is ReadAuditSettingBool bound to the caller's context.
func ReadAuditSettingBoolContext(ctx context.Context, db *sql.DB, key string, fallback bool) bool {
	if db == nil || strings.TrimSpace(key) == "" {
		return fallback
	}
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&raw)
	if err != nil {
		return fallback
	}
	return parseBool(raw, fallback)
}

// AgentSettings holds configurable agent behavior settings.
type AgentSettings struct {
	EventMode         string `json:"eventMode"`         // "poll" or "stream"
	EventPollInterval int    `json:"eventPollInterval"` // seconds (default 30)
	PingInterval      int    `json:"pingInterval"`      // seconds (default 30)
	MetricsEnabled    bool   `json:"metricsEnabled"`    // default true
	RequestTimeout    int    `json:"requestTimeout"`    // seconds (default 30)
}

// DefaultAgentSettings returns the default agent settings.
func DefaultAgentSettings() AgentSettings {
	return AgentSettings{
		EventMode:         "poll",
		EventPollInterval: 30,
		PingInterval:      30,
		MetricsEnabled:    true,
		RequestTimeout:    30,
	}
}

// ReadAgentSettings reads agent settings from the settings table, falling back to defaults.
func ReadAgentSettings(db *sql.DB) AgentSettings {
	return ReadAgentSettingsContext(context.Background(), db)
}

// ReadAgentSettingsContext is ReadAgentSettings bound to the caller's context.
func ReadAgentSettingsContext(ctx context.Context, db *sql.DB) AgentSettings {
	s := DefaultAgentSettings()

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'agent'")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "agent_event_mode":
			if value == "poll" || value == "stream" {
				s.EventMode = value
			}
		case "agent_event_poll_interval":
			if v, err := strconv.Atoi(value); err == nil && v >= 10 && v <= 300 {
				s.EventPollInterval = v
			}
		case "agent_ping_interval":
			if v, err := strconv.Atoi(value); err == nil && v >= 10 && v <= 120 {
				s.PingInterval = v
			}
		case "agent_metrics_enabled":
			if v, err := strconv.ParseBool(value); err == nil {
				s.MetricsEnabled = v
			}
		case "agent_request_timeout":
			if v, err := strconv.Atoi(value); err == nil && v >= 5 && v <= 120 {
				s.RequestTimeout = v
			}
		}
	}
	logRowsErr(rows)

	return s
}

// ScannerSettings holds the vulnerability-scanning settings that are
// genuinely the host's.
//
// The per-engine flags (trivy/grype/clair enabled), the Clair server
// URL and the default-scanner name all left this struct when the
// engines became plugins. They belonged to a specific engine or to a
// specific *instance* of one, and neither is something a global
// settings row can express once an operator can configure two Clairs.
// They live on `scanner_configs` rows now.
//
// What stayed is what is true regardless of which engine runs: how
// long the host will hold a scan open, and whether it scans
// automatically on install and update.
type ScannerSettings struct {
	ScanTimeout   int  `json:"scanTimeout"` // seconds
	ScanOnInstall bool `json:"scanOnInstall"`
	ScanOnUpdate  bool `json:"scanOnUpdate"`
}

// DefaultScannerSettings returns the default scanner settings.
func DefaultScannerSettings() ScannerSettings {
	return ScannerSettings{
		ScanTimeout:   300,
		ScanOnInstall: false,
		ScanOnUpdate:  false,
	}
}

// RetentionSettings holds configurable data retention settings.
type RetentionSettings struct {
	AuditRetentionDays         int    `json:"auditRetentionDays"`       // 0 = keep forever
	ActivityRetentionDays      int    `json:"activityRetentionDays"`    // 0 = keep forever
	LifecycleRetentionDays     int    `json:"lifecycleRetentionDays"`   // 0 = keep forever
	BackupLogRetentionDays     int    `json:"backupLogRetentionDays"`   // 0 = keep forever
	ScanRetentionDays          int    `json:"scanRetentionDays"`        // 0 = keep forever
	APIKeyUsageRetentionDays   int    `json:"apiKeyUsageRetentionDays"` // 0 = keep forever
	BackupRetentionCount       int    `json:"backupRetentionCount"`     // 0 = keep forever
	BackupRetentionDays        int    `json:"backupRetentionDays"`      // 0 = keep forever
	ActiveAuditRetentionPreset string `json:"activeAuditRetentionPreset"`

	// Resource-metric history is tiered, so it needs two horizons rather than
	// one. MetricsRawHours governs the 30s samples the Overview graphs render;
	// once a 10-minute bucket lies entirely past that line the collector folds
	// it into an average and deletes the raw rows. MetricsRollupDays then
	// governs how long those averages survive. Neither accepts 0 as
	// "keep forever": an unbounded 30s series would grow without limit.
	MetricsRawHours    int  `json:"metricsRawHours"`
	MetricsRollupDays  int  `json:"metricsRollupDays"`
	MetricsDiskEnabled bool `json:"metricsDiskEnabled"`

	// RecordExecEvents controls whether Docker exec events — overwhelmingly
	// healthcheck probes — are written to the event log at all. It lives here
	// rather than under activity settings because it is a "how much data does
	// McHarbor store" question, which is what this group answers.
	//
	// Default false. On a measured install, exec_create/exec_start/exec_die
	// accounted for 95.4% of all stored events (1,384,647 of 1,451,867 rows,
	// ~5.2 GB). Turning it on is supported but genuinely high volume, which is
	// why the UI recommends an external database alongside it. Nothing is lost
	// while it is off: healthcheck results are recorded properly, with exit
	// codes, in container_health_probes.
	RecordExecEvents bool `json:"recordExecEvents"`
}

// DefaultRetentionSettings returns the default retention settings.
func DefaultRetentionSettings() RetentionSettings {
	return RetentionSettings{
		AuditRetentionDays:         90,
		ActivityRetentionDays:      30,
		LifecycleRetentionDays:     30,
		BackupLogRetentionDays:     30,
		ScanRetentionDays:          90,
		APIKeyUsageRetentionDays:   90,
		BackupRetentionCount:       0,
		BackupRetentionDays:        0,
		ActiveAuditRetentionPreset: "custom",
		MetricsRawHours:            24,
		MetricsRollupDays:          30,
		MetricsDiskEnabled:         true,
		RecordExecEvents:           false,
	}
}

// ReadRetentionSettings reads retention settings from the settings table, falling back to defaults.
func ReadRetentionSettings(db *sql.DB) RetentionSettings {
	return ReadRetentionSettingsContext(context.Background(), db)
}

// ReadRetentionSettingsContext is ReadRetentionSettings bound to the caller's context, so a cancelled request stops the retention settings read.
func ReadRetentionSettingsContext(ctx context.Context, db *sql.DB) RetentionSettings {
	s := DefaultRetentionSettings()

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'retention'")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "retention_audit_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.AuditRetentionDays = v
			}
		case "retention_activity_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.ActivityRetentionDays = v
			}
		case "retention_lifecycle_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.LifecycleRetentionDays = v
			}
		case "retention_backup_log_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.BackupLogRetentionDays = v
			}
		case "retention_scan_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.ScanRetentionDays = v
			}
		// Per-API-key request telemetry and the tombstones left behind by
		// deleted keys. The two age from different clocks — events from
		// when the request happened, tombstones from when the key was
		// deleted — but share one horizon, because an operator thinks of
		// them as one thing: how long API key history is kept.
		// 0 = keep forever, as elsewhere in this group.
		case "retention_api_key_usage_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.APIKeyUsageRetentionDays = v
			}
		case "retention_backup_count":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 1000 {
				s.BackupRetentionCount = v
			}
		case "retention_backup_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 3650 {
				s.BackupRetentionDays = v
			}
		case "retention_metrics_raw_hours":
			// Floor of 1h: the compactor can only fold whole buckets, so a
			// sub-hour raw window would leave nothing to average.
			if v, err := strconv.Atoi(value); err == nil && v >= 1 && v <= 8760 {
				s.MetricsRawHours = v
			}
		case "retention_metrics_rollup_days":
			if v, err := strconv.Atoi(value); err == nil && v >= 1 && v <= 3650 {
				s.MetricsRollupDays = v
			}
		case "retention_metrics_disk_enabled":
			s.MetricsDiskEnabled = parseBool(value, s.MetricsDiskEnabled)
		case "retention_record_exec_events":
			s.RecordExecEvents = parseBool(value, s.RecordExecEvents)
		case "retention_audit_preset":
			value = strings.TrimSpace(value)
			if value != "" {
				s.ActiveAuditRetentionPreset = value
			}
		}
	}
	logRowsErr(rows)

	return s
}

// ReadScannerSettings reads scanner settings from the settings table, falling back to defaults.
func ReadScannerSettings(db *sql.DB) ScannerSettings {
	return ReadScannerSettingsContext(context.Background(), db)
}

// ReadScannerSettingsContext is ReadScannerSettings bound to the caller's context, so a cancelled request stops the scanner settings read.
func ReadScannerSettingsContext(ctx context.Context, db *sql.DB) ScannerSettings {
	s := DefaultScannerSettings()

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'scanners'")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		// The scanner_trivy_enabled / scanner_grype_enabled /
		// scanner_clair_enabled / scanner_clair_url / scanner_default
		// rows are deliberately not read any more. Migration 080
		// copied them into `scanner_configs` and left the rows in
		// place so a rollback finds them intact; reading them here
		// would let a stale flag override the operator's current
		// choice.
		case "scanner_timeout":
			if v, err := strconv.Atoi(value); err == nil && v >= 30 && v <= 1800 {
				s.ScanTimeout = v
			}
		case "scanner_scan_on_install":
			s.ScanOnInstall = value == "true"
		case "scanner_scan_on_update":
			s.ScanOnUpdate = value == "true"
		}
	}
	logRowsErr(rows)

	return s
}

// ─── Toasts ───────────────────────────────────────────────────────────

// ToastSettings is the app-wide default for the transient status messages
// the UI raises after a mutation. It lives in settings rather than in a
// user column because an operator needs one answer for a fresh install and
// for every account that never opens Settings.
//
// AllowUserOverride decides whether a signed-in user may replace these with
// their own. Turning it off does not discard what people already chose — the
// per-user columns stay on `users` and take effect again the moment it is
// turned back on.
type ToastSettings struct {
	// Position is one of the six slots sonner supports:
	// top-left, top-center, top-right, bottom-left, bottom-center, bottom-right.
	Position string `json:"position"`
	// DurationMs is how long a toast stays before it dismisses itself.
	// 0 means "never auto-dismiss" — the close button then does the work,
	// which is why CloseButton is forced on for that case in the UI.
	DurationMs int `json:"durationMs"`
	// MaxVisible is how many toasts stack before older ones collapse.
	MaxVisible        int  `json:"maxVisible"`
	RichColors        bool `json:"richColors"`
	CloseButton       bool `json:"closeButton"`
	AllowUserOverride bool `json:"allowUserOverride"`
}

// ToastPositions is the set of positions sonner renders. Anything outside it
// is rejected on write and ignored on read, so a hand-edited settings row can
// never put the toaster somewhere the frontend cannot place it.
var ToastPositions = []string{
	"top-left", "top-center", "top-right",
	"bottom-left", "bottom-center", "bottom-right",
}

// ValidToastPosition reports whether p is one of ToastPositions.
func ValidToastPosition(p string) bool {
	for _, valid := range ToastPositions {
		if p == valid {
			return true
		}
	}
	return false
}

// Bounds for the numeric knobs, shared by the reader and the writer so a
// value that survives a write is always a value the reader will accept back.
const (
	ToastMinDurationMs = 0
	ToastMaxDurationMs = 60000
	ToastMinVisible    = 1
	ToastMaxVisible    = 10
)

// DefaultToastSettings returns the shipped defaults, which are exactly what
// main.tsx hardcoded before this became configurable: bottom-right, sonner's
// own 4s dismissal, three stacked, rich colors and a close button on.
func DefaultToastSettings() ToastSettings {
	return ToastSettings{
		Position:          "bottom-right",
		DurationMs:        4000,
		MaxVisible:        3,
		RichColors:        true,
		CloseButton:       true,
		AllowUserOverride: true,
	}
}

// ReadToastSettings reads the app-wide toast defaults, falling back to
// DefaultToastSettings for any row that is missing or out of range.
func ReadToastSettings(db *sql.DB) ToastSettings {
	return ReadToastSettingsContext(context.Background(), db)
}

// ReadToastSettingsContext is ReadToastSettings bound to the caller's context, so a cancelled request stops the toast settings read.
func ReadToastSettingsContext(ctx context.Context, db *sql.DB) ToastSettings {
	s := DefaultToastSettings()

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'toast'")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "toast_position":
			if ValidToastPosition(value) {
				s.Position = value
			}
		case "toast_duration_ms":
			if v, err := strconv.Atoi(value); err == nil && v >= ToastMinDurationMs && v <= ToastMaxDurationMs {
				s.DurationMs = v
			}
		case "toast_max_visible":
			if v, err := strconv.Atoi(value); err == nil && v >= ToastMinVisible && v <= ToastMaxVisible {
				s.MaxVisible = v
			}
		case "toast_rich_colors":
			s.RichColors = value == "true"
		case "toast_close_button":
			s.CloseButton = value == "true"
		case "toast_allow_user_override":
			s.AllowUserOverride = value == "true"
		}
	}
	logRowsErr(rows)

	return s
}

// ─── Appearance ───────────────────────────────────────────────────────

// AppearanceSettings is the app-wide default look: which theme a fresh
// account renders, which accent overrides that theme's primary, which
// interface font it sets, and the root font size every relative size in the
// app derives from.
//
// It lives in settings rather than only in a user column for the same reason
// the toast defaults do: an operator needs one answer for a fresh install and
// for every account that never opens Profile.
//
// AllowUserOverride decides whether a signed-in user may replace these with
// their own. Turning it off does not discard what people already chose — the
// per-user columns stay on `users` and take effect again the moment it is
// turned back on.
type AppearanceSettings struct {
	// ThemeID is an id from the frontend theme manifest, or a
	// `plugin:<slug>` id registered by a loaded plugin. It is not
	// validated against a fixed list here — plugins add themes at runtime,
	// so the backend only checks that it is a plausible id and lets the
	// frontend fall back to its default when the theme is gone.
	ThemeID string `json:"themeId"`
	// Accent replaces --primary across the app. "theme" means "leave the
	// theme's own primary alone", which is the default.
	Accent string `json:"accent"`
	// Font is the interface font id, one of AppearanceFonts.
	Font string `json:"font"`
	// BaseFontPx is the root font size in CSS pixels. Every relative size
	// in the app (text-xs through text-2xl, spacing, radii) is expressed in
	// rem and therefore derives from this one number.
	BaseFontPx        int  `json:"baseFontPx"`
	AllowUserOverride bool `json:"allowUserOverride"`
}

// AppearanceAccents is the set of accents the picker offers. "theme" is the
// no-op that hands --primary back to the active theme.
var AppearanceAccents = []string{
	"theme", "blue", "green", "purple", "orange", "rose", "cyan",
}

// AppearanceFonts is the set of interface fonts the picker offers. The first
// three are the faces McHarbor self-hosts under public/fonts; the rest are
// OS-native stacks that need no download. McHarbor is frequently deployed
// air-gapped, so nothing here reaches a CDN.
var AppearanceFonts = []string{
	// Self-hosted under public/fonts.
	"inter-tight", "bricolage", "jetbrains-mono",
	// OS-native stacks.
	"system", "serif", "humanist",
	// Fetched from Google's CDN by the browser when it is online. The
	// backend only validates the id — it never reaches the network itself,
	// and every one of these falls back to a bundled face client-side.
	"inter", "roboto", "open-sans", "lato", "montserrat", "poppins",
	"nunito-sans", "source-sans-3", "work-sans", "manrope",
	"plus-jakarta-sans", "dm-sans", "ibm-plex-sans", "atkinson-hyperlegible",
	"source-serif-4", "merriweather", "lora",
	"roboto-mono", "ibm-plex-mono", "fira-code",
}

// Bounds for the root font size, shared by the reader and the writer so a
// value that survives a write is always a value the reader will accept back.
// 12px is the smallest size the tabular figures in the metrics tables stay
// legible at; 20px is where the sidebar stops fitting its labels on one line.
const (
	AppearanceMinBaseFontPx = 12
	AppearanceMaxBaseFontPx = 20
	// appearanceMaxThemeIDLen bounds the free-form theme id. Plugin ids are
	// `plugin:<slug>`, and no slug in the catalog comes close to this.
	appearanceMaxThemeIDLen = 64
)

// ValidAppearanceAccent reports whether a is one of AppearanceAccents.
func ValidAppearanceAccent(a string) bool {
	for _, valid := range AppearanceAccents {
		if a == valid {
			return true
		}
	}
	return false
}

// ValidAppearanceFont reports whether f is one of AppearanceFonts.
func ValidAppearanceFont(f string) bool {
	for _, valid := range AppearanceFonts {
		if f == valid {
			return true
		}
	}
	return false
}

// ValidAppearanceThemeID reports whether id is a plausible theme id. The set
// is open — plugins register themes at runtime — so this is a shape check,
// not a membership test: non-empty, bounded, and made of the characters the
// manifest and the `plugin:<slug>` convention actually use.
func ValidAppearanceThemeID(id string) bool {
	if id == "" || len(id) > appearanceMaxThemeIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == ':', r == '.':
		default:
			return false
		}
	}
	return true
}

// DefaultAppearanceSettings returns the shipped defaults, which are exactly
// what the frontend hardcoded before this became configurable: the McHarbor
// theme, its own primary, Inter Tight, and the browser's usual 16px root.
func DefaultAppearanceSettings() AppearanceSettings {
	return AppearanceSettings{
		ThemeID:           "mcharbor-default",
		Accent:            "theme",
		Font:              "inter-tight",
		BaseFontPx:        16,
		AllowUserOverride: true,
	}
}

// ReadAppearanceSettings reads the app-wide appearance defaults, falling back
// to DefaultAppearanceSettings for any row that is missing or out of range.
func ReadAppearanceSettings(db *sql.DB) AppearanceSettings {
	return ReadAppearanceSettingsContext(context.Background(), db)
}

// ReadAppearanceSettingsContext is ReadAppearanceSettings bound to the caller's context, so a cancelled request stops the appearance settings read.
func ReadAppearanceSettingsContext(ctx context.Context, db *sql.DB) AppearanceSettings {
	s := DefaultAppearanceSettings()

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE category = 'appearance'")
	if err != nil {
		return s
	}
	defer closeRows(rows)

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "appearance_theme_id":
			if ValidAppearanceThemeID(value) {
				s.ThemeID = value
			}
		case "appearance_accent":
			if ValidAppearanceAccent(value) {
				s.Accent = value
			}
		case "appearance_font":
			if ValidAppearanceFont(value) {
				s.Font = value
			}
		case "appearance_base_font_px":
			if v, err := strconv.Atoi(value); err == nil &&
				v >= AppearanceMinBaseFontPx && v <= AppearanceMaxBaseFontPx {
				s.BaseFontPx = v
			}
		case "appearance_allow_user_override":
			s.AllowUserOverride = value == "true"
		}
	}
	logRowsErr(rows)

	return s
}

// ClampAppearanceBaseFontPx pins a root font size into the supported range.
// Used on the per-user write path, where an out-of-range value is a stale
// client rather than a reason to reject the whole preferences update.
func ClampAppearanceBaseFontPx(px int) int {
	if px < AppearanceMinBaseFontPx {
		return AppearanceMinBaseFontPx
	}
	if px > AppearanceMaxBaseFontPx {
		return AppearanceMaxBaseFontPx
	}
	return px
}

// closeRows releases a result set; a close failure cannot change the value
// already read, so it is logged rather than returned.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Debug("settings: closing rows failed", "error", err)
	}
}

// logRowsErr records an iteration error after a settings scan; the defaults
// already in place stay authoritative, so the read degrades instead of failing.
func logRowsErr(rows *sql.Rows) {
	if err := rows.Err(); err != nil {
		slog.Debug("settings: iterating rows failed", "error", err)
	}
}
