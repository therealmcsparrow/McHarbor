// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
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
	svc := NewService(app.DB, app.DockerPool, app.AgentPool, app.Encryption)
	return &Handler{app: app, service: svc}
}

// HandleList returns all environments.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envs, err := h.service.ListContext(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to list environments", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if !h.canSeeConnection(r, user) {
		for i := range envs {
			redactConnection(&envs[i])
		}
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
	env, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to get environment", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	if !h.canSeeConnection(r, user) {
		redactConnection(env)
	}
	response.OK(w, env)
}

// canSeeConnection reports whether the caller may see where an environment
// connects. Only environments.manage holders can change it, so only they need it.
func (h *Handler) canSeeConnection(r *http.Request, user *auth.User) bool {
	if user.ID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		return false
	}
	allowed, err := h.app.RBACService.HasPermission(r.Context(), user.ID, "", rbac.PermEnvironmentsManage)
	if err != nil {
		h.app.Logger.Error("environments: permission check for connection details failed", "user", user.ID, "error", err)
		return false
	}
	return allowed
}

// redactConnection removes where an environment connects — its Docker socket
// path, TCP target and SSH target — for callers who may only view it.
func redactConnection(env *Environment) {
	env.SocketPath = nil
	env.Host = nil
	env.Port = nil
	env.SSHHost = nil
	env.SSHPort = nil
	env.SSHUser = nil
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNameRequired)
		return
	}

	validTypes := map[string]bool{"socket": true, "tcp": true, "tls": true, "ssh": true, "agent": true}
	if req.ConnectionType != "" && !validTypes[req.ConnectionType] {
		response.BadRequestCode(w, r, i18n.ErrEnvInvalidConnType)
		return
	}

	env, agentToken, err := h.service.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidAgentToken) {
			response.BadRequestCode(w, r, i18n.ErrEnvInvalidAgentToken)
			return
		}
		h.app.Logger.Error("failed to create environment", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	entry := audit.Entry{
		Action:     "create",
		EntityType: "environment",
		EntityID:   env.ID,
		EntityName: env.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(nil, env); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("environment audit diff error", "error", diffErr, "id", env.ID)
	}
	h.app.AuditLog.Log(r, entry)

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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.ConnectionType != nil {
		validTypes := map[string]bool{"socket": true, "tcp": true, "tls": true, "ssh": true, "agent": true}
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
			response.BadRequestCause(w, r, i18n.ErrEnvInvalidTimezone, err)
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

	existing, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to get environment before update", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	env, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.app.Logger.Error("failed to update environment", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	entry := audit.Entry{
		Action:     "update",
		EntityType: "environment",
		EntityID:   id,
		EntityName: env.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, env); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("environment audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

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
	existing, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to get environment before delete", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("failed to delete environment", "id", id, "error", err)
		response.NotFoundCause(w, r, i18n.ErrEnvNotFound, err)
		return
	}

	entry := audit.Entry{
		Action:     "delete",
		EntityType: "environment",
		EntityID:   id,
		EntityName: existing.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, nil); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("environment audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	h.app.Logger.Info("environment deleted", "id", id, "user", user.Username)
	response.NoContent(w)
}

// HandleTestConnection tests the Docker connection for
// an environment and returns a per-step report. The frontend dialog
// renders the steps with status badges (pass / fail / warn / skip)
// and an overall rolled-up badge.
func (h *Handler) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")

	result := h.service.TestConnection(r.Context(), id)
	h.app.Logger.Info(
		"environment connection test",
		"id", id,
		"overall", result.Overall,
		"user", user.Username,
	)
	response.OK(w, result)
}

// HandleDetectSocket auto-detects available Docker sockets.
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

// HandleMigrateConnection converts an environment between connection types.
//
// Separate from HandleUpdate because it is not a field edit: it mints or
// revokes a credential and can drop a live agent connection.
func (h *Handler) HandleMigrateConnection(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req ConnectionMigrationRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	id := chi.URLParam(r, "id")
	before, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to read environment before migration", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if before == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	result, err := h.service.MigrateConnection(r.Context(), id, req, MigrationContext{
		// The address the browser reached McHarbor on is the address a deployed
		// agent has to dial back on, which is the same source the install-token
		// path uses.
		ServerURL: httpx.BaseURL(r),
		ForceTLS:  httpx.WebSocketScheme(r) == "wss",
	})
	switch {
	case errors.Is(err, ErrSameConnectionType):
		response.ConflictCode(w, r, i18n.ErrEnvSameConnType)
		return
	case errors.Is(err, ErrUnknownTargetType):
		response.BadRequestCode(w, r, i18n.ErrEnvInvalidConnType)
		return
	case errors.Is(err, ErrMissingSocketPath), errors.Is(err, ErrMissingTCPHost),
		errors.Is(err, ErrMissingSSHHost):
		response.BadRequestCode(w, r, i18n.ErrEnvConnRequired)
		return
	case errors.Is(err, ErrInvalidAgentToken):
		response.BadRequestCode(w, r, i18n.ErrEnvInvalidAgentToken)
		return
	case err != nil:
		h.app.Logger.Error("failed to migrate environment connection", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrEnvUpdateFailed, err)
		return
	case result == nil:
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	entry := audit.Entry{
		Action:     "migrate",
		EntityType: "environment",
		EntityID:   id,
		EntityName: result.Environment.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(before, result.Environment); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("environment audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	h.app.Logger.Info("environment connection migrated",
		"id", id, "from", before.ConnectionType, "to", req.TargetType,
		"pending", result.Pending, "user", user.Username)
	if result.Agent != nil && result.Agent.Attempted && !result.Agent.Deployed {
		h.app.Logger.Error("agent deploy during migration failed",
			"id", id, "error", result.Agent.Error)
	}

	response.OK(w, result)
}

// HandleCancelMigration disarms a pending connection migration.
func (h *Handler) HandleCancelMigration(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	env, err := h.service.CancelPendingMigration(r.Context(), id)
	switch {
	case errors.Is(err, ErrNoPendingMigration):
		response.ConflictCode(w, r, i18n.ErrEnvNoPendingMigration)
		return
	case err != nil:
		h.app.Logger.Error("failed to cancel environment migration", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrEnvUpdateFailed, err)
		return
	case env == nil:
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "migrate_cancel",
		EntityType: "environment",
		EntityID:   id,
		EntityName: env.Name,
	})
	h.app.Logger.Info("environment migration cancelled", "id", id, "user", user.Username)

	response.OK(w, env)
}

// MergeRequest names the environment whose rows are merged into this one.
type MergeRequest struct {
	SourceID string `json:"sourceId"`
}

// HandleMergeEnvironments folds one environment into another.
//
// ?dryRun=true returns the same plan without writing, which is how the dialog
// shows what a merge would move before anyone consents to it.
func (h *Handler) HandleMergeEnvironments(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req MergeRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(req.SourceID) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	targetID := chi.URLParam(r, "id")
	dryRun := r.URL.Query().Get("dryRun") == "true"

	result, err := h.service.MergeEnvironments(r.Context(), req.SourceID, targetID, dryRun)
	switch {
	case errors.Is(err, ErrMergeIntoSelf):
		response.BadRequestCode(w, r, i18n.ErrEnvMergeIntoSelf)
		return
	case errors.Is(err, ErrMergeSourceGone), errors.Is(err, ErrMergeTargetGone):
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	case errors.Is(err, ErrMergeUnsupportedDriver):
		response.ConflictCode(w, r, i18n.ErrEnvMergeFailed)
		return
	case err != nil:
		h.app.Logger.Error("failed to merge environments",
			"source", req.SourceID, "target", targetID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrEnvMergeFailed, err)
		return
	}

	if !dryRun {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "merge",
			EntityType: "environment",
			EntityID:   targetID,
			EntityName: result.TargetName,
		})
		h.app.Logger.Info("environments merged",
			"source", result.SourceID, "target", result.TargetID,
			"rows", result.TotalRows, "user", user.Username)
	}

	response.OK(w, result)
}

// HandleDuplicates lists environments that reach the same machine.
func (h *Handler) HandleDuplicates(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	groups, err := h.service.Duplicates(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to scan for duplicate environments", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, groups)
}
