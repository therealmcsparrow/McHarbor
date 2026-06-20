// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for environment HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new environment handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.DockerPool, app.KubernetesPool, app.AgentPool, app.Encryption)
	return &Handler{app: app, service: svc}
}

// HandleList returns all environments.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envs, err := h.service.List()
	if err != nil {
		h.app.Logger.Error("failed to list environments", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, envs)
}

// HandleGet returns a single environment by ID.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	env, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("failed to get environment", "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	response.OK(w, env)
}

// HandleCreate creates a new environment.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNameRequired)
		return
	}

	validOrchestrators := map[string]bool{"docker": true, "kubernetes": true}
	if req.OrchestratorType != "" && !validOrchestrators[req.OrchestratorType] {
		response.BadRequestCode(w, r, i18n.ErrEnvInvalidOrchestrator)
		return
	}

	if req.OrchestratorType != "kubernetes" {
		validTypes := map[string]bool{"socket": true, "tcp": true, "tls": true, "ssh": true, "podman": true, "agent": true}
		if req.ConnectionType != "" && !validTypes[req.ConnectionType] {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidConnType)
			return
		}
	}

	env, agentToken, err := h.service.Create(req)
	if err != nil {
		h.app.Logger.Error("failed to create environment", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "environment",
		EntityID:   env.ID,
		EntityName: env.Name,
	})

	h.app.Logger.Info("environment created", "id", env.ID, "name", env.Name, "user", user.Username)

	// For agent connections, include the plaintext token (shown once)
	if agentToken != "" {
		response.Created(w, map[string]interface{}{
			"environment": env,
			"agentToken":  agentToken,
		})
		return
	}
	response.Created(w, env)
}

// HandleUpdate updates an existing environment.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")

	var req UpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.OrchestratorType != nil {
		validOrchestrators := map[string]bool{"docker": true, "kubernetes": true}
		if !validOrchestrators[*req.OrchestratorType] {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidOrchestrator)
			return
		}
	}

	if req.ConnectionType != nil {
		validTypes := map[string]bool{"socket": true, "tcp": true, "tls": true, "ssh": true, "podman": true, "agent": true}
		if !validTypes[*req.ConnectionType] {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidConnType)
			return
		}
	}
	if req.Timezone != nil {
		trimmed := strings.TrimSpace(*req.Timezone)
		if trimmed == "" {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidTimezone)
			return
		}
		if _, err := time.LoadLocation(trimmed); err != nil {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidTimezone)
			return
		}
		*req.Timezone = trimmed
	}
	if req.DockerDiskUsageThresholdPercent != nil {
		switch {
		case *req.DockerDiskUsageThresholdPercent < 1:
			*req.DockerDiskUsageThresholdPercent = 1
		case *req.DockerDiskUsageThresholdPercent > 100:
			*req.DockerDiskUsageThresholdPercent = 100
		}
	}
	if req.LogRetentionDays != nil {
		switch {
		case *req.LogRetentionDays < 0:
			*req.LogRetentionDays = 0
		case *req.LogRetentionDays > 3650:
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidLogRetention)
			return
		}
	}
	if req.ContainerPruneStoppedDays != nil {
		switch {
		case *req.ContainerPruneStoppedDays < 1:
			*req.ContainerPruneStoppedDays = 1
		case *req.ContainerPruneStoppedDays > 3650:
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidContainerPrune)
			return
		}
	}
	if req.MetricRetentionHours != nil {
		switch {
		case *req.MetricRetentionHours < 0:
			*req.MetricRetentionHours = 0
		case *req.MetricRetentionHours > 8760:
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidMetricRetention)
			return
		}
	}
	if req.AutoUpdateWindowStart != nil {
		if !isValidClockTime(*req.AutoUpdateWindowStart) {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidAutoUpdate)
			return
		}
	}
	if req.AutoUpdateWindowEnd != nil {
		if !isValidClockTime(*req.AutoUpdateWindowEnd) {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidAutoUpdate)
			return
		}
	}
	if req.AutoUpdateDays != nil {
		normalized, ok := normalizeAutoUpdateDays(*req.AutoUpdateDays)
		if !ok {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidAutoUpdateDays)
			return
		}
		*req.AutoUpdateDays = normalized
	}

	env, err := h.service.Update(id, req)
	if err != nil {
		h.app.Logger.Error("failed to update environment", "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "environment",
		EntityID:   id,
	})

	h.app.Logger.Info("environment updated", "id", id, "user", user.Username)
	response.OK(w, env)
}

// HandleDelete removes an environment.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.service.Delete(id); err != nil {
		h.app.Logger.Error("failed to delete environment", "id", id, "error", err)
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "environment",
		EntityID:   id,
	})

	h.app.Logger.Info("environment deleted", "id", id, "user", user.Username)
	response.NoContent(w)
}

// HandleTestConnection tests the Docker connection for an environment.
func (h *Handler) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")

	// Verify the environment exists
	env, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("failed to get environment for test", "id", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	result := h.service.TestConnection(r.Context(), id)
	h.app.Logger.Info("environment connection test", "id", id, "success", result.Success, "user", user.Username)
	response.OK(w, result)
}

// HandleDetectSocket auto-detects available Docker/Podman sockets.
func (h *Handler) HandleDetectSocket(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	sockets := h.service.DetectSocket()
	response.OK(w, sockets)
}

// isValidClockTime reports whether value matches a 24-hour HH:MM clock time.
func isValidClockTime(value string) bool {
	_, err := time.Parse("15:04", strings.TrimSpace(value))
	return err == nil
}

// normalizeAutoUpdateDays validates and normalizes the comma-separated days-of-week list.
// It returns a canonical lowercase, deduplicated, ordered list ("mon,tue,...,sun") or false when invalid.
func normalizeAutoUpdateDays(value string) (string, bool) {
	allowed := map[string]struct{}{
		"mon": {}, "tue": {}, "wed": {}, "thu": {}, "fri": {}, "sat": {}, "sun": {},
	}
	order := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	seen := make(map[string]struct{}, 7)
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), ",")
	for _, part := range parts {
		day := strings.TrimSpace(part)
		if day == "" {
			continue
		}
		if _, ok := allowed[day]; !ok {
			return "", false
		}
		seen[day] = struct{}{}
	}
	if len(seen) == 0 {
		return "", false
	}
	normalized := make([]string, 0, len(seen))
	for _, day := range order {
		if _, ok := seen[day]; ok {
			normalized = append(normalized, day)
		}
	}
	return strings.Join(normalized, ","), true
}
