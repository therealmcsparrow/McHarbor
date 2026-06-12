// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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
	return &Handler{app: app, service: NewService(app.DB, app.DockerPool, app.Config.DataDir, app.BackupCrypto, app.Encryption, app.Logger)}
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
	run, err := h.service.StartAdhoc(r.Context(), envID, id, input)
	if err != nil {
		if errors.Is(err, ErrBackupEncryptionKeyNotConfigured) {
			h.app.Logger.Warn("manual container backup rejected: encryption key missing", "env", envID, "container", id)
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
	run, err := h.service.StartPlan(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrBackupEncryptionKeyNotConfigured) {
			h.app.Logger.Warn("scheduled container backup rejected: encryption key missing", "plan", id)
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

// HandleDeleteRun deletes a completed or failed backup run and its local archive.
func (h *Handler) HandleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	deleted, err := h.service.DeleteRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrBackupRunActive) {
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
		h.app.Logger.Error("delete container backup run failed", "run", runID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.delete",
		EntityType: "container",
		EntityID:   runID,
		Details:    "container backup run deleted",
	})
	response.NoContent(w)
}

// HandleRestoreOptions returns restorable entries for a completed backup archive.
func (h *Handler) HandleRestoreOptions(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	var input RestoreBackupOptionsInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	options, err := h.service.RestoreOptions(r.Context(), runID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		default:
			h.app.Logger.Error("read container backup restore options failed", "run", runID, "error", err)
			response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
	}
	if options == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, options)
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

	run, err := h.service.StartRestore(r.Context(), runID, input)
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
	if run == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "backup.restore",
		EntityType:    "container",
		EntityID:      run.ContainerID,
		Details:       "restore_run=" + run.ID + " source_run=" + runID,
		EnvironmentID: run.EnvironmentID,
	})

	response.OK(w, run)
}

// HandleRestoreUpload restores an uploaded backup archive to the selected container.
func (h *Handler) HandleRestoreUpload(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	envID := response.ParseEnvID(r)
	containerID := chi.URLParam(r, "id")
	if strings.TrimSpace(envID) == "" || strings.TrimSpace(containerID) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	defer file.Close()

	result, err := h.service.RestoreUploaded(r.Context(), RestoreUploadedBackupInput{
		EnvironmentID: envID,
		ContainerID:   containerID,
		SecretKey:     r.FormValue("secretKey"),
		Reader:        file,
	})
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
			h.app.Logger.Error("restore uploaded container backup failed", "env", envID, "container", containerID, "error", err)
			response.InternalErrorCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "backup.restore_upload",
		EntityType:    "container",
		EntityID:      containerID,
		Details:       "uploaded container backup archive restored",
		EnvironmentID: envID,
	})

	response.OK(w, result)
}

// HandleRestoreTransfer streams one prepared restore archive entry to a connected agent.
func (h *Handler) HandleRestoreTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")
	entry, status, ok := restoreTransfers.consume(transferID, r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, http.StatusText(status), status)
		return
	}

	if err := h.service.writeRestoreTransferEntry(r.Context(), w, entry); err != nil {
		if errors.Is(err, ErrBackupRunNotDownloadable) {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		h.app.Logger.Error("restore transfer stream failed", "transfer", transferID, "run", entry.RunID, "entry", entry.EntryName, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// HandleAgentArchiveUpload receives a complete encrypted archive from an agent.
func (h *Handler) HandleAgentArchiveUpload(w http.ResponseWriter, r *http.Request) {
	extendAgentArchiveTransferDeadline(w, true, false)
	transferID := chi.URLParam(r, "transferId")
	entry, status, ok := agentArchiveTransfers.consume(transferID, r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if strings.TrimSpace(entry.TargetPath) == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := h.service.receiveAgentArchive(r.Context(), entry, r.Body); err != nil {
		h.app.Logger.Error("agent backup archive upload failed", "transfer", transferID, "run", entry.RunID, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	response.NoContent(w)
}

// HandleAgentArchiveDownload streams a complete encrypted archive to an agent.
func (h *Handler) HandleAgentArchiveDownload(w http.ResponseWriter, r *http.Request) {
	extendAgentArchiveTransferDeadline(w, false, true)
	transferID := chi.URLParam(r, "transferId")
	entry, status, ok := agentArchiveTransfers.consume(transferID, r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if strings.TrimSpace(entry.SourcePath) == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.service.streamAgentArchive(r.Context(), w, entry.SourcePath); err != nil {
		h.app.Logger.Error("agent backup archive download failed", "transfer", transferID, "run", entry.RunID, "error", err)
		return
	}
}

func extendAgentArchiveTransferDeadline(w http.ResponseWriter, read, write bool) {
	deadline := time.Now().Add(backupUploadTimeout)
	controller := http.NewResponseController(w)
	if read {
		_ = controller.SetReadDeadline(deadline)
	}
	if write {
		_ = controller.SetWriteDeadline(deadline)
	}
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
