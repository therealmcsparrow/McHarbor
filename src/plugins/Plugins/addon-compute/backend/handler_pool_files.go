// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// maxPoolWriteBytes bounds an inline file write.
//
// It matches the agent's own inline limit rather than the request body limit:
// a larger body would be accepted here and then refused a network hop later,
// which reads to the operator as a transport fault rather than as a rule.
const maxPoolWriteBytes = 2 << 20

// poolFileBody is the request shape every mutating pool file route takes.
type poolFileBody struct {
	Path      string `json:"path"`
	NewName   string `json:"newName,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Content   string `json:"content,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

// HandleBrowsePool serves GET /api/compute/pools/{id}/files?path=.
func (h *Handler) HandleBrowsePool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "id")
	if poolID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	entries, err := h.svc.BrowsePool(r.Context(), poolID, r.URL.Query().Get("path"))
	if err != nil {
		h.respondErr(w, r, "browse pool", err)
		return
	}
	ok(w, entries)
}

// HandleReadPoolFile serves GET /api/compute/pools/{id}/files/content?path=.
//
// The body is returned as text rather than as a download: this is the editor's
// read, and anything over the agent's inline limit is refused there with a
// message naming the limit.
func (h *Handler) HandleReadPoolFile(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if poolID == "" || path == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	content, err := h.svc.PoolFileOperation(r.Context(), poolID, PoolFileRequest{
		Operation: "read",
		Path:      path,
	})
	if err != nil {
		h.respondErr(w, r, "read pool file", err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(content); err != nil {
		h.logger.Warn("pool file read write failed", "pool", poolID, "error", err)
	}
}

// HandleWritePoolFile serves PUT /api/compute/pools/{id}/files/content.
func (h *Handler) HandleWritePoolFile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "id")
	var body poolFileBody
	if poolID == "" || decode(r, &body) != nil || body.Path == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	if len(body.Content) > maxPoolWriteBytes {
		fail(w, r, http.StatusRequestEntityTooLarge, codeUploadTooLarge)
		return
	}

	if _, err := h.svc.PoolFileOperation(r.Context(), poolID, PoolFileRequest{
		Operation: "write",
		Path:      body.Path,
		Mode:      body.Mode,
		Content:   []byte(body.Content),
	}); err != nil {
		h.respondErr(w, r, "write pool file", err)
		return
	}

	h.emit(userID, "compute.pool.file.write", map[string]any{"poolId": poolID, "path": body.Path})
	ok(w, map[string]any{"path": body.Path})
}

// HandleCreatePoolDir serves POST /api/compute/pools/{id}/files/mkdir.
func (h *Handler) HandleCreatePoolDir(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "id")
	var body poolFileBody
	if poolID == "" || decode(r, &body) != nil || body.Path == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	if _, err := h.svc.PoolFileOperation(r.Context(), poolID, PoolFileRequest{
		Operation: "mkdir",
		Path:      body.Path,
		Mode:      body.Mode,
	}); err != nil {
		h.respondErr(w, r, "create pool directory", err)
		return
	}

	h.emit(userID, "compute.pool.file.mkdir", map[string]any{"poolId": poolID, "path": body.Path})
	ok(w, map[string]any{"path": body.Path})
}

// HandleRenamePoolFile serves POST /api/compute/pools/{id}/files/rename.
func (h *Handler) HandleRenamePoolFile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "id")
	var body poolFileBody
	if poolID == "" || decode(r, &body) != nil || body.Path == "" || body.NewName == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	if _, err := h.svc.PoolFileOperation(r.Context(), poolID, PoolFileRequest{
		Operation: "rename",
		Path:      body.Path,
		NewName:   body.NewName,
	}); err != nil {
		h.respondErr(w, r, "rename pool file", err)
		return
	}

	h.emit(userID, "compute.pool.file.rename", map[string]any{
		"poolId": poolID, "path": body.Path, "newName": body.NewName,
	})
	ok(w, map[string]any{"path": body.Path, "newName": body.NewName})
}

// HandleDeletePoolFile serves DELETE /api/compute/pools/{id}/files.
//
// Recursive is taken from the query string rather than a body: several HTTP
// clients drop a DELETE body, and a flag that silently becomes false would turn
// "remove this directory" into a confusing failure.
func (h *Handler) HandleDeletePoolFile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if poolID == "" || path == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	recursive := r.URL.Query().Get("recursive") == "true"
	if _, err := h.svc.PoolFileOperation(r.Context(), poolID, PoolFileRequest{
		Operation: "delete",
		Path:      path,
		Recursive: recursive,
	}); err != nil {
		h.respondErr(w, r, "delete pool file", err)
		return
	}

	h.emit(userID, "compute.pool.file.delete", map[string]any{
		"poolId": poolID, "path": path, "recursive": recursive,
	})
	ok(w, map[string]any{"path": path})
}
