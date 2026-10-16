// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package catalogs stores the per-kind catalog sources (Docker apps, plugins, themes, VM images) and probes them for reachability.
package catalogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler exposes the per-kind catalog registry over REST.
// Handlers stay thin: parse → validate → authz → service → audit → respond.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler wires the catalog routes to a Service built on the shared application database.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB)}
}

// HandleList returns every catalog the admin can manage.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	items, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("catalogs: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	response.OK(w, items)
}

// HandleGet returns a single catalog row.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	item, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("catalogs: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	response.OK(w, item)
}

// HandleCreate adds a new catalog. Built-in (readonly) catalogs
// are skipped on collision so re-running the dialog is safe.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var input CreateRequest
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrDuplicateURL):
			response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody,
				map[string]string{"reason": "duplicate_url", "id": item.ID, "name": item.Name})
		case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidKind):
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		default:
			h.app.Logger.Error("catalogs: create error", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		}
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "catalog.created",
		EntityType: "catalog",
		EntityID:   item.ID,
		EntityName: item.Name,
	})
	response.Created(w, item)
}

// HandleUpdate edits the name, URL, or enabled flag of an existing row.
// Built-in rows allow enabled-only updates.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var input UpdateRequest
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	item, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.NotFoundCode(w, r, i18n.ErrNotFound)
		case errors.Is(err, ErrReadOnly):
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
		case errors.Is(err, ErrInvalidInput):
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		default:
			h.app.Logger.Error("catalogs: update error", "error", err, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		}
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "catalog.updated",
		EntityType: "catalog",
		EntityID:   item.ID,
		EntityName: item.Name,
	})
	response.OK(w, item)
}

// HandleDelete removes a catalog. Built-in rows are rejected.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	// Read before the delete: afterwards there is no row left to name.
	name := ""
	if catalog, catalogErr := h.service.Get(r.Context(), id); catalogErr == nil {
		name = catalog.Name
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.NotFoundCode(w, r, i18n.ErrNotFound)
		case errors.Is(err, ErrReadOnly):
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
		default:
			h.app.Logger.Error("catalogs: delete error", "error", err, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		}
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "catalog.deleted",
		EntityType: "catalog",
		EntityID:   id,
		EntityName: name,
	})
	response.NoContent(w)
}

// HandleTestStream probes one catalog and streams a step-by-step
// report over Server-Sent Events. Read-only: it stats a directory or
// issues a single outbound GET and writes nothing back to the row.
//
// The stream is used rather than a plain JSON response because the
// remote branch can spend up to 15 seconds on the fetch alone, and the
// point of the feature is showing the operator which step is taking
// that time.
func (h *Handler) HandleTestStream(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	// Resolve the row before switching the response into SSE mode: once
	// the 200 + event-stream headers are written there is no way back to
	// a 404, and a client asking about a deleted catalog deserves the
	// real status code.
	catalog, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("catalogs: test lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.app.Logger.Error("catalogs: test stream requires a flushable writer", "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Marshal failures are logged and the event dropped rather than
	// aborting the run: a single unserializable step must not cost the
	// operator the four that would have followed it.
	emit := func(event TestEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			h.app.Logger.Error("catalogs: test event marshal failed", "error", err, "id", id)
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}

	result, err := h.service.Test(r.Context(), id, emit)
	if err != nil {
		h.app.Logger.Error("catalogs: test error", "error", err, "id", id)
		emit(TestEvent{Status: "error", Total: len(TestSteps), ErrorCode: string(i18n.ErrSettingsFailed)})
		return
	}

	emit(TestEvent{Status: "done", Index: len(TestSteps), Total: len(TestSteps), Result: &result})

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "catalog.tested",
		EntityType: "catalog",
		EntityID:   catalog.ID,
		EntityName: catalog.Name,
	})
}

// Mount registers /api/catalogs behind the standard RBAC gates.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/catalogs", func(r *apispec.Router) {
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleList)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/", h.HandleCreate, apispec.Request(CreateRequest{}))
			r.Route("/{id}", func(r *apispec.Router) {
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleGet)
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/", h.HandleUpdate, apispec.Request(UpdateRequest{}))
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandleDelete)
				// Gated on manage rather than view: the probe makes an
				// outbound request to an operator-supplied URL, which is
				// the same authority that let them set the URL.
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/test/stream", h.HandleTestStream, apispec.SSE())
			})
		})
	})
}
