// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler exposes widget definition endpoints.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler creates a new widget handler.
func NewHandler(app *router.AppDeps, svc *Service) *Handler {
	return &Handler{app: app, svc: svc}
}

// HandleList returns all widget definitions with translations.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	defs, err := h.svc.List()
	if err != nil {
		h.app.Logger.Error("failed to list widget definitions", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrWidgetListFailed, err)
		return
	}

	if defs == nil {
		defs = []WidgetDefinitionWithI18n{}
	}

	response.OK(w, defs)
}

// HandleUpdate updates widget availability.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrWidgetKeyRequired)
		return
	}

	var input UpdateWidgetStateInput
	if err := response.DecodeBody(r, &input); err != nil || input.Enabled == nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.SetEnabled(key, *input.Enabled); err != nil {
		h.app.Logger.Error("failed to update widget", "key", key, "error", err)
		if strings.Contains(err.Error(), "not found") {
			response.NotFoundCode(w, r, i18n.ErrWidgetNotFound)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrWidgetUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     map[bool]string{true: "enable", false: "disable"}[*input.Enabled],
		EntityType: "widget",
		EntityID:   key,
	})

	response.OK(w, map[string]any{
		"key":     key,
		"enabled": *input.Enabled,
	})
}

// writeWidgetError responds with the i18n code for a catalog/install sentinel, so
// the operator reads why a bundle was refused rather than a generic failure.
func writeWidgetError(w http.ResponseWriter, r *http.Request, err error, fallback i18n.MsgCode) {
	switch {
	case errors.Is(err, ErrSignatureInvalid):
		response.BadRequestCode(w, r, i18n.ErrWidgetSignatureInvalid)
	case errors.Is(err, ErrChecksumMismatch):
		response.BadRequestCode(w, r, i18n.ErrWidgetChecksumMismatch)
	case errors.Is(err, ErrCatalogEntryNotFound):
		response.NotFoundCode(w, r, i18n.ErrWidgetCatalogEntryNotFound)
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrWidgetNotFound)
	case errors.Is(err, ErrDowngrade):
		response.ConflictCode(w, r, i18n.ErrWidgetDowngrade)
	case errors.Is(err, ErrKeyTaken):
		response.ConflictCode(w, r, i18n.ErrWidgetKeyExists)
	case errors.Is(err, ErrNotDownloaded):
		response.ConflictCode(w, r, i18n.ErrWidgetBuiltinDelete)
	default:
		response.InternalErrorCode(w, r, fallback)
	}
}

// HandleCatalog lists the widget catalog with signature and install state.
func (h *Handler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.svc.Catalog())
}

// HandleInstall verifies and installs one catalog widget. The signature check is
// the server's; the browser only ever loads what this route accepted.
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrWidgetKeyRequired)
		return
	}
	def, err := h.svc.Install(key, InstallOptions{})
	if err != nil {
		h.app.Logger.Error("failed to install catalog widget", "key", key, "error", err)
		writeWidgetError(w, r, err, i18n.ErrWidgetInstallFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "install",
		Severity:   "warning",
		EntityType: "widget",
		EntityID:   key,
		EntityName: def.Label,
		Details:    "version " + def.Version,
	})
	response.OK(w, def)
}

// HandleRemove uninstalls a downloaded widget; built-ins are refused.
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	key := chi.URLParam(r, "key")
	if err := h.svc.Remove(key); err != nil {
		h.app.Logger.Error("failed to remove downloaded widget", "key", key, "error", err)
		writeWidgetError(w, r, err, i18n.ErrWidgetUninstallFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "uninstall",
		EntityType: "widget",
		EntityID:   key,
	})
	response.OKMsg(w, r, i18n.MsgWidgetUninstalled)
}

// HandleBundle serves a downloaded widget's bundle from McHarbor's own origin,
// re-verified on every request so a file altered on disk is never executed.
func (h *Handler) HandleBundle(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	payload, err := h.svc.Bundle(key)
	if err != nil {
		h.app.Logger.Error("refusing downloaded widget bundle", "key", key, "error", err)
		writeWidgetError(w, r, err, i18n.ErrWidgetListFailed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		h.app.Logger.Warn("writing widget bundle", "key", key, "error", err)
	}
}
