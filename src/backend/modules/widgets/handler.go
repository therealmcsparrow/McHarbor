// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/audit"
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
		response.InternalErrorCode(w, r, i18n.ErrWidgetListFailed)
		return
	}

	if defs == nil {
		defs = []WidgetDefinitionWithI18n{}
	}

	response.OK(w, defs)
}

// HandleInstall installs a new widget definition.
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	var input InstallWidgetInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Definition.Key == "" {
		response.BadRequestCode(w, r, i18n.ErrWidgetKeyRequired)
		return
	}

	if err := h.svc.Install(input); err != nil {
		h.app.Logger.Error("failed to install widget", "key", input.Definition.Key, "error", err)
		if strings.Contains(err.Error(), "already exists") {
			response.ConflictCode(w, r, i18n.ErrWidgetKeyExists)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrWidgetInstallFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "install",
		EntityType: "widget",
		EntityID:   input.Definition.Key,
		EntityName: input.Definition.Label,
	})

	response.OKMsg(w, r, i18n.MsgWidgetInstalled)
}

// HandleUninstall removes a non-builtin widget definition.
func (h *Handler) HandleUninstall(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrWidgetKeyRequired)
		return
	}

	if err := h.svc.Uninstall(key); err != nil {
		h.app.Logger.Error("failed to uninstall widget", "key", key, "error", err)
		if strings.Contains(err.Error(), "not found") {
			response.NotFoundCode(w, r, i18n.ErrWidgetNotFound)
			return
		}
		if strings.Contains(err.Error(), "built-in") {
			response.BadRequestCode(w, r, i18n.ErrWidgetBuiltinDelete)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrWidgetUninstallFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "uninstall",
		EntityType: "widget",
		EntityID:   key,
	})

	response.OKMsg(w, r, i18n.MsgWidgetUninstalled)
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
