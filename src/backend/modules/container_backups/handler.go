// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// auditRunContainer names the container a backup run belongs to, empty when the
// run cannot be read: an audit row without a display name is still worth
// writing, and a run id alone means nothing once the run is gone.
func (h *Handler) auditRunContainer(ctx context.Context, runID string) string {
	run, err := h.service.Run(ctx, runID)
	if err != nil || run == nil {
		return ""
	}
	return run.ContainerName
}

// Handler handles container backup APIs.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a backup handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB, app.DockerPool, app.Config.DataDir, app.BackupCrypto, app.Encryption, app.Logger)}
}

// SetPluginStorageAdapter injects the loaded plugin runtime so
// the upload dispatcher can route plugin-owned LocationTypes
// (any cloud-drive type contributed by a storage plugin)
// through the plugin's StorageAdapterClient.Upload. Called from
// main.go after bootPluginRuntime.
func (h *Handler) SetPluginStorageAdapter(runtime any) {
	h.service.SetPluginStorageAdapter(runtime)
}

// SetContainerRecreator injects the container recreate bridge a config restore
// applies through. Called from Mount, alongside the scheduler's own service.
func (h *Handler) SetContainerRecreator(r ContainerRecreator) {
	h.service.SetContainerRecreator(r)
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
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		}
		h.app.Logger.Error("container backup options failed", "env", envID, "container", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerInspectFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	input.SelectedMounts = cleanMountSelection(input.SelectedMounts)
	run, err := h.service.StartAdhoc(r.Context(), envID, id, input)
	if err != nil {
		if errors.Is(err, ErrBackupDestinationRequired) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupDestinationRequired)
			return
		}
		if errors.Is(err, ErrBackupEncryptionKeyNotConfigured) {
			h.app.Logger.Warn("manual container backup rejected: encryption key missing", "env", envID, "container", id)
			response.BadRequestCode(w, r, i18n.ErrContainerBackupKeyMissing)
			return
		}
		h.app.Logger.Error("manual container backup failed", "env", envID, "container", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup", EntityType: "container", EntityID: id, EntityName: run.ContainerName, Details: "manual", EnvironmentID: envID})
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
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	input.SelectedMounts = cleanMountSelection(input.SelectedMounts)
	plan, err := h.service.CreatePlan(r.Context(), envID, input)
	if err != nil {
		if errors.Is(err, ErrBackupDestinationRequired) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupDestinationRequired)
			return
		}
		h.app.Logger.Error("create container backup plan failed", "env", envID, "container", input.ContainerID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
	}
	if input.SelectedMounts != nil {
		cleaned := cleanMountSelection(*input.SelectedMounts)
		input.SelectedMounts = &cleaned
	}
	plan, err := h.service.UpdatePlan(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, ErrBackupDestinationRequired) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupDestinationRequired)
			return
		}
		h.app.Logger.Error("update container backup plan failed", "plan", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
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
	// Read before the delete: afterwards there is no row left to name.
	planName := ""
	if plan, planErr := h.service.Plan(r.Context(), id); planErr == nil && plan != nil {
		planName = plan.Name
	}
	deleted, err := h.service.DeletePlan(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("delete container backup plan failed", "plan", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup_plan.deleted", EntityType: "container", EntityID: id, EntityName: planName})
	response.NoContent(w)
}

// HandleRelinkAllContainerLinks runs a one-shot reconciliation
// pass: every plan and run gets its container_id resolved against
// the live orchestrator and persisted if stale. Designed to be
// invoked from the Backups overview page's "Re-link stale" button
// after a bulk container recreate (e.g. docker compose up).
func (h *Handler) HandleRelinkAllContainerLinks(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	// Bound the operation so a stuck orchestrator doesn't pin a
	// request indefinitely. The helper itself uses per-row
	// sub-contexts, but the outer context caps total wall time.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := h.service.RelinkAllStaleContainerLinks(ctx)
	if err != nil {
		h.app.Logger.Error("bulk container link relink failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.relink_all",
		EntityType: "container",
		Details: fmt.Sprintf(
			"plans checked=%d plans refreshed=%d runs checked=%d runs refreshed=%d",
			result.PlansChecked, result.PlansRefreshed,
			result.RunsChecked, result.RunsRefreshed,
		),
	})
	response.OK(w, result)
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
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup", EntityType: "container", EntityID: run.ContainerID, EntityName: run.ContainerName, Details: "plan=" + id, EnvironmentID: run.EnvironmentID})
	response.OK(w, run)
}

// HandleListRuns lists recent backup runs for an environment.
// `containerId` is optional: omit it to fetch every container's
// runs (used by the Backups overview page); supply it to scope to
// one container (used by the per-container Backups tab).
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
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}

	response.OK(w, runs)
}

// HandleListProviderRuns serves the backups every other subsystem took.
//
// A route of its own rather than a wider envelope on the runs list: half a
// dozen callers read that list for container work alone, and changing its
// shape to serve one page would have broken all of them to save one request.
func (h *Handler) HandleListProviderRuns(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	providerRuns, sources := h.service.ProviderRuns(r.Context(), envID)
	response.OK(w, BackupRunsPage{
		ProviderRuns: providerRuns,
		Sources:      append(hostBackupSource(), sources...),
	})
}

// HandleListProviderPlans serves the schedules every other subsystem keeps.
func (h *Handler) HandleListProviderPlans(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	providerPlans, sources := h.service.ProviderPlans(r.Context(), envID)
	response.OK(w, BackupPlansPage{
		ProviderPlans: providerPlans,
		Sources:       append(hostBackupSource(), sources...),
	})
}

// HandleRun returns one backup or restore run. The restore dialog polls
// it for live progress, so it stays a plain read: no audit entry, and the
// same view permission the list route carries.
func (h *Handler) HandleRun(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	runID := chi.URLParam(r, "runId")
	run, err := h.service.Run(r.Context(), runID)
	if err != nil {
		h.app.Logger.Error("read container backup run failed", "run", runID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	if run == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, run)
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	defer closeLogged(file, "backup archive")

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		h.app.Logger.Error("stat container backup archive failed", "run", runID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.download",
		EntityType: "container",
		EntityID:   download.RunID,
		EntityName: h.auditRunContainer(r.Context(), download.RunID),
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
	// Read before the delete: afterwards there is no run left to name.
	runContainer := h.auditRunContainer(r.Context(), runID)
	deleted, err := h.service.DeleteRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrBackupRunActive) {
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
		h.app.Logger.Error("delete container backup run failed", "run", runID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
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
		EntityName: runContainer,
		Details:    "container backup run deleted",
	})
	response.NoContent(w)
}

// HandleRetryDestinationUpload re-uploads the on-disk archive of an
// existing backup run to one previously-failed storage destination.
// The request returns immediately with the destination row in
// `uploading` state; the actual upload runs in a background
// goroutine and the operator polls the regular run query to watch
// progress through the destination's bytes_uploaded / bytes_total
// columns.
func (h *Handler) HandleRetryDestinationUpload(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	runID := chi.URLParam(r, "runId")
	destinationID := chi.URLParam(r, "destinationId")
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(destinationID) == "" {
		response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
		return
	}
	destination, err := h.service.RetryDestinationUpload(r.Context(), runID, destinationID)
	if err != nil {
		if errors.Is(err, ErrRetryUploadNotEligible) {
			response.ConflictCode(w, r, i18n.ErrContainerActionFailed)
			return
		}
		h.app.Logger.Error(
			"retry destination upload failed",
			"run", runID, "destination", destinationID, "error", err,
		)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "backup.retry_destination",
		EntityType: "container",
		EntityID:   runID,
		EntityName: h.auditRunContainer(r.Context(), runID),
		Details:    fmt.Sprintf("retry upload for destination %s", destinationID),
	})
	response.OK(w, destination)
}

// HandleCancelRun stops a running backup; a run that already finished is refused with its own code rather than altered.
func (h *Handler) HandleCancelRun(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	run, err := h.service.CancelRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrBackupRunNotCancellable) {
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRunNotCancellable)
			return
		}
		h.app.Logger.Error("cancel container backup run failed", "run", runID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		return
	}
	if run == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "backup.cancel",
		EntityType:    "container",
		EntityID:      run.ContainerID,
		EntityName:    run.ContainerName,
		Details:       "run=" + run.ID + " operation=" + run.Operation,
		EnvironmentID: run.EnvironmentID,
	})

	response.OK(w, run)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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
			response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
			return
		}
	}
	if options == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, options)
}

// HandleRestoreFilesystemEntries returns the first-level children of
// `innerPath` inside `container/filesystem.tar` for the named run.
// Drives the restore dialog's per-file/per-folder tree when the
// operator disables the "Full restore" toggle.
func (h *Handler) HandleRestoreFilesystemEntries(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	var input RestoreFilesystemEntriesInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	items, truncated, limit, err := h.service.FilesystemEntriesForRestore(
		r.Context(), runID, input.InnerPath, input.SecretKey,
	)
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
			h.app.Logger.Error("read container backup filesystem entries failed", "run", runID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerBackupRestoreTreeEntriesFailed, err)
			return
		}
	}
	response.OK(w, RestoreEntriesResult{
		Items:          items,
		Truncated:      truncated,
		MaxChildrenCap: limit,
	})
}

// HandleRestoreMountEntries returns the first-level children of
// `innerPath` inside `mounts/<safeTarget>.tar` for the named run.
// Mount target is required and must match a source-mount path the
// backup archive actually contains.
func (h *Handler) HandleRestoreMountEntries(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	var input RestoreMountEntriesInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(input.MountTarget) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	items, truncated, limit, err := h.service.MountEntriesForRestore(
		r.Context(), runID, input.MountTarget, input.InnerPath, input.SecretKey,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRestoreMountMissing):
			response.NotFoundCode(w, r, i18n.ErrContainerBackupRestoreMountMissing)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		default:
			h.app.Logger.Error("read container backup mount entries failed", "run", runID, "mount", input.MountTarget, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerBackupRestoreTreeEntriesFailed, err)
			return
		}
	}
	response.OK(w, RestoreEntriesResult{
		Items:          items,
		Truncated:      truncated,
		MaxChildrenCap: limit,
	})
}

// HandleRestoreConfigEntries returns the synthetic config tree
// children under `parentKey` for the named run. Drives the
// per-key config picker when the operator disables the
// "Full config" toggle.
func (h *Handler) HandleRestoreConfigEntries(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	runID := chi.URLParam(r, "runId")
	var input RestoreConfigEntriesInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	items, truncated, limit, err := h.service.ConfigEntriesForRestore(
		r.Context(), runID, input.ParentKey, input.SecretKey,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRestoreConfigMissing):
			response.NotFoundCode(w, r, i18n.ErrContainerBackupRestoreConfigMissing)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		default:
			h.app.Logger.Error("read container backup config entries failed", "run", runID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerBackupRestoreTreeEntriesFailed, err)
			return
		}
	}
	response.OK(w, RestoreEntriesResult{
		Items:          items,
		Truncated:      truncated,
		MaxChildrenCap: limit,
	})
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	run, err := h.service.StartRestore(r.Context(), runID, input)
	if err != nil {
		if h.refuseSelfRestore(w, r, audit.Entry{EntityType: "container_backup_run", EntityID: runID, Details: "op=restore"}, err) {
			return
		}
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRestoreConfigFilesystemConflict):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreConfigFsConflict)
			return
		case errors.Is(err, ErrBackupRestoreConfigUnsupported):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreConfigUnsupported)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable), errors.Is(err, ErrBackupRestoreNoRestorableEntries):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		case cerrdefs.IsNotFound(err):
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		default:
			h.app.Logger.Error("restore container backup failed", "run", runID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
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
		EntityName:    run.ContainerName,
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	defer closeLogged(file, "uploaded backup archive")

	result, err := h.service.RestoreUploaded(r.Context(), RestoreUploadedBackupInput{
		EnvironmentID: envID,
		ContainerID:   containerID,
		SecretKey:     r.FormValue("secretKey"),
		Reader:        file,
	})
	if err != nil {
		if h.refuseSelfRestore(w, r, audit.Entry{EntityType: "container", EntityID: containerID, EnvironmentID: envID, Details: "op=restore_upload"}, err) {
			return
		}
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
			return
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
			return
		case errors.Is(err, ErrBackupRestoreConfigFilesystemConflict):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreConfigFsConflict)
			return
		case errors.Is(err, ErrBackupRestoreConfigUnsupported):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreConfigUnsupported)
			return
		case errors.Is(err, ErrBackupRunNotDownloadable), errors.Is(err, ErrBackupRestoreNoRestorableEntries):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
			return
		case cerrdefs.IsNotFound(err):
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return
		default:
			h.app.Logger.Error("restore uploaded container backup failed", "env", envID, "container", containerID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
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

// HandleStageUpload stores an uploaded backup archive as a run without restoring it, so
// the restore dialog can read its contents and the operator can restore a selection.
func (h *Handler) HandleStageUpload(w http.ResponseWriter, r *http.Request) {
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	defer closeLogged(file, "uploaded backup archive")

	run, err := h.service.StageUploaded(r.Context(), RestoreUploadedBackupInput{
		EnvironmentID: envID,
		ContainerID:   containerID,
		SecretKey:     r.FormValue("secretKey"),
		Reader:        file,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrBackupRestoreSecretRequired):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyRequired)
		case errors.Is(err, ErrBackupRestoreKeyInvalid):
			response.BadRequestCode(w, r, i18n.ErrContainerBackupRestoreKeyInvalid)
		case errors.Is(err, ErrBackupRestoreNoRestorableEntries):
			response.BadRequestCode(w, r, i18n.ErrContainerActionFailed)
		default:
			h.app.Logger.Error("stage uploaded container backup failed", "env", envID, "container", containerID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrContainerActionFailed, err)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "backup.upload",
		EntityType:    "container",
		EntityID:      containerID,
		Details:       "uploaded container backup archive staged as run " + run.ID,
		EnvironmentID: envID,
	})

	response.OK(w, run)
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
		if err := controller.SetReadDeadline(deadline); err != nil {
			slog.Debug("container backups: extending read deadline unsupported", "error", err)
		}
	}
	if write {
		if err := controller.SetWriteDeadline(deadline); err != nil {
			slog.Debug("container backups: extending write deadline unsupported", "error", err)
		}
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

// The resolvers rbac.PinEnvToResource uses on the run, plan and snapshot-config
// routes, so each is authorized on the environment its row lives in.
func (h *Handler) runEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.RunEnvironment(r.Context(), chi.URLParam(r, "runId")))
}

func (h *Handler) planEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.PlanEnvironment(r.Context(), chi.URLParam(r, "planId")))
}

func (h *Handler) snapshotConfigEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.SnapshotConfigEnvironment(r.Context(), chi.URLParam(r, "id")))
}

func (h *Handler) logResolve(envID string, found bool, err error) (string, bool, error) {
	if err != nil {
		h.app.Logger.Error("container backups: resolving the row's environment", "error", err)
	}
	return envID, found, err
}
