// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// HandleListBackupPlans serves GET /api/system-containers/{id}/backup-plans.
func (h *Handler) HandleListBackupPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListBackupPlans(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list backup plans", err)
		return
	}
	if plans == nil {
		plans = []BackupPlan{}
	}
	ok(w, plans)
}

// HandleCreateBackupPlan serves POST /api/system-containers/{id}/backup-plans.
func (h *Handler) HandleCreateBackupPlan(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req CreateBackupPlanInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	req.ContainerID = chi.URLParam(r, "id")

	plan, err := h.svc.CreateBackupPlan(r.Context(), req)
	if err != nil {
		h.respondBackupErr(w, r, "create backup plan", err)
		return
	}
	h.emit("plugin.lxc.backup.plan_created", map[string]any{
		"containerId": plan.ContainerID, "planId": plan.ID, "name": plan.Name,
	})
	created(w, plan, msgBackupPlanSaved)
}

// HandleUpdateBackupPlan serves PUT /api/system-containers/backup-plans/{planId}.
func (h *Handler) HandleUpdateBackupPlan(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req UpdateBackupPlanInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	plan, err := h.svc.UpdateBackupPlan(r.Context(), chi.URLParam(r, "planId"), req)
	if err != nil {
		h.respondBackupErr(w, r, "update backup plan", err)
		return
	}
	h.emit("plugin.lxc.backup.plan_updated", map[string]any{
		"containerId": plan.ContainerID, "planId": plan.ID,
	})
	okData(w, plan, msgBackupPlanSaved)
}

// HandleDeleteBackupPlan serves DELETE /api/system-containers/backup-plans/{planId}.
func (h *Handler) HandleDeleteBackupPlan(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	planID := chi.URLParam(r, "planId")
	if err := h.svc.DeleteBackupPlan(r.Context(), planID); err != nil {
		h.respondBackupErr(w, r, "delete backup plan", err)
		return
	}
	h.emit("plugin.lxc.backup.plan_deleted", map[string]any{"planId": planID})
	okMsg(w, msgBackupPlanGone)
}

// HandleRunBackup serves POST /api/system-containers/{id}/backups.
//
// Answers as soon as the run is recorded rather than when it finishes: a full
// copy of a large rootfs takes hours, and an HTTP request is the wrong thing to
// hold open for it. The UI polls the run.
func (h *Handler) HandleRunBackup(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req RunBackupInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	containerID := chi.URLParam(r, "id")
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))
	run, err := h.svc.StartBackupRun(r.Context(), containerID, planID, req)
	if err != nil {
		h.respondBackupErr(w, r, "run backup", err)
		return
	}
	h.emit("plugin.lxc.backup.started", map[string]any{
		"containerId": containerID, "runId": run.ID, "kind": run.Kind,
	})
	okData(w, run, msgBackupStarted)
}

// HandleListBackupRuns serves GET /api/system-containers/{id}/backups.
func (h *Handler) HandleListBackupRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := h.svc.ListBackupRuns(r.Context(), chi.URLParam(r, "id"), limit)
	if err != nil {
		h.respondErr(w, r, "list backups", err)
		return
	}
	if runs == nil {
		runs = []BackupRun{}
	}
	ok(w, runs)
}

// HandleBackupRun serves GET /api/system-containers/backups/{runId}.
func (h *Handler) HandleBackupRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.BackupRunByID(r.Context(), chi.URLParam(r, "runId"))
	if err != nil {
		h.respondErr(w, r, "read backup", err)
		return
	}
	ok(w, run)
}

// HandleDeleteBackupRun serves DELETE /api/system-containers/backups/{runId}.
func (h *Handler) HandleDeleteBackupRun(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	runID := chi.URLParam(r, "runId")
	if err := h.svc.DeleteBackupRun(r.Context(), runID); err != nil {
		h.respondBackupErr(w, r, "delete backup", err)
		return
	}
	h.emit("plugin.lxc.backup.deleted", map[string]any{"runId": runID})
	okMsg(w, msgBackupDeleted)
}

// HandleRestoreBackup serves POST /api/system-containers/backups/{runId}/restore.
func (h *Handler) HandleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req RestoreBackupInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	runID := chi.URLParam(r, "runId")
	run, err := h.svc.StartRestore(r.Context(), runID, req)
	if err != nil {
		h.respondBackupErr(w, r, "restore backup", err)
		return
	}
	h.emit("plugin.lxc.backup.restored", map[string]any{
		"containerId": run.ContainerID, "runId": run.ID,
		"sourceRunId": runID, "targetContainerId": run.TargetContainerID,
	})
	okData(w, run, msgRestoreStarted)
}

// HandleOrphanedBackups serves GET /api/system-containers/orphaned-backups.
func (h *Handler) HandleOrphanedBackups(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = 0
	}
	runs, err := h.svc.OrphanedBackups(r.Context(), strings.TrimSpace(r.URL.Query().Get("env")), limit)
	if err != nil {
		h.respondErr(w, r, "list orphaned backups", err)
		return
	}
	ok(w, runs)
}

// HandleBackupManifest serves GET /api/system-containers/backups/{runId}/manifest.
//
// Read-only: it answers with a draft and builds nothing, which is why it takes
// ct.backup rather than the keys the recreate itself needs.
func (h *Handler) HandleBackupManifest(w http.ResponseWriter, r *http.Request) {
	draft, err := h.svc.BackupManifest(r.Context(), chi.URLParam(r, "runId"))
	if err != nil {
		h.respondBackupErr(w, r, "read backup manifest", err)
		return
	}
	ok(w, draft)
}

// HandleRecreateFromBackup serves POST /api/system-containers/backups/{runId}/recreate.
//
// Answers once the container is defined and the restore is running; the UI
// follows the restore run the same way it follows any other.
func (h *Handler) HandleRecreateFromBackup(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req RecreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	runID := chi.URLParam(r, "runId")
	create, origin, err := h.svc.ResolveRecreate(r.Context(), runID, req)
	if err != nil {
		h.respondBackupErr(w, r, "recreate from backup", err)
		return
	}
	if err := validateRecreate(create); err != nil {
		// The operator's own input, so the reason is theirs to read.
		h.logger.Warn("system containers: rejected a recreate from backup", "reason", err)
		failDetailErr(w, r, http.StatusBadRequest, requestCode(err), hostErrorDetail(err), err)
		return
	}
	if !h.allowRecreate(w, r, userID, create) {
		return
	}

	result, err := h.svc.Recreate(r.Context(), runID, create, origin)
	if err != nil {
		h.respondBackupErr(w, r, "recreate from backup", err)
		return
	}
	h.emit("plugin.lxc.backup.recreated", map[string]any{
		"environmentId": result.Container.EnvironmentID,
		"entityType":    "container",
		"entityId":      result.Container.ID,
		"name":          result.Container.Name,
		"sourceRunId":   runID,
		"restoreRunId":  result.Run.ID,
		"privileged":    result.Container.Privileged,
		"userId":        userID,
	})
	created(w, result, msgRecreateStarted)
}

// allowRecreate requires both halves of a recreate on the host it lands on, and
// writes the refusal itself.
//
// The route middleware checks ct.restore; ct.manage is checked here because a
// recreate creates a container, and both are checked against the target host
// because the one in the request is the one that gets the container. The
// definition's own escalations then follow exactly the create route's rules.
func (h *Handler) allowRecreate(w http.ResponseWriter, r *http.Request, userID string, create CreateRequest) bool {
	if h.rbac == nil {
		h.logger.Error("system containers: no rbac handle to check a recreate")
		fail(w, r, http.StatusForbidden, codeRecreateNeedsRestore)
		return false
	}
	for _, need := range []struct{ perm, code string }{
		{"ct.restore", codeRecreateNeedsRestore},
		{"ct.manage", codeRecreateNeedsManage},
	} {
		allowed, err := h.rbac.HasPermission(userID, create.EnvironmentID, need.perm)
		if err != nil {
			h.logger.Error("system containers: checking a recreate permission",
				"permission", need.perm, "error", err)
		}
		if err != nil || !allowed {
			failErr(w, r, http.StatusForbidden, need.code, err)
			return false
		}
	}
	if create.Privileged || create.Nesting {
		if !h.allowPrivileged(w, r, userID, create.EnvironmentID) {
			return false
		}
	}
	for _, mount := range create.Mounts {
		if requiresPrivilege(mount.Kind) {
			return h.allowPrivileged(w, r, userID, create.EnvironmentID)
		}
	}
	return true
}

// HandleBackupUpload serves PUT /api/system-containers/backups/internal/{grantId}.
//
// Public because the caller is an agent and holds no session; it redeems a
// one-shot grant McHarbor minted moments earlier. The request body is piped
// straight to the destinations — nothing is written to McHarbor's disk, which is
// what makes backing up a large rootfs possible at all.
func (h *Handler) HandleBackupUpload(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	grant, granted := h.svc.backupGrants.redeem(grantID, token)
	if !granted {
		h.logger.Warn("system containers: backup upload refused",
			"grant", grantID, "remote", r.RemoteAddr)
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	target, live := h.svc.backupUploadFor(grant.runID)
	if !live {
		fail(w, r, http.StatusConflict, codeNotFound)
		return
	}

	part := backupPartFromHeader(r.Header.Get("X-McHarbor-Part"))
	kind := strings.TrimSpace(r.Header.Get("X-McHarbor-Backup-Kind"))
	defer func() { _ = r.Body.Close() }()
	h.extendTransferDeadlines(w)

	if err := h.svc.fanOutBackup(r.Context(), target, part, kind, r.Body, r.ContentLength); err != nil {
		h.logger.Error("system containers: storing a backup",
			"run", grant.runID, "part", part, "error", err)
		if _, rejected := validationCode(err); rejected {
			h.respondBackupErr(w, r, "store backup", err)
			return
		}
		// The destination's own reason goes back to the agent, so the run it
		// records says why — not only that McHarbor answered 502.
		failDetailErr(w, r, http.StatusBadGateway, codeStoreFailed, hostErrorDetail(err), err)
		return
	}
	okMsg(w, msgBackupStored)
}

// HandleBackupDownload serves GET /api/system-containers/backups/internal/download/{grantId}.
//
// Public because the caller is an agent with no session; it redeems a one-shot
// grant. The object is streamed from its destination and decrypted in flight, so
// McHarbor never writes the artifact to its own disk — the same discipline the
// upload direction follows.
func (h *Handler) HandleBackupDownload(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	if _, granted := h.svc.backupGrants.redeem(grantID, token); !granted {
		h.logger.Warn("system containers: backup download refused",
			"grant", grantID, "remote", r.RemoteAddr)
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	download, found := h.svc.downloadFor(grantID)
	if !found {
		fail(w, r, http.StatusNotFound, codeNotFound)
		return
	}

	streamer, err := h.svc.streamHandle()
	if err != nil {
		h.respondBackupErr(w, r, "open backup", err)
		return
	}
	body, err := streamer.DownloadStream(r.Context(), plugin.StorageDownloadRequest{
		LocationID: download.locationID,
		Key:        download.remotePath,
		Decrypt:    true,
	})
	if err != nil {
		h.logger.Error("system containers: opening a stored backup",
			"grant", grantID, "error", err)
		h.respondBackupErr(w, r, "open backup", err)
		return
	}
	defer func() { _ = body.Close() }()
	h.extendTransferDeadlines(w)

	w.Header().Set("Content-Type", "application/octet-stream")
	// No Content-Length: the plaintext size is not known until the envelope has
	// been unwrapped, and a wrong one is worse than none — the agent would stop
	// reading early and rebuild a truncated filesystem.
	if _, err := io.Copy(w, body); err != nil {
		// The status is already sent, so this can only be logged. The agent sees
		// a short read and fails its own side, which is the right place for it
		// to surface.
		h.logger.Error("system containers: streaming a stored backup",
			"grant", grantID, "error", err)
	}
}

// extendTransferDeadlines lifts the server's read and write limits for one
// artifact transfer, which lasts as long as the rootfs takes to cross.
//
// Bounded by the grant's lifetime, the same budget the agent works to. Without
// it every transfer longer than the server's read timeout failed mid-stream as a
// 502.
func (h *Handler) extendTransferDeadlines(w http.ResponseWriter) {
	deadline := time.Now().Add(backupGrantTTL)
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(deadline); err != nil {
		h.logger.Warn("system containers: cannot extend the backup transfer read deadline",
			"error", err)
	}
	if err := controller.SetWriteDeadline(deadline); err != nil {
		h.logger.Warn("system containers: cannot extend the backup transfer write deadline",
			"error", err)
	}
}

// backupPartFromHeader reads which half of an artifact an upload carries.
//
// Defaults to the rootfs so a header McHarbor did not understand still stores
// something addressable rather than failing a transfer that has already crossed
// the network.
func backupPartFromHeader(value string) string {
	if strings.TrimSpace(value) == BackupPartManifest {
		return BackupPartManifest
	}
	return BackupPartRootfs
}
