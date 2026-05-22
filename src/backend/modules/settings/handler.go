// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Setting represents a key-value setting.
type Setting struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Category  string `json:"category"`
	UpdatedAt string `json:"updatedAt"`
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
var sensitiveKeys = map[string]struct{}{
	"tls_cert": {},
	"tls_key":  {},
}

// --- Agent Settings ---

// AgentSettingsInput is the request body for updating agent settings.
type AgentSettingsInput struct {
	EventMode         string `json:"eventMode"`
	EventPollInterval int    `json:"eventPollInterval"`
	PingInterval      int    `json:"pingInterval"`
	MetricsEnabled    bool   `json:"metricsEnabled"`
	RequestTimeout    int    `json:"requestTimeout"`
}

// --- Scanner Settings ---

// ScannerSettingsInput is the request body for updating scanner settings.
type ScannerSettingsInput struct {
	TrivyEnabled   bool   `json:"trivyEnabled"`
	GrypeEnabled   bool   `json:"grypeEnabled"`
	ClairEnabled   bool   `json:"clairEnabled"`
	ClairURL       string `json:"clairUrl"`
	DefaultScanner string `json:"defaultScanner"`
	ScanTimeout    int    `json:"scanTimeout"`
	ScanOnInstall  bool   `json:"scanOnInstall"`
	ScanOnUpdate   bool   `json:"scanOnUpdate"`
}

// --- Retention Settings ---

// RetentionSettingsInput is the request body for updating retention settings.
type RetentionSettingsInput struct {
	AuditRetentionDays    int `json:"auditRetentionDays"`
	ActivityRetentionDays int `json:"activityRetentionDays"`
}

// --- TLS types ---

// TLSStatus is the response shape for TLS configuration.
type TLSStatus struct {
	Enabled    bool      `json:"enabled"`
	ForceHttps bool      `json:"forceHttps"`
	HasCert    bool      `json:"hasCert"`
	CertInfo   *CertInfo `json:"certInfo,omitempty"`
}

// CertInfo contains parsed certificate metadata (never raw PEM).
type CertInfo struct {
	Subject      string   `json:"subject"`
	Issuer       string   `json:"issuer"`
	NotBefore    string   `json:"notBefore"`
	NotAfter     string   `json:"notAfter"`
	SerialNumber string   `json:"serialNumber"`
	DNSNames     []string `json:"dnsNames"`
}

// TLSUpdateRequest is the request body for updating TLS settings.
type TLSUpdateRequest struct {
	Cert       *string `json:"cert,omitempty"`
	Key        *string `json:"key,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
	ForceHttps *bool   `json:"forceHttps,omitempty"`
}

// Handler holds dependencies for settings HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new settings handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption, app.Config.DataDir),
	}
}

// HandleList returns all settings.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	items, err := h.service.List(r.Context(), category)
	if err != nil {
		h.app.Logger.Error("settings: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, items)
}

// HandleBulkUpdate updates multiple settings at once.
func (h *Handler) HandleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	var input BulkUpdateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if len(input.Settings) == 0 {
		response.BadRequestCode(w, r, i18n.ErrSettingsNoSettings)
		return
	}

	result, err := h.service.BulkUpdate(r.Context(), input.Settings)
	if err != nil {
		h.app.Logger.Error("settings: bulk update error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("bulk update: %d settings", result.Updated),
	})

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
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, setting)
}

// HandleSetByKey sets a single setting by key (upsert).
func (h *Handler) HandleSetByKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var input SettingInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	setting, err := h.service.SetByKey(r.Context(), key, input)
	if err != nil {
		h.app.Logger.Error("settings: set error", "error", err, "key", key)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		EntityName: key,
	})

	response.OK(w, setting)
}

// HandleGetAgentSettings returns the current agent settings.
func (h *Handler) HandleGetAgentSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.AgentSettings()
	response.OK(w, s)
}

// HandleUpdateAgentSettings validates and upserts agent settings.
func (h *Handler) HandleUpdateAgentSettings(w http.ResponseWriter, r *http.Request) {
	var input AgentSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	err := h.service.UpdateAgentSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: agent update error", "error", err)
		response.BadRequestCode(w, r, i18n.ErrAgentSettingsInvalid)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "agent settings updated",
	})

	response.OKMsg(w, r, i18n.MsgAgentSettingsUpdated)
}

// HandleGetScannerSettings returns the current scanner settings.
func (h *Handler) HandleGetScannerSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.ScannerSettings()
	response.OK(w, s)
}

// HandleUpdateScannerSettings validates and upserts scanner settings.
func (h *Handler) HandleUpdateScannerSettings(w http.ResponseWriter, r *http.Request) {
	var input ScannerSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	err := h.service.UpdateScannerSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: scanner update error", "error", err)
		response.BadRequestCode(w, r, i18n.ErrScannerSettingsInvalid)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "scanner settings updated",
	})

	response.OKMsg(w, r, i18n.MsgScannerSettingsUpdated)
}

// HandleGetRetentionSettings returns the current retention settings.
func (h *Handler) HandleGetRetentionSettings(w http.ResponseWriter, r *http.Request) {
	s := h.service.RetentionSettings()
	response.OK(w, s)
}

// HandleUpdateRetentionSettings validates and upserts retention settings.
func (h *Handler) HandleUpdateRetentionSettings(w http.ResponseWriter, r *http.Request) {
	var input RetentionSettingsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	err := h.service.UpdateRetentionSettings(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("settings: retention update error", "error", err)
		response.BadRequestCode(w, r, i18n.ErrRetentionSettingsInvalid)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    fmt.Sprintf("retention settings updated: audit=%dd, activity=%dd", input.AuditRetentionDays, input.ActivityRetentionDays),
	})

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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

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
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "settings",
		Details:    "TLS settings updated",
	})

	response.OK(w, status)
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

	return &CertInfo{
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		NotBefore:    cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:     cert.NotAfter.UTC().Format(time.RFC3339),
		SerialNumber: fmt.Sprintf("%x", cert.SerialNumber),
		DNSNames:     cert.DNSNames,
	}
}
