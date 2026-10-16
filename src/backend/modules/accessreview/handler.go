// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package accessreview

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves /api/access-review.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler wires the access review handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB, app.RBACService, app.Logger)}
}

// HandleEffective returns one user's effective permissions at
// ?environmentId= and ?stack=, each key with the sources that grant or deny it.
func (h *Handler) HandleEffective(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := strings.TrimSpace(q.Get("userId"))
	envID := strings.TrimSpace(q.Get("environmentId"))
	stack := strings.TrimSpace(q.Get("stack"))
	if userID == "" {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "user_required"})
		return
	}
	if stack != "" && envID == "" {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "stack_requires_environment"})
		return
	}
	out, err := h.service.Effective(r.Context(), userID, envID, stack)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.ErrCodeWithData(w, r, http.StatusNotFound, i18n.ErrNotFound, map[string]string{"reason": "user_not_found"})
		return
	case errors.Is(err, ErrEnvironmentNotFound):
		response.ErrCodeWithData(w, r, http.StatusNotFound, i18n.ErrNotFound, map[string]string{"reason": "environment_not_found"})
		return
	case err != nil:
		h.app.Logger.Error("access review: effective permissions", "error", err, "user", userID, "env", envID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, out)
}

// HandleCatalogExport streams the permission catalog as CSV or JSON for a
// security review. The export is audited because it maps who can do what.
func (h *Handler) HandleCatalogExport(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "unsupported_format"})
		return
	}
	export, err := h.service.Export(r.Context())
	if err != nil {
		h.app.Logger.Error("access review: catalog export", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	var body []byte
	contentType := "application/json; charset=utf-8"
	if format == "csv" {
		body, err = ExportCSV(export)
		contentType = "text/csv; charset=utf-8"
	} else {
		body, err = json.MarshalIndent(export, "", "  ")
	}
	if err != nil {
		h.app.Logger.Error("access review: encoding catalog export", "error", err, "format", format)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if user := auth.UserFromContext(r.Context()); user != nil {
		h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
			Action:     "permission_catalog.exported",
			Severity:   "info",
			EntityType: "permission_catalog",
			EntityName: format,
		})
	}

	filename := "mcharbor-permission-catalog-" + time.Now().UTC().Format("20060102-1504") + "." + format
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		h.app.Logger.Error("access review: writing catalog export", "error", err)
	}
}
