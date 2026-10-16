// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/tlsconfig"
)

// Setting represents a key-value setting.
type Setting struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Category  string `json:"category"`
	UpdatedAt string `json:"updatedAt"`
}

func (h *Handler) logAuditWithDiff(r *http.Request, entry audit.Entry, before, after any) {
	before, after = redactSensitiveSnapshot(before), redactSensitiveSnapshot(after)
	if diffEntry, err := entry.WithDiff(before, after); err == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("settings: audit diff error", "error", err, "entity", entry.EntityName)
	}
	h.app.AuditLog.Log(r, entry)
}

// sensitiveValueMarker replaces a sensitive setting's value in an audit diff: the trail
// records that the key changed, never the key material itself.
const sensitiveValueMarker = "[redacted]"

// redactSensitiveSnapshot blanks the value of any sensitive setting inside an audit
// snapshot, whether it is one Setting or the bulk path's map of them.
func redactSensitiveSnapshot(snapshot any) any {
	switch v := snapshot.(type) {
	case Setting:
		if isSensitiveKey(v.Key) && v.Value != "" {
			v.Value = sensitiveValueMarker
		}
		return v
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				item = redactSensitiveSnapshot(item)
			}
			out[key] = item
		}
		return out
	}
	return snapshot
}

func (h *Handler) settingSnapshot(ctx context.Context, key string) any {
	setting, err := h.service.ByKey(ctx, key)
	if err == nil {
		return setting
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	h.app.Logger.Error("settings: audit snapshot error", "error", err, "key", key)
	return nil
}

func (h *Handler) settingsSnapshot(ctx context.Context, settings []SettingInput) map[string]any {
	snapshot := make(map[string]any, len(settings))
	for _, setting := range settings {
		if setting.Key == "" {
			continue
		}
		snapshot[setting.Key] = h.settingSnapshot(ctx, setting.Key)
	}
	return snapshot
}

// validateSettingValue range-checks the handful of settings keys whose
// values can lock the product. Everything else is a free-form string by
// design — this is a key/value bag, not a schema — so an unknown key is
// always accepted.
//
// Returns the i18n code to answer with when the value is rejected.
func validateSettingValue(key, value string) (i18n.MsgCode, bool) {
	switch key {
	case auth.SettingSessionIdleTimeout:
		// Without this check a typo could set a two-second window and
		// sign everyone out of the app, including whoever would have to
		// go back in and fix it. 0 is the explicit "disabled" value.
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || seconds < 0 {
			return i18n.ErrSettingsIdleTimeoutInvalid, false
		}
		if seconds > 0 && (seconds < auth.MinIdleTimeoutSeconds || seconds > auth.MaxIdleTimeoutSeconds) {
			return i18n.ErrSettingsIdleTimeoutInvalid, false
		}
	case rbac.SettingMFARequired:
		// A typo here would read as "off" at runtime, so a policy the operator
		// believes is on would silently not be.
		if _, ok := rbac.ParseMFARequirement(value); !ok {
			return i18n.ErrSettingsMFARequirementInvalid, false
		}
	}
	return "", true
}

// BulkUpdateInput is the request body for bulk-updating settings.
type BulkUpdateInput struct {
	Settings []SettingInput `json:"settings"`
}

// SettingInput represents a single setting key-value pair for create/update.
type SettingInput struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Category string `json:"category"`
}

// sensitiveKeys are setting keys that must never be returned via list or get-by-key endpoints.
// They are also refused by the generic write routes: each has a dedicated writer that
// encrypts it (TLS upload, push key rotation), which a raw write would bypass.
var sensitiveKeys = map[string]struct{}{
	"tls_cert":              {},
	"tls_key":               {},
	"webpush_vapid_private": {},
}

// isSensitiveKey reports whether key is one of sensitiveKeys.
func isSensitiveKey(key string) bool {
	_, blocked := sensitiveKeys[key]
	return blocked
}

// --- Agent Settings ---

// AgentSettingsInput is the request body for updating agent settings.
type AgentSettingsInput struct {
	EventMode         string `json:"eventMode"`
	EventPollInterval int    `json:"eventPollInterval"`
	PingInterval      int    `json:"pingInterval"`
	MetricsEnabled    bool   `json:"metricsEnabled"`
	RequestTimeout    int    `json:"requestTimeout"`
	// StatsPushInterval is seconds between an agent's stats pushes. Zero keeps
	// the stored value, so a client that predates the field changes nothing.
	StatsPushInterval int `json:"statsPushInterval"`
}

// --- Scanner Settings ---

// ScannerSettingsInput is the request body for updating scanner
// settings.
//
// Which engines are enabled, the Clair URL and the default scanner are
// no longer here: they are per-instance properties of a
// `scanner_configs` row and are edited through /scans/configs. What
// remains applies to every engine equally.
type ScannerSettingsInput struct {
	ScanTimeout   int  `json:"scanTimeout"`
	ScanOnInstall bool `json:"scanOnInstall"`
	ScanOnUpdate  bool `json:"scanOnUpdate"`
}

// --- Retention Settings ---

// RetentionSettingsInput is the request body for updating retention settings.
type RetentionSettingsInput struct {
	AuditRetentionDays         int    `json:"auditRetentionDays"`
	ActivityRetentionDays      int    `json:"activityRetentionDays"`
	LifecycleRetentionDays     int    `json:"lifecycleRetentionDays"`
	BackupLogRetentionDays     int    `json:"backupLogRetentionDays"`
	ScanRetentionDays          int    `json:"scanRetentionDays"`
	BackupRetentionCount       int    `json:"backupRetentionCount"`
	BackupRetentionDays        int    `json:"backupRetentionDays"`
	ActiveAuditRetentionPreset string `json:"activeAuditRetentionPreset"`

	// These three arrived after the original payload shape, so a client that
	// predates them omits the keys. Zero is therefore read as "not supplied"
	// and the stored value is kept — see UpdateRetentionSettings. The bool is
	// a pointer for the same reason: false is a legitimate choice and cannot
	// be told apart from an omitted key on a plain bool.
	MetricsRawHours    int   `json:"metricsRawHours"`
	MetricsRollupDays  int   `json:"metricsRollupDays"`
	MetricsDiskEnabled *bool `json:"metricsDiskEnabled"`
	RecordExecEvents   *bool `json:"recordExecEvents"`

	// A pointer for the same reason the bools above are: 0 is a legitimate
	// choice here — it means keep forever — so an omitted key must not be
	// read as a request to disable retention. A client that predates this
	// field keeps whatever is stored.
	APIKeyUsageRetentionDays *int `json:"apiKeyUsageRetentionDays"`
}

// ToastSettingsInput is the request body for PUT /settings/toast. Every
// field is required: the form posts all six on every save, so there is
// nothing to merge and a missing field is a malformed request rather than
// an intent to keep what is stored.
type ToastSettingsInput struct {
	Position          string `json:"position"`
	DurationMs        int    `json:"durationMs"`
	MaxVisible        int    `json:"maxVisible"`
	RichColors        bool   `json:"richColors"`
	CloseButton       bool   `json:"closeButton"`
	AllowUserOverride bool   `json:"allowUserOverride"`
}

// RetentionSettingsResponse is the response body for retention settings.
type RetentionSettingsResponse struct {
	coreSettings.RetentionSettings
	AuditRetentionPresets []AuditRetentionPreset `json:"auditRetentionPresets"`
}

// AuditRetentionPreset describes a selectable audit-log retention preset.
type AuditRetentionPreset struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Framework     string `json:"framework"`
	RetentionDays int    `json:"retentionDays"`
	Description   string `json:"description"`
	IsCustom      bool   `json:"isCustom"`
	SortOrder     int    `json:"sortOrder"`
}

// --- TLS types ---

// TLSStatus is the response shape for TLS configuration. Phase 7
// (default-HTTPS) extends the payload with certSource, tlsMinVersion,
// hstsMaxAge, and transferPort so the UI can render the new banner
// and info rows without a second config round-trip.
type TLSStatus struct {
	Enabled    bool      `json:"enabled"`
	ForceHttps bool      `json:"forceHttps"`
	HasCert    bool      `json:"hasCert"`
	CertInfo   *CertInfo `json:"certInfo,omitempty"`
	// CertSource is "self-signed" when the running listener is
	// using the auto-generated first-boot cert, "user-uploaded"
	// when the operator pushed a custom cert via this UI.
	CertSource string `json:"certSource,omitempty"`
	// TLSMinVersion is the floor TLS version enforced by the
	// listeners (12 = TLS 1.2, 13 = TLS 1.3).
	TLSMinVersion uint16 `json:"tlsMinVersion,omitempty"`
	// HSTSMaxAge is the Strict-Transport-Security max-age in
	// seconds. 0 (the default) means HSTS is disabled.
	HSTSMaxAge int `json:"hstsMaxAge,omitempty"`
	// TransferPort is the listener port for the agent file-
	// transfer helper. Surfaced so operators can confirm the
	// host matches their compose port mapping.
	TransferPort int `json:"transferPort,omitempty"`
	// HasCA reports whether a local root CA exists on disk, which is
	// what makes the "download and trust" flow available at all.
	HasCA bool `json:"hasCa"`
	// CAFingerprint is the root's SHA-256, the value an operator
	// compares against their OS trust store after installing it.
	CAFingerprint string `json:"caFingerprint,omitempty"`
	// CANotAfter is the root's expiry in RFC3339. A lapsed root breaks
	// every client at once, so it is surfaced well before that.
	CANotAfter string `json:"caNotAfter,omitempty"`
}

// CertInfo contains parsed certificate metadata (never raw PEM).
type CertInfo struct {
	Subject      string   `json:"subject"`
	Issuer       string   `json:"issuer"`
	NotBefore    string   `json:"notBefore"`
	NotAfter     string   `json:"notAfter"`
	SerialNumber string   `json:"serialNumber"`
	DNSNames     []string `json:"dnsNames"`
	// Fingerprint is the SHA-256 over the DER, colon-separated — the
	// only field a browser's certificate viewer can be checked against,
	// and therefore the one that turns "click through" into a real
	// verification step.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// TLSUpdateRequest is the request body for updating TLS settings.
type TLSUpdateRequest struct {
	Cert       *string `json:"cert,omitempty"`
	Key        *string `json:"key,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
	ForceHttps *bool   `json:"forceHttps,omitempty"`
}

// BackupKeyResponse is returned once after generating a backup encryption key.
type BackupKeyResponse struct {
	Key          string `json:"key"`
	Encoding     string `json:"encoding"`
	Secret       string `json:"secret"`
	SecretKey    string `json:"secretKey"`
	SecretPath   string `json:"secretPath"`
	SetupShell   string `json:"setupShell"`
	SetupCommand string `json:"setupCommand"`
}

// Handler holds dependencies for settings HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new settings handler.
func NewHandler(app *router.AppDeps) *Handler {
	// Pull the cert reloader out of the AppDeps service registry.
	// main.go registers it under the canonical name "tls_reloader"
	// before this constructor runs. The lookup tolerates a missing
	// reloader (e.g. unit tests, dev builds) by passing nil to the
	// service, which then falls back to "read settings only" mode.
	var reloader *tlsconfig.Reloader
	if svc, ok := app.LookupService("tls_reloader"); ok {
		if r, isReloader := svc.(*tlsconfig.Reloader); isReloader {
			reloader = r
		}
	}

	return &Handler{
		app: app,
		service: NewService(
			app.DB,
			app.Encryption,
			app.Config.DataDir,
			reloader,
			app.Config.TLSSANHosts,
			app.Config.TLSSANIPs,
			uint16(app.Config.TLSMinVersion),
		),
	}
}

// HandleGetTimezone returns the configured McHarbor process timezone.
func (h *Handler) HandleGetTimezone(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, h.service.Timezone(r.Context()))
}

// HandleUpdateTimezone persists TZ and schedules a McHarbor container recreate.
func (h *Handler) HandleUpdateTimezone(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input TimezoneInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.Timezone(r.Context())
	status, err := h.service.UpdateTimezone(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrTimezoneNative) {
			response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "native_timezone"})
			return
		}
		var validationErr *ErrValidation
		if errors.As(err, &validationErr) {
			code := validationErr.Code
			if code == "" {
				code = i18n.ErrInvalidBody
			}
			response.BadRequestCause(w, r, code, err)
			return
		}
		h.app.Logger.Error("settings: timezone update failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		EntityName: timezoneSettingKey,
		Details:    fmt.Sprintf("timezone updated to %s and McHarbor recreate scheduled", status.Timezone),
	}, before, status)

	response.OK(w, status)
}

// HandleList returns all settings.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	items, err := h.service.List(r.Context(), category)
	if err != nil {
		h.app.Logger.Error("settings: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, items)
}

// HandleBulkUpdate updates multiple settings at once.
func (h *Handler) HandleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	var input BulkUpdateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if len(input.Settings) == 0 {
		response.BadRequestCode(w, r, i18n.ErrSettingsNoSettings)
		return
	}

	for _, setting := range input.Settings {
		if isSensitiveKey(setting.Key) {
			response.BadRequestCode(w, r, i18n.ErrProtectedTarget)
			return
		}
		if code, ok := validateSettingValue(setting.Key, setting.Value); !ok {
			response.BadRequestCode(w, r, code)
			return
		}
	}

	before := h.settingsSnapshot(r.Context(), input.Settings)
	result, err := h.service.BulkUpdate(r.Context(), input.Settings)
	if err != nil {
		h.app.Logger.Error("settings: bulk update error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	for _, setting := range input.Settings {
		if setting.Key == rbac.SettingMFARequired {
			rbac.InvalidateMFAPolicy()
		}
	}

	after := h.settingsSnapshot(r.Context(), input.Settings)
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("bulk update: %d settings", result.Updated),
	}, before, after)

	response.OK(w, map[string]any{
		"updated": result.Updated,
		"message": "Settings updated",
	})
}

// HandleGetByKey returns a single setting by key.
func (h *Handler) HandleGetByKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	if _, blocked := sensitiveKeys[key]; blocked {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	setting, err := h.service.ByKey(r.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("settings: get error", "error", err, "key", key)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, setting)
}

// HandleSetByKey sets a single setting by key (upsert).
func (h *Handler) HandleSetByKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	if isSensitiveKey(key) {
		response.BadRequestCode(w, r, i18n.ErrProtectedTarget)
		return
	}

	var input SettingInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if code, ok := validateSettingValue(key, input.Value); !ok {
		response.BadRequestCode(w, r, code)
		return
	}

	before := h.settingSnapshot(r.Context(), key)
	setting, err := h.service.SetByKey(r.Context(), key, input)
	if err != nil {
		h.app.Logger.Error("settings: set error", "error", err, "key", key)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	// Drop the memoised policy so the new window applies to the next
	// request rather than up to the cache TTL later.
	if key == auth.SettingSessionIdleTimeout && h.app.AuthService != nil {
		h.app.AuthService.InvalidateSessionPolicy()
	}
	if key == rbac.SettingMFARequired {
		rbac.InvalidateMFAPolicy()
	}

	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		EntityName: key,
	}, before, setting)

	response.OK(w, setting)
}

// HandleGetAgentSettings returns the current agent settings.
func (h *Handler) HandleGetAgentSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.AgentSettings(r.Context())
	response.OK(w, s)
}

// HandleUpdateAgentSettings validates and upserts agent settings.
func (h *Handler) HandleUpdateAgentSettings(w http.ResponseWriter, r *http.Request) {
	var input AgentSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.AgentSettings(r.Context())
	err := h.service.UpdateAgentSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: agent update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrAgentSettingsInvalid, err)
		return
	}

	after := h.service.AgentSettings(r.Context())
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "agent settings updated",
	}, before, after)

	response.OKMsg(w, r, i18n.MsgAgentSettingsUpdated)
}

// HandleGetScannerSettings returns the current scanner settings.
func (h *Handler) HandleGetScannerSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.ScannerSettings(r.Context())
	response.OK(w, s)
}

// HandleUpdateScannerSettings validates and upserts scanner settings.
func (h *Handler) HandleUpdateScannerSettings(w http.ResponseWriter, r *http.Request) {
	var input ScannerSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.ScannerSettings(r.Context())
	err := h.service.UpdateScannerSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: scanner update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrScannerSettingsInvalid, err)
		return
	}

	after := h.service.ScannerSettings(r.Context())
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "scanner settings updated",
	}, before, after)

	response.OKMsg(w, r, i18n.MsgScannerSettingsUpdated)
}

// HandleGetRetentionSettings returns the current retention settings.
func (h *Handler) HandleGetRetentionSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.RetentionSettings(r.Context())
	response.OK(w, s)
}

// HandleUpdateRetentionSettings validates and upserts retention settings.
func (h *Handler) HandleUpdateRetentionSettings(w http.ResponseWriter, r *http.Request) {
	var input RetentionSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.RetentionSettings(r.Context())
	err := h.service.UpdateRetentionSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: retention update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrRetentionSettingsInvalid, err)
		return
	}

	after := h.service.RetentionSettings(r.Context())
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details: fmt.Sprintf(
			"retention settings updated: audit=%dd preset=%s, activity=%dd, backups=%d, backupDays=%d",
			input.AuditRetentionDays,
			input.ActiveAuditRetentionPreset,
			input.ActivityRetentionDays,
			input.BackupRetentionCount,
			input.BackupRetentionDays,
		),
	}, before, after)

	response.OKMsg(w, r, i18n.MsgRetentionSettingsUpdated)
}

// HandleGetTLS returns the current TLS configuration status.
func (h *Handler) HandleGetTLS(w http.ResponseWriter, r *http.Request) {
	status := h.service.TLSStatus(r.Context())
	response.OK(w, status)
}

// HandleUpdateTLS updates TLS certificate, key, and toggle settings.
func (h *Handler) HandleUpdateTLS(w http.ResponseWriter, r *http.Request) {
	var input TLSUpdateRequest
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.TLSStatus(r.Context())
	status, err := h.service.UpdateTLS(r.Context(), input)
	if err != nil {
		var validationErr *ErrValidation
		if errors.As(err, &validationErr) {
			if validationErr.Message == "cert and key must be provided together" {
				response.BadRequestCode(w, r, i18n.ErrSettingsTLSPairReq)
			} else {
				response.BadRequestCode(w, r, i18n.ErrSettingsTLSInvalid)
			}
			return
		}
		h.app.Logger.Error("settings: tls update error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "TLS settings updated",
	}, before, status)

	response.OK(w, status)
}

// HandleRegenerateTLS re-mints the auto-generated self-signed cert
// and reloads the listeners in place. Used by the security tab when
// the operator wants a fresh fingerprint without uploading a real
// cert. Phase 7 default-HTTPS.
//
// Only meaningful when the running cert is self-signed (certSource
// = "self-signed"); if the operator uploaded a cert we reject the
// request rather than silently trashing it. The service does the
// actual regeneration; the handler's role is auth, audit, and
// returning a translated error code on failure.
func (h *Handler) HandleRegenerateTLS(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// Guard against accidentally blowing away an admin-uploaded
	// cert — the UI hides the button in that case but a direct
	// POST still needs the check.
	status := h.service.TLSStatus(r.Context())
	if status.CertSource == "user-uploaded" {
		response.BadRequestCode(w, r, i18n.ErrSettingsTLSRegenUserCert)
		return
	}

	fingerprint, err := h.service.RegenerateSelfSigned(r.Context())
	if err != nil {
		h.app.Logger.Error("settings: tls regenerate failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsTLSRegenerate, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("self-signed TLS certificate regenerated: fingerprint=%s", fingerprint),
	})

	h.app.Logger.Info("self-signed TLS certificate regenerated", "fingerprint", fingerprint)

	// Return the full status so the UI can refresh the cert
	// metadata (subject, expiry, new fingerprint) without a
	// second GET.
	newStatus := h.service.TLSStatus(r.Context())
	response.OK(w, newStatus)
}

// HandleGetBackupKeyStatus reports whether the runtime backup encryption key is active.
func (h *Handler) HandleGetBackupKeyStatus(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, h.service.BackupKeyStatus(h.app.Config.BackupEncryptionKeyFile))
}

// HandleGenerateBackupKey generates a one-time backup encryption key for Docker secret storage.
func (h *Handler) HandleGenerateBackupKey(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		h.app.Logger.Error("settings: backup key generation failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "generate",
		EntityType: "settings",
		EntityName: "backup_encryption_key",
		Details:    "backup encryption key generated for one-time copy",
	})

	encodedKey := base64.StdEncoding.EncodeToString(key)
	if dataDir, _, native := nativeHostConfig(); native {
		path := NativeBackupKeyPath(dataDir)
		response.OK(w, BackupKeyResponse{ //nolint:gosec // G101: names the file the key goes in, not a credential
			Key:          encodedKey,
			Encoding:     "base64",
			Secret:       "mcharbor_backup_key",
			SecretKey:    "BACKUP_ENCRYPTION_KEY_FILE",
			SecretPath:   path,
			SetupShell:   "sh",
			SetupCommand: nativeSetupCommand(path, encodedKey),
		})
		return
	}
	response.OK(w, BackupKeyResponse{ //nolint:gosec // G101: the secret name below is a docker secret name, not a credential
		Key:        encodedKey,
		Encoding:   "base64",
		Secret:     "mcharbor_backup_key",
		SecretKey:  "BACKUP_ENCRYPTION_KEY_FILE",
		SecretPath: "secrets/mcharbor_backup_key",
		SetupShell: "powershell",
		SetupCommand: strings.Join([]string{
			"New-Item -ItemType Directory -Force secrets",
			fmt.Sprintf("Set-Content -NoNewline -Path secrets/mcharbor_backup_key -Value %q", encodedKey),
			"docker compose -f docker-compose.yml -f docker-compose.secrets.yml up -d",
		}, "\n"),
	})
}

// HandleInstallBackupKey installs a one-time backup encryption key as a host-side Docker secret file.
func (h *Handler) HandleInstallBackupKey(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req BackupKeyInstallRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	result, err := h.service.InstallBackupKeyWithPath(r.Context(), req.Key, req.ProjectPath)
	if err != nil {
		var validationErr *ErrValidation
		if errors.As(err, &validationErr) {
			code := validationErr.Code
			if code == "" {
				code = i18n.ErrInvalidBody
			}
			response.BadRequestCause(w, r, code, err)
			return
		}
		if errors.Is(err, ErrNativeRestartUnsupervised) {
			h.app.Logger.Warn("settings: backup key installed but McHarbor cannot restart itself", "error", err)
			response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrSystemRestartNotLocal,
				map[string]string{"reason": "not_supervised"})
			return
		}
		h.app.Logger.Error("settings: backup key install failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "install",
		EntityType: "settings",
		EntityName: "backup_encryption_key",
		Details:    "backup encryption key installed as Docker secret and restart scheduled",
	})

	response.OK(w, result)
}

// parseCertInfo extracts metadata from a PEM-encoded certificate.
func parseCertInfo(certPEM string) *CertInfo {
	block, _ := pem.Decode([]byte(certPEM)) // safe: second return is unused rest bytes, block nil-checked below
	if block == nil {
		return nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	sum := sha256.Sum256(cert.Raw)

	return &CertInfo{
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		NotBefore:    cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:     cert.NotAfter.UTC().Format(time.RFC3339),
		SerialNumber: fmt.Sprintf("%x", cert.SerialNumber),
		DNSNames:     cert.DNSNames,
		Fingerprint:  hexColon(sum[:]),
	}
}

// hexColon formats a byte slice as lowercase colon-separated hex, the
// form `openssl x509 -fingerprint -sha256` prints so the two can be
// compared by eye.
func hexColon(b []byte) string {
	const hex = "0123456789abcdef"
	if len(b) == 0 {
		return ""
	}
	out := make([]byte, 0, len(b)*3-1)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[c>>4], hex[c&0x0f])
	}
	return string(out)
}

// HandleDownloadCA serves the local root certificate so an operator can
// install it into an OS trust store and stop clicking through browser
// warnings. The root is public material — only the private key beside it
// is sensitive, and that never leaves the host.
func (h *Handler) HandleDownloadCA(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	pemBytes, err := h.service.CACertificate()
	if err != nil {
		h.app.Logger.Error("settings: ca certificate read failed", "error", err)
		response.NotFoundCause(w, r, i18n.ErrSettingsTLSNoCA, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="mcharbor-root-ca.pem"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(pemBytes); err != nil {
		h.app.Logger.Warn("settings: ca certificate write failed", "error", err)
	}
}

// HandleGetToastSettings returns the app-wide toast defaults.
//
// This route is authenticated but carries no permission check: every signed-in
// user needs the default to render their own toasts, and the response holds
// nothing an operator would withhold — six display knobs and a flag saying
// whether the user is allowed to overrule them.
func (h *Handler) HandleGetToastSettings(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.service.ToastSettings(r.Context()))
}

// HandleUpdateToastSettings validates and upserts the app-wide toast defaults.
func (h *Handler) HandleUpdateToastSettings(w http.ResponseWriter, r *http.Request) {
	var input ToastSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.ToastSettings(r.Context())
	if err := h.service.UpdateToastSettings(r.Context(), input); err != nil {
		h.app.Logger.Error("settings: toast update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrToastSettingsInvalid, err)
		return
	}

	after := h.service.ToastSettings(r.Context())
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details: fmt.Sprintf(
			"toast defaults updated: position=%s duration=%dms max=%d userOverride=%t",
			input.Position, input.DurationMs, input.MaxVisible, input.AllowUserOverride,
		),
	}, before, after)

	response.OKMsg(w, r, i18n.MsgToastSettingsUpdated)
}

// HandleResetUserToastOverrides clears every account's toast overrides so the
// whole install follows the app-wide default again.
func (h *Handler) HandleResetUserToastOverrides(w http.ResponseWriter, r *http.Request) {
	reset, err := h.service.ResetUserToastOverrides(r.Context())
	if err != nil {
		h.app.Logger.Error("settings: toast override reset error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("toast overrides reset for %d account(s)", reset),
	})

	response.OK(w, map[string]int64{"reset": reset})
}

// AppearanceSettingsInput is the request body for PUT /settings/appearance.
// Every field is required: the form posts the whole set on every save, so a
// missing field is a malformed request rather than "keep what is stored".
type AppearanceSettingsInput struct {
	ThemeID           string `json:"themeId"`
	Accent            string `json:"accent"`
	Font              string `json:"font"`
	BaseFontPx        int    `json:"baseFontPx"`
	AllowUserOverride bool   `json:"allowUserOverride"`
}

// HandleGetAppearanceSettings returns the app-wide appearance defaults.
//
// Authenticated-only on purpose, like GET /settings/toast: every signed-in
// user needs the app-wide default to paint their own theme and font, and it
// exposes nothing sensitive.
func (h *Handler) HandleGetAppearanceSettings(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.service.AppearanceSettings(r.Context()))
}

// HandleUpdateAppearanceSettings validates and upserts the app-wide
// appearance defaults.
func (h *Handler) HandleUpdateAppearanceSettings(w http.ResponseWriter, r *http.Request) {
	var input AppearanceSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.AppearanceSettings(r.Context())
	if err := h.service.UpdateAppearanceSettings(r.Context(), input); err != nil {
		h.app.Logger.Error("settings: appearance update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrAppearanceSettingsInvalid, err)
		return
	}
	after := h.service.AppearanceSettings(r.Context())

	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details: fmt.Sprintf(
			"appearance defaults updated: theme=%s accent=%s font=%s baseFontPx=%d userOverride=%t",
			input.ThemeID, input.Accent, input.Font, input.BaseFontPx, input.AllowUserOverride,
		),
	}, before, after)

	response.OKMsg(w, r, i18n.MsgAppearanceSettingsUpdated)
}

// ─── Branding ─────────────────────────────────────────────────────────

// BrandingSettingsInput is the request body for PUT /settings/branding.
//
// Both fields are `data:` URIs and both are required: the card posts the
// whole set on every save, so an empty string is an explicit "clear this
// mark and fall back to the bundled one", not an omission.
type BrandingSettingsInput struct {
	LogoWide     string `json:"logoWide"`
	LogoWideDark string `json:"logoWideDark"`
	LogoIcon     string `json:"logoIcon"`
	LogoIconDark string `json:"logoIconDark"`
	// PNG renditions of the two light marks, produced by the branding card
	// so document exporters have a raster to embed. See BrandingSettings.
	LogoWidePNG string `json:"logoWidePng"`
	LogoIconPNG string `json:"logoIconPng"`
}

// HandleGetBranding returns the operator-supplied application marks.
//
// Public on purpose — this is the one app-wide default the *login* page needs
// before anyone has a session, and it carries nothing but two images an
// operator uploaded to be shown to every visitor anyway.
func (h *Handler) HandleGetBranding(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.service.BrandingSettings(r.Context()))
}

// HandleUpdateBranding validates and upserts both application marks.
func (h *Handler) HandleUpdateBranding(w http.ResponseWriter, r *http.Request) {
	var input BrandingSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	before := h.service.BrandingSettings(r.Context())
	if err := h.service.UpdateBrandingSettings(r.Context(), input); err != nil {
		h.app.Logger.Error("settings: branding update error", "error", err)
		response.BadRequestCause(w, r, i18n.ErrBrandingInvalid, err)
		return
	}
	after := h.service.BrandingSettings(r.Context())

	// The images themselves are far too large to put in an audit detail, and
	// useless there — what an operator needs to see afterwards is which mark
	// changed and whether it was set or cleared.
	h.logAuditWithDiff(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details: fmt.Sprintf(
			"branding updated: wordmark=%s wordmarkDark=%s icon=%s iconDark=%s",
			brandingMarkState(input.LogoWide), brandingMarkState(input.LogoWideDark),
			brandingMarkState(input.LogoIcon), brandingMarkState(input.LogoIconDark),
		),
	}, brandingAuditView(before), brandingAuditView(after))

	response.OKMsg(w, r, i18n.MsgBrandingUpdated)
}

// brandingMarkState renders one mark for the audit detail line.
func brandingMarkState(value string) string {
	if value == "" {
		return "default"
	}
	return "custom"
}

// brandingAuditView reduces the marks to what the audit diff should carry:
// set-or-cleared and a size, never half a megabyte of base64.
func brandingAuditView(s coreSettings.BrandingSettings) map[string]any {
	return map[string]any{
		"logoWide":          brandingMarkState(s.LogoWide),
		"logoWideBytes":     len(s.LogoWide),
		"logoWideDark":      brandingMarkState(s.LogoWideDark),
		"logoWideDarkBytes": len(s.LogoWideDark),
		"logoIcon":          brandingMarkState(s.LogoIcon),
		"logoIconBytes":     len(s.LogoIcon),
		"logoIconDark":      brandingMarkState(s.LogoIconDark),
		"logoIconDarkBytes": len(s.LogoIconDark),
		"logoWidePng":       brandingMarkState(s.LogoWidePNG),
		"logoIconPng":       brandingMarkState(s.LogoIconPNG),
	}
}

// HandleResetUserAppearanceOverrides clears every account's appearance
// overrides so the whole install follows the app-wide default again.
func (h *Handler) HandleResetUserAppearanceOverrides(w http.ResponseWriter, r *http.Request) {
	reset, err := h.service.ResetUserAppearanceOverrides(r.Context())
	if err != nil {
		h.app.Logger.Error("settings: appearance override reset error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("appearance overrides reset for %d account(s)", reset),
	})

	response.OK(w, map[string]int64{"reset": reset})
}
