// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleListSnapshotConfigs returns the snapshot configurations for the
// selected environment.
func (h *Handler) HandleListSnapshotConfigs(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	if strings.TrimSpace(envID) == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	configs, err := h.service.ListSnapshotConfigs(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("listing snapshot configs failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, configs)
}

// HandleCreateSnapshotConfig creates a snapshot configuration.
func (h *Handler) HandleCreateSnapshotConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	if strings.TrimSpace(envID) == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	var input CreateSnapshotConfigInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	input.EnvironmentID = envID
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Driver) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	config, err := h.service.CreateSnapshotConfig(r.Context(), input)
	if errors.Is(err, ErrSnapshotDriverUnavailable) {
		h.app.Logger.Warn("creating snapshot config refused: driver not loaded", "env", envID, "driver", input.Driver)
		response.BadRequestCode(w, r, i18n.ErrPluginNotInstalled)
		return
	}
	if err != nil {
		h.app.Logger.Error("creating snapshot config failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.auditSnapshotConfig(r, "snapshot_config.created", config.ID, config.Name, envID)
	response.Created(w, config)
}

// HandleUpdateSnapshotConfig updates a snapshot configuration.
func (h *Handler) HandleUpdateSnapshotConfig(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	var input UpdateSnapshotConfigInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	config, err := h.service.UpdateSnapshotConfig(r.Context(), id, input)
	if errors.Is(err, ErrSnapshotDriverUnavailable) {
		h.app.Logger.Warn("updating snapshot config refused: driver not loaded", "config", id)
		response.BadRequestCode(w, r, i18n.ErrPluginNotInstalled)
		return
	}
	if err != nil {
		h.app.Logger.Error("updating snapshot config failed", "config", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if config == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	h.auditSnapshotConfig(r, "snapshot_config.updated", config.ID, config.Name, config.EnvironmentID)
	response.OK(w, config)
}

// HandleDeleteSnapshotConfig removes a snapshot configuration. Plans
// pointing at it keep working: the foreign key nulls the reference and
// those plans fall back to pausing.
func (h *Handler) HandleDeleteSnapshotConfig(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteSnapshotConfig(r.Context(), id); err != nil {
		h.app.Logger.Error("deleting snapshot config failed", "config", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.auditSnapshotConfig(r, "snapshot_config.deleted", id, "", response.ParseEnvID(r))
	response.OK(w, map[string]bool{"deleted": true})
}

// HandleProbeSnapshotConfig runs the driver's capability check and
// stores the result, which is what the Test button reports.
func (h *Handler) HandleProbeSnapshotConfig(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	result, err := h.service.ProbeSnapshotConfig(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrSnapshotDriverUnavailable) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("probing snapshot config failed", "config", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, result)
}

// HandleRecentSnapshots lists the snapshots recent backup runs took.
func (h *Handler) HandleRecentSnapshots(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	if strings.TrimSpace(envID) == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	rows, err := h.service.RecentSnapshots(r.Context(), envID, limit)
	if err != nil {
		h.app.Logger.Error("listing recent snapshots failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, rows)
}

// HandleDetectSnapshotDrivers reports which snapshot drivers this
// environment can use. For an agent environment the agent probes its
// own host; for a local one McHarbor probes its own container. Either
// way the operator is told what exists rather than having to guess
// before saving a configuration.
func (h *Handler) HandleDetectSnapshotDrivers(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	if strings.TrimSpace(envID) == "" {
		response.BadRequestCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	statuses, err := h.service.SnapshotDriverStatuses(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("detecting snapshot drivers failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, statuses)
}

// auditSnapshotConfig records a mutation of the snapshot configuration
// set. Snapshot configs decide what a backup freezes and whether a
// container is paused, so a change to one is as auditable as a change
// to the plan that uses it.
func (h *Handler) auditSnapshotConfig(r *http.Request, action, configID, name, envID string) {
	if h.app == nil || h.app.AuditLog == nil {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		EntityType:    "snapshot_config",
		EntityID:      configID,
		EntityName:    name,
		EnvironmentID: envID,
	})
}
