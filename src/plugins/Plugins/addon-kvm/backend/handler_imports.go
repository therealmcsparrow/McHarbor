// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// importDeadline is how long one import request may take.
//
// The server's 30s ReadTimeout and 120s WriteTimeout are right for an API call
// and wrong for every step of an import. Uploading a disk died mid-body with
// "i/o timeout"; inspecting or converting one died at exactly 120 seconds with
// the connection closed under the handler, which reaches the browser as
// "Failed to fetch" and the server as "context canceled". Each of the long
// import routes pushes this out for its own request.
const importDeadline = 12 * time.Hour

// HandleListImports serves GET /api/vms/imports.
func (h *Handler) HandleListImports(w http.ResponseWriter, r *http.Request) {
	records, err := h.svc.ListImports(r.Context())
	if err != nil {
		h.respondErr(w, r, "list imports", err)
		return
	}
	ok(w, map[string]any{"imports": records, "formats": importExtensions})
}

// HandleUploadImport serves POST /api/vms/imports?environmentId=…
//
// The file part ends the read, so nothing after it is looked at: the filename
// travels on the part header and the environment on the query string, which is
// the only place it can be read before the body starts arriving.
//
// The environment is required because the upload does not stop here: once the
// bytes are on disk they are handed to that host, and everything after runs
// against the host's copy.
func (h *Handler) HandleUploadImport(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	h.extendImportDeadline(w)

	envID := strings.TrimSpace(r.URL.Query().Get("environmentId"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeHostRequired)
		return
	}

	parts, err := r.MultipartReader()
	if err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	var record Import
	received := false
	for {
		part, partErr := parts.NextPart()
		if partErr != nil {
			break
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		record, err = h.svc.ReceiveImport(r.Context(), part.FileName(), envID, part)
		_ = part.Close()
		received = true
		break
	}

	if !received {
		fail(w, r, http.StatusBadRequest, codeImportEmpty)
		return
	}
	if err != nil {
		h.respondErr(w, r, "receive import", err)
		return
	}

	// Handing it to the host is part of the upload, not a later step: until it
	// lands there, the only copy is on the control plane and every operation
	// after this would have to pull it again.
	staged, err := h.svc.StageImportOnHost(r.Context(), record.ID, envID)
	if err != nil {
		h.respondErr(w, r, "stage import on host", err)
		return
	}

	h.emit(userID, "plugin.kvm.import.uploaded", map[string]any{
		"importId": staged.ID,
		"fileName": staged.FileName, "sizeBytes": staged.SizeBytes,
		"sourceKind": staged.SourceKind, "environmentId": envID,
	})
	created(w, staged, msgImportUploaded)
}

// HandleInspectImport serves POST /api/vms/imports/{importId}/inspect.
func (h *Handler) HandleInspectImport(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	h.extendImportDeadline(w)
	importID := chi.URLParam(r, "importId")

	var req struct {
		EnvironmentID string `json:"environmentId"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if strings.TrimSpace(req.EnvironmentID) == "" {
		fail(w, r, http.StatusBadRequest, codeHostRequired)
		return
	}

	record, err := h.svc.InspectImport(r.Context(), importID, req.EnvironmentID)
	if err != nil {
		h.respondErr(w, r, "inspect import", err)
		return
	}

	h.emit(userID, "plugin.kvm.import.inspected", map[string]any{
		"importId":      importID,
		"environmentId": req.EnvironmentID,
	})
	ok(w, record)
}

// HandleConvertImport serves POST /api/vms/imports/{importId}/convert.
func (h *Handler) HandleConvertImport(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	h.extendImportDeadline(w)
	importID := chi.URLParam(r, "importId")

	var req ImportConvertRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	vm, err := h.svc.ConvertImport(r.Context(), importID, req)
	if err != nil {
		h.respondErr(w, r, "convert import", err)
		return
	}

	h.emit(userID, "plugin.kvm.import.converted", map[string]any{
		"importId": importID, "vmId": vm.ID,
		"environmentId": vm.EnvironmentID, "name": vm.Name,
		"diskCount": len(vm.Disks),
	})
	created(w, withheldVM(vm), msgImportConverted)
}

// HandleDeleteImport serves DELETE /api/vms/imports/{importId}.
func (h *Handler) HandleDeleteImport(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	importID := chi.URLParam(r, "importId")

	if err := h.svc.DeleteImport(r.Context(), importID); err != nil {
		h.respondErr(w, r, "delete import", err)
		return
	}

	h.emit(userID, "plugin.kvm.import.deleted", map[string]any{
		"importId": importID,
	})
	okMsg(w, msgImportDeleted)
}

// HandleImportPull serves GET /api/vms/imports/internal/{grantId}.
//
// Public because the caller is an agent and holds no session. It redeems a
// per-transfer grant whose token McHarbor minted moments earlier, so the route
// only ever serves the one staged file it was asked for.
func (h *Handler) HandleImportPull(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	target, importID, granted := h.svc.imports.redeem(grantID, token)
	if !granted {
		h.logger.Warn("kvm: import pull refused", "grant", grantID, "remote", r.RemoteAddr)
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	file, err := os.Open(target)
	if err != nil {
		h.logger.Error("kvm: import pull could not open the staged file", "error", err)
		failErr(w, r, http.StatusNotFound, codeNotFound, err)
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		h.logger.Error("kvm: import pull could not stat the staged file", "error", err)
		failErr(w, r, http.StatusInternalServerError, codeCommandFailed, err)
		return
	}

	// The same deadline problem the upload has, in the other direction: a
	// multi-gigabyte body cannot be written inside the server's WriteTimeout.
	h.extendImportDeadline(w)

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, filepath.Base(target),
		info.ModTime(), &countingFile{File: file, svc: h.svc, importID: importID, total: info.Size()})
}

// extendImportDeadline pushes this request's read and write deadlines out.
func (h *Handler) extendImportDeadline(w http.ResponseWriter) {
	deadline := time.Now().Add(importDeadline)
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(deadline); err != nil {
		h.logger.Warn("kvm: import read deadline not extended", "error", err)
	}
	if err := controller.SetWriteDeadline(deadline); err != nil {
		h.logger.Warn("kvm: import write deadline not extended", "error", err)
	}
}

// HandleCancelImport serves POST /api/vms/imports/{importId}/cancel.
//
// It stops a running inspect or convert and returns the record to the state it
// can be acted on from, leaving the staged upload alone: cancelling a
// conversion must not throw away the 40 GB that was uploaded to feed it.
func (h *Handler) HandleCancelImport(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	importID := chi.URLParam(r, "importId")

	record, err := h.svc.CancelImport(r.Context(), importID)
	if err != nil {
		h.respondErr(w, r, "cancel import", err)
		return
	}

	h.emit(userID, "plugin.kvm.import.cancelled", map[string]any{
		"importId": importID,
	})
	ok(w, record)
}
