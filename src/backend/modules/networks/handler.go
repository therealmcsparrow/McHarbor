// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package networks manages Docker networks and reports host network interfaces per environment.
package networks

import (
	"errors"
	"net/http"

	cerrdefs "github.com/containerd/errdefs"
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
		svc: NewService(app.DockerPool, app.AgentPool),
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
		response.InternalErrorCause(w, r, i18n.ErrNetworkListFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkNameRequired)
		return
	}

	resp, err := h.svc.Create(r.Context(), envID, req)
	if err != nil {
		if reason, ok := ipamRefusalReason(err); ok {
			h.app.Logger.Warn("create network: invalid ipam", "env", envID, "name", req.Name, "error", err)
			response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": reason})
			return
		}
		if h.writeHostLinkError(w, r, i18n.ErrNetworkCreateFailed, "create network", envID, err) {
			return
		}
		h.app.Logger.Error("create network failed", "env", envID, "name", req.Name, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkCreateFailed, err)
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrNetworkNotFound)
			return
		}
		h.app.Logger.Error("inspect network failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkInspectFailed, err)
		return
	}

	response.OK(w, net)
}

// HandleHostInterfaces returns the links that can serve as a macvlan or ipvlan
// parent on the selected environment.
// GET /networks/host-interfaces?env=envId
//
// Read-only and never fails on a missing agent: an environment McHarbor cannot
// enumerate returns an empty list with agentAvailable false, which the form
// renders as an explanation rather than as an error.
func (h *Handler) HandleHostInterfaces(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	result, err := h.svc.HostInterfaces(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("list host interfaces failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkListFailed, err)
		return
	}

	response.OK(w, result)
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
		if errors.Is(err, ErrBuiltIn) {
			h.app.Logger.Warn("refused to remove built-in network", "env", envID, "id", id, "error", err)
			response.BadRequestCause(w, r, i18n.ErrNetworkBuiltIn, err)
			return
		}
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrNetworkNotFound)
			return
		}
		if h.writeHostLinkError(w, r, i18n.ErrNetworkRemoveFailed, "remove network", envID, err) {
			return
		}
		h.app.Logger.Error("remove network failed", "env", envID, "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkRemoveFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Container == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkContainerReq)
		return
	}

	if err := h.svc.Connect(r.Context(), envID, id, req); err != nil {
		if h.writeSelfRefusal(w, r, envID, id, req.Container, "connect", err) {
			return
		}
		h.app.Logger.Error("connect to network failed", "env", envID, "network", id, "container", req.Container, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkConnectFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Container == "" {
		response.BadRequestCode(w, r, i18n.ErrNetworkContainerReq)
		return
	}

	if err := h.svc.Disconnect(r.Context(), envID, id, req); err != nil {
		if h.writeSelfRefusal(w, r, envID, id, req.Container, "disconnect", err) {
			return
		}
		h.app.Logger.Error("disconnect from network failed", "env", envID, "network", id, "container", req.Container, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrNetworkDisconnectFailed, err)
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
