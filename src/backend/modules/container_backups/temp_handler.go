// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// tempBodyLimit bounds the PUT body; a path is a few hundred bytes at most.
const tempBodyLimit = 8 << 10

// HandleGetTemp reports the backup temp root, its source and what McHarbor-named data it holds.
func (h *Handler) HandleGetTemp(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	st, err := h.service.TempStatus(r.Context())
	if err != nil {
		h.app.Logger.Error("backup temp status failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrBackupTempStatusFailed, err)
		return
	}
	response.OK(w, st)
}

// HandleSaveTemp validates, probes and saves the temp location, then answers with the new status.
func (h *Handler) HandleSaveTemp(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, tempBodyLimit)
	var input BackupTempLocationInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	oldRoot, newRoot, err := h.service.SaveTempRoot(r.Context(), input.Path)
	if err != nil {
		h.respondTempSaveError(w, r, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "backup_temp.location_changed", EntityType: "backup_temp", EntityID: "location", EntityName: newRoot, StateBefore: oldRoot, StateAfter: newRoot})
	st, err := h.service.TempStatus(r.Context())
	if err != nil {
		h.app.Logger.Error("backup temp status after save failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrBackupTempStatusFailed, err)
		return
	}
	response.OK(w, st)
}

// HandleCleanupTemp removes the McHarbor-named leftovers no run owns from the current and retired temp folders and
// audits the counts at warning severity, because the removal is irreversible.
func (h *Handler) HandleCleanupTemp(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	res, err := h.service.CleanupTemp(r.Context())
	if err != nil {
		h.app.Logger.Error("backup temp cleanup failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrBackupTempCleanupFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "backup_temp.cleaned", Severity: "warning", EntityType: "backup_temp", EntityID: "location", EntityName: "cleanup",
		Details: fmt.Sprintf("removedEntries=%d removedBytes=%d keptOwned=%d keptRecent=%d activeRuns=%d", res.RemovedEntries, res.RemovedBytes, res.KeptOwned, res.KeptRecent, res.ActiveRuns),
	})
	response.OK(w, res)
}

func (h *Handler) respondTempSaveError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errTempEnvOverride) {
		response.ConflictCode(w, r, i18n.ErrBackupTempEnvOverride)
		return
	}
	code := tempErrCode(err)
	switch {
	case errors.Is(err, errTempNotWritable):
		response.BadRequestCause(w, r, code, err)
	case code != "":
		response.BadRequestCode(w, r, code)
	default:
		h.app.Logger.Error("saving backup temp folder failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrBackupTempSaveFailed, err)
	}
}
