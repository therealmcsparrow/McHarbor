// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// HandleNodeList returns every node in the cluster.
// GET /swarm/nodes?env=envId
func (h *Handler) HandleNodeList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	nodes, err := h.svc.ListNodes(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm node list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeListFailed)
		return
	}

	response.OK(w, nodes)
}

// HandleNodeInspect returns one node.
// GET /swarm/nodes/{id}?env=envId
func (h *Handler) HandleNodeInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	node, err := h.svc.InspectNode(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm node inspect failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeNotFound)
		return
	}

	response.OK(w, node)
}

// HandleNodeLabels replaces a node's labels.
// PUT /swarm/nodes/{id}?env=envId
func (h *Handler) HandleNodeLabels(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req NodeLabelsRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.svc.SetNodeLabels(r.Context(), envID, id, req.Labels); err != nil {
		h.logger.Error("swarm node labels failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "update_labels",
		EntityType:    "swarm_node",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleNodeAvailability moves a node between active, pause and drain.
// POST /swarm/nodes/{id}/availability?env=envId
func (h *Handler) HandleNodeAvailability(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req NodeAvailabilityRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Availability)) {
	case "active", "pause", "drain":
	default:
		response.BadRequestCode(w, r, i18n.ErrSwarmNodeAvailabilityInvalid)
		return
	}

	if err := h.svc.SetNodeAvailability(r.Context(), envID, id, req.Availability); err != nil {
		h.logger.Error("swarm node availability failed", "env", envID, "id", id,
			"availability", req.Availability, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "set_availability",
		Severity:      "warning",
		EntityType:    "swarm_node",
		EntityID:      id,
		Details:       "availability=" + req.Availability,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleNodeRole promotes or demotes a node.
// POST /swarm/nodes/{id}/role?env=envId
func (h *Handler) HandleNodeRole(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req NodeRoleRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Role)) {
	case "manager", "worker":
	default:
		response.BadRequestCode(w, r, i18n.ErrSwarmNodeRoleInvalid)
		return
	}

	if err := h.svc.SetNodeRole(r.Context(), envID, id, req.Role); err != nil {
		h.logger.Error("swarm node role failed", "env", envID, "id", id,
			"role", req.Role, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "set_role",
		Severity:      "warning",
		EntityType:    "swarm_node",
		EntityID:      id,
		Details:       "role=" + req.Role,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleNodeRemove removes a node from the cluster.
// DELETE /swarm/nodes/{id}?env=envId&force=true
func (h *Handler) HandleNodeRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "true"

	if err := h.svc.RemoveNode(r.Context(), envID, id, force); err != nil {
		h.logger.Error("swarm node remove failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNodeRemoveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "delete",
		Severity:      "critical",
		EntityType:    "swarm_node",
		EntityID:      id,
		Details:       boolDetail("force", force),
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"removed": true})
}
