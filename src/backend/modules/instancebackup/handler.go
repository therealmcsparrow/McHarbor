// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler serves the instance backup routes.
type Handler struct {
	app      *router.AppDeps
	svc      *Service
	uploader Uploader
}

// NewHandler returns a Handler over svc.
func NewHandler(app *router.AppDeps, svc *Service, uploader Uploader) *Handler {
	return &Handler{app: app, svc: svc, uploader: uploader}
}

// HandleList serves GET /api/instance-backups.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	items, total, err := h.svc.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("instance backup: listing exports", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// HandleGet serves GET /api/instance-backups/{id}.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if h.writeLookupError(w, r, err) {
		return
	}
	response.OK(w, e)
}

// HandleKeyFile serves POST /api/instance-backups/key-file. The key is
// generated here, returned once and never stored.
func (h *Handler) HandleKeyFile(w http.ResponseWriter, r *http.Request) {
	kf, err := core.GenerateKeyFile()
	if err != nil {
		h.app.Logger.Error("instance backup: generating a key file", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.NoStore(w)
	response.OK(w, KeyFileResult{KeyFile: kf, FileName: "mcharbor-instance-" + time.Now().UTC().Format("20060102") + ".key"})
}

// HandleCreate serves POST /api/instance-backups.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in CreateExportInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Note) > 500 {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: "note_too_long"})
		return
	}
	if reason := (core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}).Check(); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: reason})
		return
	}
	e, err := h.svc.StartExport(r.Context(), user.Username, "manual", nil, in, h.uploader)
	if errors.Is(err, ErrBusy) {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, InvalidReason{Reason: "busy"})
		return
	}
	if err != nil {
		h.app.Logger.Error("instance backup: starting an export", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action: "create", EntityType: "instance_backup", EntityID: e.ID,
		Details: fmt.Sprintf("key=%s metrics=%t", e.KeyKind, in.IncludeMetrics),
	})
	response.Created(w, e)
}

// HandleDownload serves GET /api/instance-backups/{id}/download. It needs a
// step-up window on top of the permission, like revealing a vault secret.
func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if err := h.requireSudo(r); err != nil {
		response.ForbiddenCode(w, r, i18n.ErrSudoRequired)
		return
	}
	id := chi.URLParam(r, "id")
	p, e, err := h.svc.FilePath(r.Context(), id)
	if h.writeLookupError(w, r, err) {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		h.app.Logger.Error("instance backup: opening an export", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	defer closeLogged(h.app.Logger, f, "export download")
	st, err := f.Stat()
	if err != nil {
		h.app.Logger.Error("instance backup: sizing an export", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Hour)); err != nil {
		h.app.Logger.Warn("instance backup: extending the download deadline", "error", err)
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "download", EntityType: "instance_backup", EntityID: e.ID, EntityName: e.FileName})
	name := strings.TrimPrefix(e.FileName, e.ID+"-")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	response.NoStore(w)
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// VerifyInput carries the key a rehearsal needs to open the export.
type VerifyInput struct {
	Passphrase string `json:"passphrase,omitempty"`
	KeyFile    string `json:"keyFile,omitempty"`
}

// HandleVerify serves POST /api/instance-backups/{id}/verify.
func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	var in VerifyInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	secret := core.Secret{Passphrase: in.Passphrase, KeyFile: in.KeyFile}
	if reason := secret.CheckOpen(); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: reason})
		return
	}
	e, err := h.svc.StartVerify(r.Context(), chi.URLParam(r, "id"), secret)
	if h.writeLookupError(w, r, err) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "verify", EntityType: "instance_backup", EntityID: e.ID, EntityName: e.FileName})
	response.OK(w, e)
}

// HandleDelete serves DELETE /api/instance-backups/{id}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Delete(r.Context(), chi.URLParam(r, "id"), h.uploader)
	if h.writeLookupError(w, r, err) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "delete", EntityType: "instance_backup", EntityID: e.ID, EntityName: e.FileName})
	response.OK(w, map[string]string{"status": "deleted"})
}

func (h *Handler) writeLookupError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
	default:
		h.app.Logger.Error("instance backup: reading an export", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
	return true
}

func (h *Handler) requireSudo(r *http.Request) error {
	if h.app.AuthService == nil {
		return nil
	}
	cookie, err := r.Cookie(auth.SessionCookie)
	if err != nil || cookie.Value == "" {
		return auth.ErrSudoRequired
	}
	ok, err := h.app.AuthService.HasSudoWindow(r.Context(), cookie.Value, core.SudoScopeDownload)
	if err != nil {
		h.app.Logger.Error("instance backup: checking the step-up window", "error", err)
		return auth.ErrSudoRequired
	}
	if !ok {
		return auth.ErrSudoRequired
	}
	return nil
}
