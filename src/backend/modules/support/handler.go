// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// BundleList is the history answer: bundles plus how many finished ones are kept.
type BundleList struct {
	Items []Bundle `json:"items"`
	Keep  int      `json:"keep"`
}

// Handler serves the support routes.
type Handler struct {
	svc   *Service
	audit *audit.Logger
}

// NewHandler returns a Handler over svc; auditLog may be nil in tests.
//
// narrow deps: it needs the service and the audit logger and nothing else from AppDeps,
// so Mount takes AppDeps and hands the handler only those two.
func NewHandler(svc *Service, auditLog *audit.Logger) *Handler {
	return &Handler{svc: svc, audit: auditLog}
}

func (h *Handler) log(r *http.Request, e audit.Entry) {
	if h.audit != nil {
		h.audit.Log(r, e)
	}
}

func badReason(w http.ResponseWriter, r *http.Request, reason string) {
	response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": reason})
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (h *Handler) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	rep, err := h.svc.Diagnostics(r.Context(), splitList(r.URL.Query().Get("check")))
	if errors.Is(err, ErrUnknownCheck) {
		badReason(w, r, "unknown_check")
		return
	}
	if err != nil {
		h.svc.d.Logger.Error("support: diagnostics", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.NoStore(w)
	response.OK(w, rep)
}

func (h *Handler) handleKeySource(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	response.OK(w, h.svc.EncryptionKeySource())
}

func (h *Handler) handleInventory(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	inv, err := h.svc.StateInventory(r.Context())
	if err != nil {
		h.svc.d.Logger.Error("support: state inventory", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, inv)
}

func (h *Handler) handlePreUpgrade(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	rep, err := h.svc.PreUpgrade(r.Context(), r.URL.Query().Get("target"))
	if errors.Is(err, ErrInvalidVersion) {
		badReason(w, r, "invalid_version")
		return
	}
	if err != nil {
		h.svc.d.Logger.Error("support: pre-upgrade check", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, rep)
}

func (h *Handler) handleListBundles(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	list, err := h.svc.ListBundles()
	if err != nil {
		h.svc.d.Logger.Error("support: listing bundles", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, BundleList{Items: list, Keep: h.svc.Keep()})
}

func (h *Handler) handlePreview(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	p, err := h.svc.Preview(r.Context(), splitList(r.URL.Query().Get("categories")))
	if errors.Is(err, ErrNoCategories) || errors.Is(err, ErrBadCategory) {
		badReason(w, r, "invalid_categories")
		return
	}
	if err != nil {
		h.svc.d.Logger.Error("support: bundle preview", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.NoStore(w)
	response.OK(w, p)
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var req BundleRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	b, err := h.svc.StartBundle(req.Categories, user.Username)
	switch {
	case errors.Is(err, ErrNoCategories), errors.Is(err, ErrBadCategory):
		badReason(w, r, "invalid_categories")
		return
	case errors.Is(err, ErrBundleRunning):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "bundle_running"})
		return
	case err != nil:
		h.svc.d.Logger.Error("support: starting bundle", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.log(r, audit.Entry{Action: "support.bundle.generate", Severity: "info", EntityType: "support_bundle", EntityID: b.ID, EntityName: b.ID, Details: "categories: " + strings.Join(b.Categories, ", ")})
	response.Created(w, b)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	b, err := h.svc.BundleByID(chi.URLParam(r, "id"))
	if err != nil {
		response.NotFoundCause(w, r, i18n.ErrNotFound, err)
		return
	}
	response.NoStore(w)
	response.OK(w, b)
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	path, b, err := h.svc.BundlePath(id)
	switch {
	case errors.Is(err, ErrBundleNotReady):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "not_ready"})
		return
	case err != nil:
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		h.svc.d.Logger.Error("support: opening bundle", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	defer func() { closeutil.Report(f.Close(), "f") }()
	h.log(r, audit.Entry{Action: "support.bundle.download", Severity: "info", EntityType: "support_bundle", EntityID: b.ID, EntityName: b.ID})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "mcharbor-support-"+b.ID+".zip"))
	w.Header().Set("Content-Length", fmt.Sprint(b.SizeBytes))
	response.NoStore(w)
	if _, err := io.Copy(w, f); err != nil {
		h.svc.d.Logger.Warn("support: streaming bundle", "id", id, "error", err)
	}
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	b, err := h.svc.DeleteBundle(chi.URLParam(r, "id"))
	switch {
	case errors.Is(err, ErrBundleNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	case errors.Is(err, ErrBundleRunning):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "bundle_running"})
		return
	case err != nil:
		h.svc.d.Logger.Error("support: deleting bundle", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.log(r, audit.Entry{Action: "support.bundle.delete", Severity: "warning", EntityType: "support_bundle", EntityID: b.ID, EntityName: b.ID})
	response.OK(w, map[string]string{"id": b.ID})
}
