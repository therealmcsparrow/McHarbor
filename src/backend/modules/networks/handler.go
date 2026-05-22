// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for network HTTP handlers.
type Handler struct {
	svc *Service
	app *router.AppDeps
}

// NewHandler creates a new network handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc: NewService(app.DockerPool),
		app: app,
	}
}

// HandleList returns all networks.
// GET /networks?env=envId
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	networks, err := h.svc.List(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("list networks failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkListFailed)
		return
	}

	response.OK(w, networks)
}

// HandleCreate creates a new network.
// POST /networks
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkNameRequired)
		return
	}

	resp, err := h.svc.Create(r.Context(), envID, req)
	if err != nil {
		h.app.Logger.Error("create network failed", "env", envID, "name", req.Name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "create",
		EntityType:    "network",
		EntityName:    req.Name,
		EnvironmentID: envID,
	})

	response.Created(w, resp)
}

// HandleInspect returns detailed network information.
// GET /networks/{id}
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	net, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		h.app.Logger.Error("inspect network failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkInspectFailed)
		return
	}

	response.OK(w, net)
}

// HandleRemove removes a network.
// DELETE /networks/{id}
func (h *Handler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.Remove(r.Context(), envID, id); err != nil {
		h.app.Logger.Error("remove network failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkRemoveFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "delete",
		EntityType:    "network",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.NoContent(w)
}

// HandleConnect connects a container to a network.
// POST /networks/{id}/connect
func (h *Handler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req ConnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Container == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkContainerReq)
		return
	}

	if err := h.svc.Connect(r.Context(), envID, id, req); err != nil {
		h.app.Logger.Error("connect to network failed", "env", envID, "network", id, "container", req.Container, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkConnectFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "connect",
		EntityType:    "network",
		EntityID:      id,
		Details:       "container=" + req.Container,
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgNetworkConnected)
}

// HandleDisconnect disconnects a container from a network.
// POST /networks/{id}/disconnect
func (h *Handler) HandleDisconnect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req DisconnectRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Container == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkContainerReq)
		return
	}

	if err := h.svc.Disconnect(r.Context(), envID, id, req); err != nil {
		h.app.Logger.Error("disconnect from network failed", "env", envID, "network", id, "container", req.Container, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNetworkDisconnectFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "disconnect",
		EntityType:    "network",
		EntityID:      id,
		Details:       "container=" + req.Container,
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgNetworkDisconnected)
}
