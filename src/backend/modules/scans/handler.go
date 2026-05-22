// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Handler holds dependencies for scan HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// HandleList returns a paginated list of scans.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	envID := response.ParseEnvID(r)

	items, total, err := h.service.List(r.Context(), envID, page, perPage)
	if err != nil {
		h.app.Logger.Error("scans: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrScanListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleStartScan starts a new vulnerability scan.
func (h *Handler) HandleStartScan(w http.ResponseWriter, r *http.Request) {
	var input StartScanInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.ImageRef == "" {
		response.BadRequestCode(w, r, i18n.ErrScanImageRequired)
		return
	}
	if input.Scanner == "" {
		input.Scanner = "trivy"
	}
	if input.EnvironmentID == "" {
		input.EnvironmentID = response.ParseEnvID(r)
	}

	scan, err := h.service.StartScan(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("scans: start scan error", "error", err, "image", input.ImageRef, "scanner", input.Scanner)
		response.InternalErrorCode(w, r, i18n.ErrScanCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "scan",
		EntityID:   scan.ID,
		Details:    "started " + input.Scanner + " scan for " + input.ImageRef,
	})

	response.Created(w, scan)
}

// HandleGetScan returns a single scan by ID.
func (h *Handler) HandleGetScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	scan, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("scans: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrScanFailed)
		return
	}
	if scan == nil {
		response.NotFoundCode(w, r, i18n.ErrScanNotFound)
		return
	}

	response.OK(w, scan)
}

// HandleGetVulnerabilities returns vulnerabilities for a scan.
func (h *Handler) HandleGetVulnerabilities(w http.ResponseWriter, r *http.Request) {
	scanID := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListVulnerabilities(r.Context(), scanID, page, perPage)
	if err != nil {
		h.app.Logger.Error("scans: vuln list error", "error", err, "scanId", scanID)
		response.InternalErrorCode(w, r, i18n.ErrScanFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleSummary returns aggregated vulnerability counts.
func (h *Handler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	envID := response.ParseEnvID(r)

	summary, err := h.service.Summary(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("scans: summary error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrScanSummaryFailed)
		return
	}

	response.OK(w, summary)
}

// HandleDelete removes a scan and its vulnerabilities.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("scans: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrScanDeleteFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "scan",
		EntityID:   id,
	})

	response.OKMsg(w, r, i18n.MsgScanDeleted)
}

// HandleAvailableScanners returns a list of scanners that are enabled in settings and available.
func (h *Handler) HandleAvailableScanners(w http.ResponseWriter, r *http.Request) {
	settings := coreSettings.ReadScannerSettings(h.app.DB)
	enabled := map[string]bool{
		"trivy": settings.TrivyEnabled,
		"grype": settings.GrypeEnabled,
		"clair": settings.ClairEnabled,
	}
	scanners := h.service.AvailableScanners(enabled)
	response.OK(w, ScannersResponse{
		Scanners:       scanners,
		DefaultScanner: settings.DefaultScanner,
	})
}

// HandleScanByImage returns scans for a specific image reference.
func (h *Handler) HandleScanByImage(w http.ResponseWriter, r *http.Request) {
	imageRef := r.URL.Query().Get("image")
	if imageRef == "" {
		response.BadRequestCode(w, r, i18n.ErrScanImageRequired)
		return
	}

	envID := response.ParseEnvID(r)
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListByImage(r.Context(), imageRef, envID, page, perPage)
	if err != nil {
		h.app.Logger.Error("scans: list by image error", "error", err, "image", imageRef)
		response.InternalErrorCode(w, r, i18n.ErrScanListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
