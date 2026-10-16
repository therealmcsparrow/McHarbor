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

// HandleSecretList returns Swarm secret metadata.
// GET /swarm/secrets?env=envId
func (h *Handler) HandleSecretList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	secrets, err := h.svc.ListSecrets(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm secret list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmSecretListFailed)
		return
	}

	response.OK(w, secrets)
}

// HandleSecretInspect returns one secret's metadata.
// GET /swarm/secrets/{id}?env=envId
//
// There is no payload in this response and no endpoint that returns one:
// Docker's secret API is write-only for the value.
func (h *Handler) HandleSecretInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	secret, err := h.svc.InspectSecret(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm secret inspect failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmSecretNotFound)
		return
	}

	response.OK(w, secret)
}

// HandleSecretCreate stores a new secret.
// POST /swarm/secrets?env=envId
func (h *Handler) HandleSecretCreate(w http.ResponseWriter, r *http.Request) {
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
		response.BadRequestCode(w, r, i18n.ErrSwarmSecretNameRequired)
		return
	}
	if req.Data == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmSecretPayloadInvalid)
		return
	}

	id, err := h.svc.CreateSecret(r.Context(), envID, req)
	if err != nil {
		// The payload never reaches the log line — only the name does.
		h.logger.Error("swarm secret create failed", "env", envID, "name", req.Name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmSecretCreateFailed)
		return
	}

	// The audit detail carries the name and nothing else: an audit row is
	// readable by every holder of audit.view, which is a wider set than the
	// people entitled to the secret's value.
	h.audit(r, auditEntry{
		Action:        "create",
		Severity:      "warning",
		EntityType:    "swarm_secret",
		EntityID:      id,
		EntityName:    req.Name,
		EnvironmentID: envID,
	})

	response.Created(w, map[string]string{"id": id})
}

// HandleSecretUpdate updates a secret's labels.
// PUT /swarm/secrets/{id}?env=envId
func (h *Handler) HandleSecretUpdate(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.UpdateSecretLabels(r.Context(), envID, id, req.Labels); err != nil {
		h.logger.Error("swarm secret update failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmSecretUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "update",
		EntityType:    "swarm_secret",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleSecretRemove deletes a secret.
// DELETE /swarm/secrets/{id}?env=envId
func (h *Handler) HandleSecretRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.RemoveSecret(r.Context(), envID, id); err != nil {
		h.logger.Error("swarm secret remove failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmSecretRemoveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "delete",
		Severity:      "warning",
		EntityType:    "swarm_secret",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"removed": true})
}
