// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// HandleConfigList returns Swarm configs without their payloads.
// GET /swarm/configs?env=envId
func (h *Handler) HandleConfigList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	configs, err := h.svc.ListConfigs(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm config list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmConfigListFailed)
		return
	}

	response.OK(w, configs)
}

// HandleConfigInspect returns one config including its payload.
// GET /swarm/configs/{id}?env=envId
func (h *Handler) HandleConfigInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	config, err := h.svc.InspectConfig(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm config inspect failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmConfigNotFound)
		return
	}

	// The payload travels only here, so it must not sit in a shared cache.
	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, config)
}

// HandleConfigCreate stores a new config.
// POST /swarm/configs?env=envId
func (h *Handler) HandleConfigCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req PayloadWriteRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmConfigNameRequired)
		return
	}
	if req.Data == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmConfigPayloadInvalid)
		return
	}

	id, err := h.svc.CreateConfig(r.Context(), envID, req)
	if err != nil {
		h.logger.Error("swarm config create failed", "env", envID, "name", req.Name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmConfigCreateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "create",
		EntityType:    "swarm_config",
		EntityID:      id,
		EntityName:    req.Name,
		EnvironmentID: envID,
	})

	response.Created(w, map[string]string{"id": id})
}

// HandleConfigUpdate updates a config's labels.
// PUT /swarm/configs/{id}?env=envId
func (h *Handler) HandleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req LabelsWriteRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.svc.UpdateConfigLabels(r.Context(), envID, id, req.Labels); err != nil {
		h.logger.Error("swarm config update failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmConfigUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "update",
		EntityType:    "swarm_config",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleConfigRemove deletes a config.
// DELETE /swarm/configs/{id}?env=envId
func (h *Handler) HandleConfigRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.RemoveConfig(r.Context(), envID, id); err != nil {
		h.logger.Error("swarm config remove failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmConfigRemoveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "delete",
		Severity:      "warning",
		EntityType:    "swarm_config",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"removed": true})
}
