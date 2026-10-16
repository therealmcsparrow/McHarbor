// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// HandleListBackupPlans serves GET /api/vms/{id}/backup-plans.
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

// HandleCreateBackupPlan serves POST /api/vms/{id}/backup-plans.
func (h *Handler) HandleCreateBackupPlan(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req CreateBackupPlanInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	req.VMID = chi.URLParam(r, "id")

	plan, err := h.svc.CreateBackupPlan(r.Context(), req)
	if err != nil {
		if h.respondBackupRefusal(w, r, err) {
			return
		}
		if code, isValidation := validationCode(err); isValidation {
			h.logger.Warn("kvm: rejected a backup plan", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "create backup plan", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.plan_created", map[string]any{
		"vmId": plan.VMID, "planId": plan.ID, "name": plan.Name,
	})
	created(w, plan, msgBackupPlanSaved)
}

// HandleUpdateBackupPlan serves PUT /api/vms/backup-plans/{planId}.
func (h *Handler) HandleUpdateBackupPlan(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
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
		if h.respondBackupRefusal(w, r, err) {
			return
		}
		if code, isValidation := validationCode(err); isValidation {
			h.logger.Warn("kvm: rejected a backup request", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "update backup plan", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.plan_updated", map[string]any{
		"vmId": plan.VMID, "planId": plan.ID,
	})
	okData(w, plan, msgBackupPlanSaved)
}

// HandleDeleteBackupPlan serves DELETE /api/vms/backup-plans/{planId}.
func (h *Handler) HandleDeleteBackupPlan(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	planID := chi.URLParam(r, "planId")
	if err := h.svc.DeleteBackupPlan(r.Context(), planID); err != nil {
		h.respondErr(w, r, "delete backup plan", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.plan_deleted", map[string]any{"planId": planID})
	okMsg(w, msgBackupPlanGone)
}

// HandleRunBackup serves POST /api/vms/{id}/backups.
//
// Answers as soon as the run is recorded rather than when it finishes: a full
// copy of a large guest takes hours, and an HTTP request is the wrong thing to
// hold open for it. The UI polls the run.
func (h *Handler) HandleRunBackup(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req RunBackupInput
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	vmID := chi.URLParam(r, "id")
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))
	run, err := h.svc.StartBackupRun(r.Context(), vmID, planID, req)
	if err != nil {
		if h.respondBackupRefusal(w, r, err) {
			return
		}
		if code, isValidation := validationCode(err); isValidation {
			h.logger.Warn("kvm: rejected a backup request", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "run backup", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.started", map[string]any{
		"vmId": vmID, "runId": run.ID, "kind": run.Kind,
	})
	okData(w, run, msgBackupStarted)
}

// HandleListBackupRuns serves GET /api/vms/{id}/backups.
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

// HandleBackupRun serves GET /api/vms/backups/{runId}.
func (h *Handler) HandleBackupRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.BackupRunByID(r.Context(), chi.URLParam(r, "runId"))
	if err != nil {
		h.respondErr(w, r, "read backup", err)
		return
	}
	ok(w, run)
}

// HandleDeleteBackupRun serves DELETE /api/vms/backups/{runId}.
func (h *Handler) HandleDeleteBackupRun(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	runID := chi.URLParam(r, "runId")
	if err := h.svc.DeleteBackupRun(r.Context(), runID); err != nil {
		h.respondErr(w, r, "delete backup", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.deleted", map[string]any{"runId": runID})
	okMsg(w, msgDeleted)
}

// HandleRestoreBackup serves POST /api/vms/backups/{runId}/restore.
func (h *Handler) HandleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
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
		if h.respondBackupRefusal(w, r, err) {
			return
		}
		if code, isValidation := validationCode(err); isValidation {
			h.logger.Warn("kvm: rejected a backup request", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "restore backup", err)
		return
	}
	h.emit(userID, "plugin.kvm.backup.restored", map[string]any{
		"vmId": run.VMID, "runId": run.ID, "sourceRunId": runID,
	})
	okData(w, run, msgRestoreStarted)
}

// HandleBackupDownload serves GET /api/vms/backups/internal/download/{grantId}.
//
// Public because the caller is an agent with no session; it redeems a one-shot
// grant. The object is streamed from its destination and decrypted in flight,
// so McHarbor never writes the artifact to its own disk — the same discipline
// the upload direction follows.
func (h *Handler) HandleBackupDownload(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	if _, granted := h.svc.backupGrants.redeem(grantID, token); !granted {
		h.logger.Warn("kvm: backup download refused", "grant", grantID, "remote", r.RemoteAddr)
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
		h.respondErr(w, r, "open backup", err)
		return
	}
	body, err := streamer.DownloadStream(r.Context(), plugin.StorageDownloadRequest{
		LocationID: download.locationID,
		Key:        download.remotePath,
		Decrypt:    true,
	})
	if err != nil {
		h.logger.Error("kvm: opening a stored backup",
			"grant", grantID, "error", err)
		h.respondErr(w, r, "open backup", err)
		return
	}
	defer func() { _ = body.Close() }()
	h.extendTransferDeadlines(w)

	w.Header().Set("Content-Type", "application/octet-stream")
	// No Content-Length: the plaintext size is not known until the envelope
	// has been unwrapped, and a wrong one is worse than none — the agent
	// would stop reading early and rebuild a truncated disk.
	if _, err := io.Copy(w, body); err != nil {
		// The status is already sent, so this can only be logged. The agent
		// sees a short read and fails its own side, which is the right place
		// for it to surface.
		h.logger.Error("kvm: streaming a stored backup",
			"grant", grantID, "error", err)
	}
}
