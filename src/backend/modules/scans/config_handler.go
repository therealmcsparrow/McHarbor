// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package scans runs vulnerability scans against images and manages the scanner configurations they use.
package scans

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

const (
	auditActionScannerSaved   = "scans.scanner.saved"
	auditActionScannerDeleted = "scans.scanner.deleted"
	auditActionScannerTested  = "scans.scanner.tested"
)

// HandleScannerAdapters lists the installed scanner engines.
//
// Separate from the instance list because the Add picker asks a
// different question than the tab's list does: "what engines can I
// use" versus "what am I currently scanning with". One adapter backs
// any number of instances.
func (h *Handler) HandleScannerAdapters(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.configs.Adapters(r.Context()))
}

// HandleScannerConfigs lists every configured scanner instance with
// its stored config merged in.
//
// The response carries no secret values — only a per-field boolean
// saying one is stored. See ConfigService.buildInfo.
func (h *Handler) HandleScannerConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := h.configs.Configs(r.Context())
	if err != nil {
		h.app.Logger.Error("scans: listing scanner configs failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerListFailed, err)
		return
	}
	response.OK(w, items)
}

// saveScannerRequest is the Scanners tab's form submission. Config
// carries the adapter's declared ConfigSchema keys; the host does not
// know or validate their meaning beyond the declared type.
type saveScannerRequest struct {
	Name string `json:"name"`
	// ScannerType is read on create only — see SaveScannerConfigInput.
	ScannerType string            `json:"scannerType"`
	Enabled     bool              `json:"enabled"`
	MakeDefault bool              `json:"makeDefault"`
	Config      map[string]string `json:"config"`
}

func (req saveScannerRequest) input() SaveScannerConfigInput {
	return SaveScannerConfigInput(req)
}

// writeScannerSaveError maps the shared create/update validation
// failures. Returns true when it handled the error.
func (h *Handler) writeScannerSaveError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, ErrScannerAdapterUnknown):
		response.NotFoundCode(w, r, i18n.ErrScannerAdapterUnknown)
	case errors.Is(err, ErrScannerNotConfigured):
		response.NotFoundCode(w, r, i18n.ErrScannerNotConfigured)
	case errors.Is(err, ErrScannerNameRequired):
		response.BadRequestCode(w, r, i18n.ErrScannerNameRequired)
	case errors.Is(err, ErrScannerFieldRequired):
		// The operator left a field the adapter declared required
		// empty. Their form to fix, not a server fault — and the
		// response names no field value, only the shape of the
		// mistake.
		response.BadRequestCode(w, r, i18n.ErrScannerFieldRequired)
	default:
		return false
	}
	return true
}

// HandleCreateScannerConfig adds one scanner instance.
func (h *Handler) HandleCreateScannerConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req saveScannerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(req.ScannerType) == "" {
		response.BadRequestCode(w, r, i18n.ErrScannerAdapterUnknown)
		return
	}

	id, err := h.configs.Create(r.Context(), req.input())
	if h.writeScannerSaveError(w, r, err) {
		return
	}
	if err != nil {
		h.app.Logger.Error("scans: creating scanner config failed",
			"scanner", req.ScannerType, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerCreateFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionScannerSaved,
		Severity:   "warning",
		EntityType: "scanner_config",
		EntityID:   id,
		EntityName: strings.TrimSpace(req.Name),
		Details:    req.ScannerType,
	})

	response.Created(w, map[string]string{"id": id})
}

// HandleSaveScannerConfig rewrites one scanner instance.
func (h *Handler) HandleSaveScannerConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}

	var req saveScannerRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	err := h.configs.Update(r.Context(), id, req.input())
	if h.writeScannerSaveError(w, r, err) {
		return
	}
	if err != nil {
		h.app.Logger.Error("scans: saving scanner config failed", "scanner", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerSaveFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionScannerSaved,
		Severity:   "warning",
		EntityType: "scanner_config",
		EntityID:   id,
		EntityName: strings.TrimSpace(req.Name),
	})

	response.OKMsg(w, r, i18n.MsgScannerSaved)
}

// HandleDeleteScannerConfig removes one scanner instance.
//
// The plugin stays installed and its adapter stays registered — what
// goes is this configured engine. Uninstalling the plugin is a
// separate action on a separate page, and scans this engine already
// ran keep their findings either way.
func (h *Handler) HandleDeleteScannerConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}

	// Read before deleting so the audit entry names what was removed.
	// An entry saying only "id cvv0f2…" is useless six months later,
	// which is exactly when it gets read.
	info, err := h.configs.Config(r.Context(), id)
	if errors.Is(err, ErrScannerNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}
	if err != nil {
		h.app.Logger.Error("scans: reading scanner config before delete failed",
			"scanner", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerDeleteFailed, err)
		return
	}

	err = h.configs.Delete(r.Context(), id)
	if errors.Is(err, ErrScannerNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}
	if err != nil {
		h.app.Logger.Error("scans: deleting scanner config failed", "scanner", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerDeleteFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionScannerDeleted,
		Severity:   "warning",
		EntityType: "scanner_config",
		EntityID:   id,
		EntityName: info.Name,
		Details:    info.ScannerType,
	})

	response.OKMsg(w, r, i18n.MsgScannerDeleted)
}

// HandleTestScannerConfig runs the scanner's self-test and returns a
// per-step report.
//
// A *failing* test is 200, not 400. The request was well-formed and
// the server answered it correctly — it produced a diagnosis, which is
// exactly what was asked for. Returning 400 would leave the client
// with an error and no report, which is why the old single-shot
// version could only raise a toast. Same contract as
// /communication-channels/{id}/test and /storage-locations/{id}/test.
//
// Only a genuinely unanswerable request is an error status: no such
// scanner row.
func (h *Handler) HandleTestScannerConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}

	result, err := h.configs.TestScanner(r.Context(), id)
	if errors.Is(err, ErrScannerNotConfigured) {
		response.NotFoundCode(w, r, i18n.ErrScannerNotConfigured)
		return
	}
	if err != nil {
		h.app.Logger.Error("scans: scanner test could not run", "scanner", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerTestFailed, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     auditActionScannerTested,
		Severity:   "info",
		EntityType: "scanner_config",
		EntityID:   id,
		EntityName: result.ScannerName,
		Details:    result.OverallStatus,
	})

	response.OK(w, result)
}
