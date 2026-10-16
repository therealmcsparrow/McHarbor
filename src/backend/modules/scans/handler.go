// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// validScanStatus reports whether status is one of the four values the
// `scans.status` column stores; an empty string means "no filter" and is
// checked separately by the caller.
func validScanStatus(status string) bool {
	switch status {
	case "pending", "running", "completed", "failed":
		return true
	default:
		return false
	}
}

// Handler holds dependencies for scan HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
	// configs backs the /scans/adapters and /scans/configs subtree —
	// the admin surface that replaced the three hard-coded scanner
	// toggles once the engines became plugins.
	configs *ConfigService
}

// HandleList returns a paginated list of scans, optionally filtered by
// status and a substring match against the image reference or scanner.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	envID := response.ParseEnvID(r)

	status := r.URL.Query().Get("status")
	if status != "" && !validScanStatus(status) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 200 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	items, total, err := h.service.List(r.Context(), envID, page, perPage, status, search)
	if err != nil {
		h.app.Logger.Error("scans: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScanListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleStartScan starts a new vulnerability scan.
func (h *Handler) HandleStartScan(w http.ResponseWriter, r *http.Request) {
	var input StartScanInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.ImageRef == "" {
		response.BadRequestCode(w, r, i18n.ErrScanImageRequired)
		return
	}
	// An empty scanner is no longer defaulted to "trivy" here. The
	// estate's default lives on a `scanner_configs` row, so hard-coding
	// an engine at this layer would override an operator who chose
	// Grype — and fail outright on an install where Trivy is not the
	// plugin they installed. Service.Resolve reads the flag.
	if input.EnvironmentID == "" {
		input.EnvironmentID = response.ParseEnvID(r)
	}

	scan, err := h.service.StartScan(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("scans: start scan error", "error", err, "image", input.ImageRef, "scanner", input.Scanner)
		switch {
		case errors.Is(err, ErrNoScannerConfigured):
			// Nothing to scan with. A 400 naming the real cause sends
			// the operator to the Scanners tab; a 500 would send them
			// to the logs for a problem that is not a fault.
			response.BadRequestCode(w, r, i18n.ErrNoScannerConfigured)
		case errors.Is(err, ErrScannerUnavailable):
			response.BadRequestCode(w, r, i18n.ErrScannerNotAvailable)
		case errors.Is(err, ErrScannerAdapterUnknown), errors.Is(err, ErrScannerNotConfigured):
			response.NotFoundCode(w, r, i18n.ErrScannerNotConfigured)
		default:
			response.InternalErrorCode(w, r, i18n.ErrScanCreateFailed)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "scan",
		EntityID:   scan.ID,
		EntityName: scan.ImageRef,
		Details:    "started " + scan.Scanner + " scan for " + input.ImageRef,
	})

	response.Created(w, scan)
}

// HandleGetScan returns a single scan by ID.
func (h *Handler) HandleGetScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	scan, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("scans: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrScanFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrScanFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrScanSummaryFailed, err)
		return
	}

	response.OK(w, summary)
}

// HandleDelete removes a scan and its vulnerabilities.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Read before the delete: afterwards there is no scan left to name.
	imageRef := ""
	if scan, scanErr := h.service.ByID(r.Context(), id); scanErr == nil && scan != nil {
		imageRef = scan.ImageRef
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("scans: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrScanDeleteFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "scan",
		EntityID:   id,
		EntityName: imageRef,
	})

	response.OKMsg(w, r, i18n.MsgScanDeleted)
}

// HandlePurge deletes every scan (and its vulnerabilities) older
// than the configured retention cutoff. The action is recorded as
// a new audit entry so the user can see when the purge happened.
// When the ?vacuum=true query parameter is set the handler also
// triggers a WAL checkpoint + background VACUUM.
func (h *Handler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	deleted, days, err := h.service.PurgeByRetention(r.Context())
	if err != nil {
		h.app.Logger.Error("scans: purge failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "purge",
		EntityType: "scans",
		Details:    fmt.Sprintf("cleared %d scans older than %d days", deleted, days),
	})
	vacuuming := false
	if r.URL.Query().Get("vacuum") == "true" && h.app.Compact != nil {
		_ = h.app.Compact.CheckpointAndVacuum(r.Context())
		vacuuming = true
	}
	response.OK(w, map[string]any{
		"deleted":       deleted,
		"retentionDays": days,
		"vacuuming":     vacuuming,
	})
}

// HandleAvailableScanners returns the enabled scanner instances a scan
// can be started with, plus which one is the estate default.
//
// The old version read three booleans out of the `scanners` settings
// category and intersected them with a built-in registry. Both halves
// are gone: a scanner is now a `scanner_configs` row, and its enabled
// flag lives on the row.
func (h *Handler) HandleAvailableScanners(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.AvailableScanners(r.Context())
	if err != nil {
		h.app.Logger.Error("scans: listing available scanners failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScannerListFailed, err)
		return
	}
	response.OK(w, resp)
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
		response.InternalErrorCause(w, r, i18n.ErrScanListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// scanEnvironment is the rbac.PinEnvToResource resolver for /scans/{id}.
func (h *Handler) scanEnvironment(r *http.Request) (string, bool, error) {
	envID, found, err := h.service.ScanEnvironment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.app.Logger.Error("scans: resolving the scan's environment", "error", err)
	}
	return envID, found, err
}
