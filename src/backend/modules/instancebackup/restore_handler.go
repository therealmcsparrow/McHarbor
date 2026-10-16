// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// ArtifactProblem is a refusal caused by the file or key the operator gave. It
// carries a reason key only: the underlying message is logged, never sent.
type ArtifactProblem struct {
	Reason string `json:"reason"`
}

// artifactReason picks the restore.reasons key for an artifact refusal, most
// specific first, so a wrong key is never reported as a damaged file.
func artifactReason(err error) string {
	switch {
	case errors.Is(err, core.ErrWrongKey):
		return "wrong_key"
	case errors.Is(err, core.ErrKeyKind):
		return "key_kind"
	case errors.Is(err, core.ErrBadKeyFile):
		return "bad_key_file"
	case errors.Is(err, core.ErrNotExport):
		return "not_export"
	case errors.Is(err, core.ErrUnsupportedFormat):
		return "format"
	case errors.Is(err, core.ErrIncomplete):
		return "incomplete"
	case errors.Is(err, core.ErrDamaged):
		return "damaged"
	}
	return "unreadable"
}

// HandleUpload serves PUT /api/instance-backups/uploads. The body is the raw
// artifact; an export can be large, so the read deadline is extended here.
func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(4 * time.Hour)); err != nil {
		h.app.Logger.Warn("instance backup: extending the upload deadline", "error", err)
	}
	up, err := h.svc.SaveUpload(r.Context(), r.Body)
	if h.writeRestoreError(w, r, err) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "upload", EntityType: "instance_backup", EntityID: up.UploadID,
		Details: fmt.Sprintf("export from %s taken %s", up.Manifest.Hostname, up.Manifest.CreatedAt.Format(time.RFC3339))})
	response.Created(w, up)
}

func validRestoreInput(in RestoreInput) string {
	switch in.Mode {
	case core.ModeFull, core.ModeMigration:
	case core.ModeSelective:
		if len(in.Categories) == 0 && !in.IncludeAudit {
			return "no_categories"
		}
		for _, c := range in.Categories {
			if !slices.Contains(core.Categories, c) || c == core.CategoryAudit {
				return "bad_category"
			}
		}
		if in.Conflict != "" && in.Conflict != core.ConflictReplace && in.Conflict != core.ConflictKeep {
			return "bad_conflict"
		}
	default:
		return "bad_mode"
	}
	return (core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}).CheckOpen()
}

// HandlePlan serves POST /api/instance-backups/restore/plan: the whole file
// is read and checked, and nothing is written.
func (h *Handler) HandlePlan(w http.ResponseWriter, r *http.Request) {
	var in RestoreInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if reason := validRestoreInput(in); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: reason})
		return
	}
	plan, err := h.svc.Plan(r.Context(), in)
	if h.writeRestoreError(w, r, err) {
		return
	}
	response.OK(w, plan)
}

// HandleRestore serves POST /api/instance-backups/restore. Restoring audit
// history needs instance_backup.restore_audit through an approved elevation:
// a standing grant is not enough, so a second person agrees at the time.
func (h *Handler) HandleRestore(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in RestoreInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if reason := validRestoreInput(in); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: reason})
		return
	}
	if in.IncludeAudit {
		ok, err := h.svc.HasActiveElevation(r.Context(), user.ID, string(rbac.PermInstanceBackupRestoreAudit))
		if err != nil {
			h.app.Logger.Error("instance restore: checking the audit approval", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !ok {
			h.app.AuditLog.Log(r, audit.Entry{Action: "restore.denied", EntityType: "instance_backup", Details: "audit history needs an approved elevation"})
			response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrRBACPermissionDenied, InvalidReason{Reason: "approval_required"})
			return
		}
	}
	st, err := h.svc.StartRestore(r.Context(), user.Username, in)
	if errors.Is(err, ErrRestoreRunning) {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, InvalidReason{Reason: "busy"})
		return
	}
	if h.writeRestoreError(w, r, err) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "restore.start", EntityType: "instance_backup", EntityID: st.ID,
		Details: fmt.Sprintf("mode=%s categories=%v audit=%t", in.Mode, in.Categories, in.IncludeAudit)})
	response.Created(w, st)
}

// HandleRestoreStatus serves GET /api/instance-backups/restore/status.
func (h *Handler) HandleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.RestoreStatus()
	if err != nil {
		h.app.Logger.Error("instance restore: reading the status", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, st)
}

// HandleLedger serves GET /api/instance-backups/restore/ledger, the restore
// history that survives the audit log being replaced.
func (h *Handler) HandleLedger(w http.ResponseWriter, r *http.Request) {
	entries, err := core.ReadLedger(h.svc.dataDir)
	if err != nil {
		h.app.Logger.Error("instance restore: reading the ledger", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if entries == nil {
		entries = []core.LedgerEntry{}
	}
	response.OK(w, entries)
}

func (h *Handler) writeRestoreError(w http.ResponseWriter, r *http.Request, err error) bool {
	var artifact *core.ArtifactError
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
	case errors.Is(err, core.ErrWrongKey):
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: "wrong_key"})
	case errors.As(err, &artifact):
		reason := artifactReason(artifact)
		h.app.Logger.Warn("instance restore: artifact refused", "reason", reason, "error", artifact)
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, ArtifactProblem{Reason: reason})
	default:
		h.app.Logger.Error("instance restore", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
	return true
}
