// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// HandleStackList returns the Swarm stacks.
// GET /swarm/stacks?env=envId
func (h *Handler) HandleStackList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	stacks, err := h.svc.ListStacks(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm stack list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmStackListFailed)
		return
	}

	response.OK(w, stacks)
}

// HandleStackInspect returns one stack and its services.
// GET /swarm/stacks/{name}?env=envId
func (h *Handler) HandleStackInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	name := chi.URLParam(r, "name")

	stack, err := h.svc.InspectStack(r.Context(), envID, name)
	if err != nil {
		h.logger.Error("swarm stack inspect failed", "env", envID, "name", name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmStackNotFound)
		return
	}

	response.OK(w, stack)
}

// HandleStackDeploy deploys a Compose document to the cluster.
// POST /swarm/stacks?env=envId
//
// A partially-applied deploy answers 207-style: the result body carries which
// services were created, updated and rejected, so the UI can name what failed
// instead of showing one opaque error over a stack that is half up.
func (h *Handler) HandleStackDeploy(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req DeployStackRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = composeDocumentName(req.Compose)
	}
	if name == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmStackNameRequired)
		return
	}
	if strings.TrimSpace(req.Compose) == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmStackComposeRequired)
		return
	}

	result, err := h.svc.DeployStack(r.Context(), envID, name, req.Compose, req.Prune)
	if err != nil {
		if errors.Is(err, ErrStackRejected) {
			// This is the operator's file being wrong, not the server failing:
			// answer 400 with the per-service reasons attached.
			h.audit(r, auditEntry{
				Action:        "deploy_rejected",
				EntityType:    "swarm_stack",
				EntityName:    name,
				EnvironmentID: envID,
			})
			response.BadRequestCodeWithData(w, r, i18n.ErrSwarmStackRejected, result)
			return
		}
		h.logger.Error("swarm stack deploy failed", "env", envID, "name", name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmStackDeployFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "deploy",
		EntityType:    "swarm_stack",
		EntityName:    name,
		Details:       stackDetail(result),
		EnvironmentID: envID,
	})

	response.OK(w, result)
}

// HandleStackRemove deletes a stack.
// DELETE /swarm/stacks/{name}?env=envId
func (h *Handler) HandleStackRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	name := chi.URLParam(r, "name")

	if err := h.svc.RemoveStack(r.Context(), envID, name); err != nil {
		h.logger.Error("swarm stack remove failed", "env", envID, "name", name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmStackRemoveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "delete",
		Severity:      "warning",
		EntityType:    "swarm_stack",
		EntityName:    name,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"removed": true})
}

// HandleNetworkList returns the overlay networks.
// GET /swarm/networks?env=envId
func (h *Handler) HandleNetworkList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	networks, err := h.svc.ListOverlayNetworks(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm network list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmNetworkListFailed)
		return
	}

	response.OK(w, networks)
}

// stackDetail summarises a deploy for the audit row.
func stackDetail(result DeployStackResult) string {
	return "created=" + itoa(len(result.Created)) +
		" updated=" + itoa(len(result.Updated)) +
		" removed=" + itoa(len(result.Removed))
}

// itoa is a tiny local helper so the audit detail builder does not pull in
// strconv for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
