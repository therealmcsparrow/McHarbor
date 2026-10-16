// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler exposes /api/scim/endpoints admin CRUD + sync-now + history.
// SCIM-protocol handlers under /api/scim/v2/* live in users.go and
// groups.go and are mounted under RegisterPublicRoutes with bearer
// auth so external IdPs (or our own poller) can hit them.
type Handler struct {
	app     *router.AppDeps
	service *Service
	puller  *Puller
}

// NewHandler wires up the SCIM admin handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption),
		puller:  NewPuller(),
	}
}

// HandleList returns every configured SCIM endpoint.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ListEndpoints(r.Context())
	if err != nil {
		h.app.Logger.Error("scim: list endpoints", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if out == nil {
		out = []Endpoint{}
	}
	response.OK(w, out)
}

// HandleGet returns a single endpoint by id.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := h.service.EndpointByID(r.Context(), id)
	if errors.Is(err, ErrEndpointNotFound) {
		response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("scim: get endpoint", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, ep)
}

// HandleCreate validates input and inserts a new endpoint. If the
// caller did not supply a bearer token, the service generates a
// fresh 32-byte token and returns the plaintext once via
// `bearerToken` in the response body.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input EndpointInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	ep, plaintext, err := h.service.CreateEndpoint(r.Context(), input, user.ID)
	if err != nil {
		h.respondCreateError(w, r, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "create",
		EntityType: "scim_endpoint",
		EntityID:   ep.ID,
		EntityName: ep.Name,
	})

	if plaintext == "" {
		response.Created(w, ep)
		return
	}
	// Surface the freshly generated token exactly once.
	response.Created(w, map[string]any{
		"endpoint":    ep,
		"bearerToken": plaintext,
	})
}

// HandleUpdate applies changes to an existing endpoint. Tokens are
// only overwritten when the caller supplies a non-empty
// bearerToken in the body.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	var input EndpointInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	ep, err := h.service.UpdateEndpoint(r.Context(), id, input)
	if errors.Is(err, ErrEndpointNotFound) {
		response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("scim: update endpoint", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "update",
		EntityType: "scim_endpoint",
		EntityID:   ep.ID,
		EntityName: ep.Name,
	})
	response.OK(w, ep)
}

// HandleDelete removes an endpoint and (via cascade) its sync log.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	// Read before the delete: afterwards there is no endpoint left to name.
	name := ""
	if endpoint, endpointErr := h.service.EndpointByID(r.Context(), id); endpointErr == nil && endpoint != nil {
		name = endpoint.Name
	}

	if err := h.service.DeleteEndpoint(r.Context(), id); err != nil {
		if errors.Is(err, ErrEndpointNotFound) {
			response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
			return
		}
		h.app.Logger.Error("scim: delete endpoint", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "delete",
		EntityType: "scim_endpoint",
		EntityID:   id,
		EntityName: name,
	})
	response.OKMsg(w, r, i18n.MsgSCIMEndpointRemoved)
}

// HandleSyncNow triggers an immediate pull for the endpoint. The
// poller runs in the background; this endpoint merely enqueues a
// "force now" marker that the poller's loop will pick up on its
// next tick. We avoid starting a one-shot goroutine per request
// because the poller holds the only sync lock per endpoint.
func (h *Handler) HandleSyncNow(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")

	endpoint, err := h.service.EndpointByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrEndpointNotFound) {
			response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
			return
		}
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if err := h.service.RequestSyncNow(r.Context(), id); err != nil {
		h.app.Logger.Error("scim: request sync-now", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "sync_now",
		EntityType: "scim_endpoint",
		EntityID:   id,
		EntityName: endpoint.Name,
	})
	response.OKMsg(w, r, i18n.MsgSCIMSynced)
}

// HandleHistory returns the most-recent sync log entries for an
// endpoint. Honours ?limit=N (default 50, max 1000).
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	entries, err := h.service.SyncHistory(r.Context(), id, limit)
	if err != nil {
		h.app.Logger.Error("scim: history", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if entries == nil {
		entries = []SyncLogEntry{}
	}
	response.OK(w, entries)
}

// HandleDryRun pulls the remote directory and returns the plan a real sync
// would apply, writing nothing but a dry-run row in the history. A failed pull
// is still a 200 with status "failed" so the preview can show the reason.
func (h *Handler) HandleDryRun(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	endpoint, err := h.service.EndpointByID(r.Context(), id)
	if errors.Is(err, ErrEndpointNotFound) {
		response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("scim: dry-run lookup", "error", err, "endpoint", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	result := DryRunResult{Status: "success"}
	plan, planErr := h.service.PlanSync(r.Context(), h.puller, id)
	run := SyncRun{DryRun: true, Trigger: TriggerPreview, ActorID: user.ID}
	if planErr != nil {
		h.app.Logger.Error("scim: dry-run", "error", planErr, "endpoint", id)
		result.Status = "failed"
		result.Error = planErr.Error()
		run.Status = "failed"
		run.Error = planErr.Error()
	} else {
		result.Plan = plan
		run.Status = "success"
		run.Summary = plan.Summary
		run.Notes = PlanNotes(plan)
	}
	if err := h.service.RecordRun(r.Context(), id, run); err != nil {
		h.app.Logger.Error("scim: record dry-run", "error", err, "endpoint", id)
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "dry_run",
		EntityType: "scim_endpoint",
		EntityID:   id,
		EntityName: endpoint.Name,
	})
	response.OK(w, result)
}

// HandleRotateToken issues a new bearer token and revokes the old one in the
// same write. The plaintext is in this response and nowhere else, ever.
func (h *Handler) HandleRotateToken(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	plaintext, endpoint, err := h.service.RotateToken(r.Context(), id)
	switch {
	case errors.Is(err, ErrEndpointNotFound):
		response.NotFoundCode(w, r, i18n.ErrSCIMEndpointNotFound)
		return
	case errors.Is(err, ErrTokenRotationUnsupported):
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "auth_type_not_bearer"})
		return
	case err != nil:
		h.app.Logger.Error("scim: rotate token", "error", err, "endpoint", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "rotate_token",
		EntityType: "scim_endpoint",
		EntityID:   id,
		EntityName: endpoint.Name,
		Severity:   "warning",
	})
	response.OK(w, RotatedToken{Endpoint: endpoint, BearerToken: plaintext})
}

// respondCreateError maps service errors from CreateEndpoint to the
// right i18n code + status. We avoid leaking the underlying error
// message to the client (per project rules — never expose err.Error()).
func (h *Handler) respondCreateError(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	switch msg {
	case "endpoint name is required", "endpoint baseUrl is required":
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
	case "endpoint name already taken":
		response.ConflictCode(w, r, i18n.ErrSCIMEndpointNameTaken)
	default:
		h.app.Logger.Error("scim: create endpoint", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
}
