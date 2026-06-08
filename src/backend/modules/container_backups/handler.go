// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler handles container backup APIs.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a backup handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB, app.DockerPool, app.Config.DataDir, app.BackupCrypto, app.Logger)}
}

// HandleOptions returns backup choices for a container.
func (h *Handler) HandleOptions(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	options, err := h.service.Options(r.Context(), envID, id)
	if err != nil {
		if client.IsErrNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container backup options failed", "env", envID, "container", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerInspectFailed)
		return
	}
	response.OK(w, options)
}

// HandleRunAdhoc runs a manual backup for one container.
func (h *Handler) HandleRunAdhoc(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	var input RunBackupInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	input.SelectedMounts = cleanMountSelection(input.SelectedMounts)
	run, err := h.service.RunAdhoc(r.Context(), envID, id, input)
	if err != nil {
		if errors.Is(err, ErrBackupEncryptionKeyNotConfigured) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupKeyMissing)
			return
		}
		h.app.Logger.Error("manual container backup failed", "env", envID, "container", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup", EntityType: "container", EntityID: id, Details: "manual", EnvironmentID: envID})
	response.OK(w, run)
}

// HandleListPlans lists backup plans for an environment and optional container.
func (h *Handler) HandleListPlans(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	plans, err := h.service.ListPlans(r.Context(), envID, r.URL.Query().Get("containerId"))
	if err != nil {
		h.app.Logger.Error("list container backup plans failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	response.OK(w, plans)
}

// HandleCreatePlan creates a backup plan.
func (h *Handler) HandleCreatePlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	var input CreateBackupPlanInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ContainerID) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if strings.TrimSpace(input.Cron) != "" && !validCronSpec(input.Cron) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := validatePlanRetention(input.RetentionCount, input.RetentionDays); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	input.SelectedMounts = cleanMountSelection(input.SelectedMounts)
	plan, err := h.service.CreatePlan(r.Context(), envID, input)
	if err != nil {
		h.app.Logger.Error("create container backup plan failed", "env", envID, "container", input.ContainerID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup_plan.created", EntityType: "container", EntityID: input.ContainerID, EntityName: plan.Name, EnvironmentID: envID})
	response.Created(w, plan)
}

// HandleUpdatePlan updates a backup plan.
func (h *Handler) HandleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "planId")
	var input UpdateBackupPlanInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.Cron != nil && strings.TrimSpace(*input.Cron) != "" && !validCronSpec(*input.Cron) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.RetentionCount != nil || input.RetentionDays != nil {
		count := 0
		days := 0
		if input.RetentionCount != nil {
			count = *input.RetentionCount
		}
		if input.RetentionDays != nil {
			days = *input.RetentionDays
		}
		if err := validatePlanRetention(count, days); err != nil {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}
	if input.SelectedMounts != nil {
		cleaned := cleanMountSelection(*input.SelectedMounts)
		input.SelectedMounts = &cleaned
	}
	plan, err := h.service.UpdatePlan(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("update container backup plan failed", "plan", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	if plan == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup_plan.updated", EntityType: "container", EntityID: plan.ContainerID, EntityName: plan.Name, EnvironmentID: plan.EnvironmentID})
	response.OK(w, plan)
}

// HandleDeletePlan deletes a backup plan.
func (h *Handler) HandleDeletePlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "planId")
	deleted, err := h.service.DeletePlan(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("delete container backup plan failed", "plan", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup_plan.deleted", EntityType: "container", EntityID: id})
	response.NoContent(w)
}

// HandleRunPlan runs a saved backup plan.
func (h *Handler) HandleRunPlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "planId")
	run, err := h.service.RunPlan(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrBackupEncryptionKeyNotConfigured) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupKeyMissing)
			return
		}
		h.app.Logger.Error("run container backup plan failed", "plan", id, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup", EntityType: "container", EntityID: run.ContainerID, Details: "plan=" + id, EnvironmentID: run.EnvironmentID})
	response.OK(w, run)
}

// HandleListRuns lists recent backup runs for one container.
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	containerID := r.URL.Query().Get("containerId")
	runs, err := h.service.ListRuns(r.Context(), envID, containerID)
	if err != nil {
		h.app.Logger.Error("list container backup runs failed", "env", envID, "container", containerID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	response.OK(w, runs)
}

// HandleDownloadRun streams a completed backup archive.
func (h *Handler) HandleDownloadRun(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	download, err := h.service.Download(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrBackupRunNotDownloadable) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("download container backup failed", "run", runID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if download == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	file, err := os.Open(download.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("open container backup archive failed", "run", runID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		h.app.Logger.Error("stat container backup archive failed", "run", runID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.download",
		EntityType: "container",
		EntityID:   download.RunID,
		Details:    "container backup archive downloaded",
	})

	w.Header().Set("Content-Type", download.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, download.FileName))
	http.ServeContent(w, r, download.FileName, stat.ModTime(), file)
}

// HandleRestoreRun restores a completed backup archive to its original container.
func (h *Handler) HandleRestoreRun(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	var input RestoreBackupInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	result, err := h.service.Restore(r.Context(), runID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable), errors.Is(err, ErrBackupRestoreNoRestorableEntries):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		case client.IsErrNotFound(err):
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		default:
			h.app.Logger.Error("restore container backup failed", "run", runID, "error", err)
			response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.restore",
		EntityType: "container",
		EntityID:   result.RunID,
		Details:    "container backup archive restored",
	})

	response.OK(w, result)
}

func cleanMountSelection(input []string) []string {
	out := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, mountPath := range input {
		mountPath = strings.TrimSpace(mountPath)
		if mountPath == "" || strings.Contains(mountPath, "..") || !strings.HasPrefix(mountPath, "/") || seen[mountPath] {
			continue
		}
		seen[mountPath] = true
		out = append(out, mountPath)
	}
	return out
}
