// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the caller's own table views; the user always comes from the session,
// never from the path, which is why the routes need no permission key.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler returns a Handler backed by app's database.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB)}
}

func (h *Handler) scope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return "", "", false
	}
	tableID := chi.URLParam(r, "tableId")
	if !ValidTableID(tableID) {
		response.BadRequestCode(w, r, i18n.ErrTableViewInvalid)
		return "", "", false
	}
	return user.ID, tableID, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrTableViewNotFound)
	case errors.Is(err, ErrInvalid):
		response.BadRequestCode(w, r, i18n.ErrTableViewInvalid)
	case errors.Is(err, ErrLimit):
		response.ConflictCode(w, r, i18n.ErrTableViewLimit)
	case errors.Is(err, ErrNameTaken):
		response.ConflictCode(w, r, i18n.ErrTableViewNameTaken)
	default:
		h.app.Logger.Error("table_views: "+op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrTableViewFailed, err)
	}
}

// HandleGet returns the caller's layout and saved filters for one table.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	view, err := h.service.View(r.Context(), userID, tableID)
	if err != nil {
		h.fail(w, r, "get", err)
		return
	}
	response.OK(w, view)
}

// HandlePutLayout stores the caller's layout; it is not audited because every column
// resize writes one.
func (h *Handler) HandlePutLayout(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxPayloadBytes+1024)
	var payload json.RawMessage
	if err := response.DecodeBody(r, &payload); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	layout, err := h.service.PutLayout(r.Context(), userID, tableID, payload)
	if err != nil {
		h.fail(w, r, "put layout", err)
		return
	}
	response.OK(w, layout)
}

// HandleDeleteLayout resets the caller's layout to the page defaults.
func (h *Handler) HandleDeleteLayout(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteLayout(r.Context(), userID, tableID); err != nil {
		h.fail(w, r, "delete layout", err)
		return
	}
	response.NoContent(w)
}

// HandleCreateFilter saves a named filter for the caller.
func (h *Handler) HandleCreateFilter(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxPayloadBytes+1024)
	var in FilterInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	f, err := h.service.CreateFilter(r.Context(), userID, tableID, in)
	if err != nil {
		h.fail(w, r, "create filter", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "create", EntityType: "table_filter", EntityID: f.ID, EntityName: f.Name, Details: "table " + tableID})
	response.Created(w, f)
}

// HandleUpdateFilter renames, re-captures or (un)defaults one of the caller's filters.
func (h *Handler) HandleUpdateFilter(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxPayloadBytes+1024)
	var in FilterUpdate
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	f, err := h.service.UpdateFilter(r.Context(), userID, tableID, chi.URLParam(r, "id"), in)
	if err != nil {
		h.fail(w, r, "update filter", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "update", EntityType: "table_filter", EntityID: f.ID, EntityName: f.Name, Details: "table " + tableID})
	response.OK(w, f)
}

// HandleDeleteFilter removes one of the caller's filters.
func (h *Handler) HandleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	userID, tableID, ok := h.scope(w, r)
	if !ok {
		return
	}
	f, err := h.service.DeleteFilter(r.Context(), userID, tableID, chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, "delete filter", err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "delete", EntityType: "table_filter", EntityID: f.ID, EntityName: f.Name, Details: "table " + tableID})
	response.NoContent(w)
}

// HandleImport copies the caller's browser-stored table state to the server once.
func (h *Handler) HandleImport(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var in ImportInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	n, err := h.service.Import(r.Context(), user.ID, in)
	if err != nil {
		h.fail(w, r, "import", err)
		return
	}
	if n > 0 {
		h.app.AuditLog.Log(r, audit.Entry{Action: "import", EntityType: "table_filter", Details: "imported browser table views"})
	}
	response.OK(w, map[string]int{"imported": n})
}
