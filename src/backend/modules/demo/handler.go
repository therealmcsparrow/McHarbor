// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves /demo.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler returns a Handler over service.
func NewHandler(app *router.AppDeps, service *Service) *Handler {
	return &Handler{app: app, service: service}
}

// reasonOf names a refusal with a stable value the UI translates, because the
// backend's generic codes cannot say why demo mode refused.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrAlreadyActive):
		return "already_active"
	case errors.Is(err, ErrNotActive):
		return "not_active"
	case errors.Is(err, ErrClustered):
		return "clustered"
	case errors.Is(err, ErrBusy):
		return "busy"
	}
	return ""
}

// HandleStatus reports whether demo mode is active; every signed-in user needs
// it, because the banner that says the data is fake must reach everyone.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	response.OK(w, h.service.Status())
}

func (h *Handler) change(w http.ResponseWriter, r *http.Request, action string, run func(ctx context.Context, userID string, cfg *coredemo.Config) (Outcome, error)) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var req ChangeRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	outcome, err := run(ctx, user.ID, req.Config)
	var fieldErr *coredemo.FieldError
	if errors.As(err, &fieldErr) {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]any{
			"reason": "invalid_config", "field": fieldErr.Field, "min": fieldErr.Range.Min, "max": fieldErr.Range.Max,
		})
		return
	}
	if err != nil {
		if reason := reasonOf(err); reason != "" {
			code := i18n.ErrForbidden
			if reason == "clustered" {
				code = i18n.ErrPlatformUnsupported
			}
			response.BadRequestCodeWithData(w, r, code, map[string]string{"reason": reason})
			return
		}
		h.app.Logger.Error("demo: "+action+" failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "demo." + action,
		EntityType: "system",
		EntityName: "demo-mode",
		Details:    "demo mode " + action,
	})
	response.OK(w, outcome)
}

// HandleEnter switches the instance into demo mode.
func (h *Handler) HandleEnter(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "enter", h.service.Enter)
}

// HandleExit switches the instance back to its real database.
func (h *Handler) HandleExit(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "exit", func(ctx context.Context, _ string, _ *coredemo.Config) (Outcome, error) { return h.service.Exit(ctx) })
}

// HandleReset discards the demo estate so the next visit starts fresh.
func (h *Handler) HandleReset(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "reset", h.service.Reset)
}
