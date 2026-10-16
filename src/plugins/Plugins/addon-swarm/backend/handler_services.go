// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// HandleServiceList returns every Swarm service.
// GET /swarm/services?env=envId
func (h *Handler) HandleServiceList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	services, err := h.svc.ListServices(r.Context(), envID)
	if err != nil {
		h.logRead(err, "swarm service list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceListFailed)
		return
	}

	response.OK(w, services)
}

// HandleServiceInspect returns one service.
// GET /swarm/services/{id}?env=envId
func (h *Handler) HandleServiceInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	detail, err := h.svc.InspectService(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm service inspect failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceNotFound)
		return
	}

	response.OK(w, detail)
}

// HandleServiceEnv returns a service's environment with credential-looking values in the
// clear. The manifest gates it on swarm_services.manage and audits every call.
// GET /swarm/services/{id}/env?env=envId
func (h *Handler) HandleServiceEnv(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	res, err := h.svc.ServiceEnv(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm service env read failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceNotFound)
		return
	}
	h.auditEnvReveal(r, envID, id, res)

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, map[string][]string{"env": res.Env})
}

// auditActor is the host's actor-attributing audit handle (plugin.AuditActorHandle),
// asserted on the sink so a host without it falls back to Emit.
type auditActor interface {
	EmitEvent(ev plugin.AuditEvent) error
}

// auditEnvReveal writes the reveal's own row at warning severity, naming the service, its
// environment and what stayed withheld; the manifest's fallback row carries none of that.
func (h *Handler) auditEnvReveal(r *http.Request, envID, id string, res ServiceEnvResult) {
	details := "env values revealed"
	if len(res.Withheld) > 0 {
		details += "; still withheld: " + strings.Join(res.Withheld, ",")
	}
	actor, ok := h.audits.(auditActor)
	if !ok {
		h.audit(r, auditEntry{Action: "env_revealed", Severity: "warning", EntityType: "service",
			EntityID: id, EntityName: res.Name, Details: details, EnvironmentID: envID})
		return
	}
	userID := ""
	if user := auth.RequireAuth(r); user != nil {
		userID = user.ID
	}
	if err := actor.EmitEvent(plugin.AuditEvent{
		Action: "plugin.swarm.service.env_revealed", UserID: userID, Severity: "warning",
		Fields: map[string]any{
			"entity_type": "swarm_service", "entity_id": id, "entity_name": res.Name,
			"environment_id": envID, "details": details,
		},
	}); err != nil {
		h.logger.Warn("swarm: writing the env reveal audit row failed", "env", envID, "id", id, "error", err)
	}
}

// HandleServiceLogs returns the aggregated logs of a service's tasks.
// GET /swarm/services/{id}/logs?env=envId&tail=200
func (h *Handler) HandleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	query, err := parseLogsQuery(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	logs, err := h.svc.ServiceLogs(r.Context(), envID, id, query)
	if err != nil {
		h.logger.Error("swarm service logs failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceLogsFailed)
		return
	}

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, map[string]string{"logs": logs})
}

// HandleServiceCreate creates a service from the structured form.
// POST /swarm/services?env=envId
func (h *Handler) HandleServiceCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req ServiceWriteRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmServiceNameRequired)
		return
	}
	if req.Image == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmServiceImageRequired)
		return
	}
	if req.Replicas > maxReplicas {
		response.BadRequestCode(w, r, i18n.ErrSwarmReplicasInvalid)
		return
	}

	id, err := h.svc.CreateService(r.Context(), envID, req)
	if err != nil {
		h.logger.Error("swarm service create failed", "env", envID, "name", req.Name, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceCreateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "create",
		EntityType:    "swarm_service",
		EntityID:      id,
		EntityName:    req.Name,
		Details:       "image=" + req.Image,
		EnvironmentID: envID,
	})

	response.Created(w, map[string]string{"id": id})
}

// HandleServiceUpdate applies the structured form to an existing service.
// PUT /swarm/services/{id}?env=envId
func (h *Handler) HandleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req ServiceWriteRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Replicas > maxReplicas {
		response.BadRequestCode(w, r, i18n.ErrSwarmReplicasInvalid)
		return
	}

	if err := h.svc.UpdateService(r.Context(), envID, id, req); err != nil {
		if errors.Is(err, ErrMaskedValueMoved) {
			h.logger.Warn("swarm service update refused: a withheld value would move", "env", envID, "id", id, "error", err)
			response.BadRequestCause(w, r, i18n.ErrSwarmServiceMaskedMoved, err)
			return
		}
		h.logger.Error("swarm service update failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "update",
		EntityType:    "swarm_service",
		EntityID:      id,
		EntityName:    req.Name,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleServiceScale changes a service's replica count.
// POST /swarm/services/{id}/scale?env=envId
func (h *Handler) HandleServiceScale(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var req ScaleRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Replicas > maxReplicas {
		response.BadRequestCode(w, r, i18n.ErrSwarmReplicasInvalid)
		return
	}

	if err := h.svc.ScaleService(r.Context(), envID, id, req.Replicas); err != nil {
		h.logger.Error("swarm service scale failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceScaleFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "scale",
		EntityType:    "swarm_service",
		EntityID:      id,
		Details:       "replicas=" + strconv.FormatUint(req.Replicas, 10),
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"scaled": true})
}

// HandleServiceRestart forces every task of a service to be recreated.
// POST /swarm/services/{id}/restart?env=envId
func (h *Handler) HandleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.RestartService(r.Context(), envID, id); err != nil {
		h.logger.Error("swarm service restart failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceRestartFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "restart",
		EntityType:    "swarm_service",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"restarted": true})
}

// HandleServiceRollback returns a service to its previous spec.
// POST /swarm/services/{id}/rollback?env=envId
func (h *Handler) HandleServiceRollback(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.RollbackService(r.Context(), envID, id); err != nil {
		h.logger.Error("swarm service rollback failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceRollbackFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "rollback",
		Severity:      "warning",
		EntityType:    "swarm_service",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"rolledBack": true})
}

// HandleServiceRemove deletes a service.
// DELETE /swarm/services/{id}?env=envId
func (h *Handler) HandleServiceRemove(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	if err := h.svc.RemoveService(r.Context(), envID, id); err != nil {
		h.logger.Error("swarm service remove failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmServiceRemoveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "delete",
		Severity:      "warning",
		EntityType:    "swarm_service",
		EntityID:      id,
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"removed": true})
}

// parseLogsQuery reads the shared log query parameters, bounding tail so a
// crafted request cannot ask the daemon for an unbounded history.
func parseLogsQuery(r *http.Request) (LogsQuery, error) {
	q := LogsQuery{
		Stdout: r.URL.Query().Get("stdout") != "false",
		Stderr: r.URL.Query().Get("stderr") != "false",
		Since:  r.URL.Query().Get("since"),
	}

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	count, err := strconv.Atoi(tail)
	if err != nil || count < 1 || count > 5000 {
		return LogsQuery{}, errInvalidTail
	}
	q.Tail = strconv.Itoa(count)

	return q, nil
}
