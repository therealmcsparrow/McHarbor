// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Response codes for migration and the remote-viewer file. Keys in the vm
// namespace, like every other code this plugin answers with.
const (
	codeMigrateSameHost    = "err.vm.migrate_same_host"
	codeMigrateInFlight    = "err.vm.migrate_in_flight"
	codeMigrateUnsupported = "err.vm.migrate_unsupported"
	codeMigrateSnapshots   = "err.vm.migrate_snapshots"
	codeMigrateDevices     = "err.vm.migrate_devices"
	codeMigrateShares      = "err.vm.migrate_shares"
	codeMigrateTPM         = "err.vm.migrate_tpm"
	codeMigrateMedia       = "err.vm.migrate_media"
	codeMigrateForbidden   = "err.vm.migrate_forbidden"
	codeMigrateAddress     = "err.vm.migrate_address"
	codeMigrationNone      = "err.vm.migration_none"
	codeSpiceUnavailable   = "err.vm.spice_unavailable"

	msgMigrationStarted   = "msg.vm.migration_started"
	msgMigrationCancelled = "msg.vm.migration_cancel_requested"
)

// permMigrate is the key a migration needs on both hosts.
const permMigrate = "vm.migrate"

// migrationFailure maps the migration and SPICE sentinels onto a status and a
// code, or reports false for any other error.
func migrationFailure(err error) (int, string, bool) {
	switch {
	case errors.Is(err, ErrMigrateSameHost):
		return http.StatusBadRequest, codeMigrateSameHost, true
	case errors.Is(err, ErrMigrateInFlight):
		return http.StatusConflict, codeMigrateInFlight, true
	case errors.Is(err, ErrMigrateUnsupported):
		return http.StatusConflict, codeMigrateUnsupported, true
	case errors.Is(err, ErrMigrateSnapshots):
		return http.StatusConflict, codeMigrateSnapshots, true
	case errors.Is(err, ErrMigrateDevices):
		return http.StatusConflict, codeMigrateDevices, true
	case errors.Is(err, ErrMigrateShares):
		return http.StatusConflict, codeMigrateShares, true
	case errors.Is(err, ErrMigrateTPM):
		return http.StatusConflict, codeMigrateTPM, true
	case errors.Is(err, ErrMigrateMedia):
		return http.StatusConflict, codeMigrateMedia, true
	case errors.Is(err, ErrMigrateForbidden):
		return http.StatusForbidden, codeMigrateForbidden, true
	case errors.Is(err, ErrMigrateAddress):
		return http.StatusBadRequest, codeMigrateAddress, true
	case errors.Is(err, ErrMigrationNone):
		return http.StatusConflict, codeMigrationNone, true
	case errors.Is(err, ErrSpiceUnavailable):
		return http.StatusConflict, codeSpiceUnavailable, true
	}
	return 0, "", false
}

// HandleMigrate serves POST /api/vms/{id}/migrate.
//
// The route middleware checks vm.migrate against the request's ?env=; the
// handler checks it again against both the guest's own host and the target,
// because a grant on one environment must not move a guest onto another.
func (h *Handler) HandleMigrate(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	var req MigrateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	id := chi.URLParam(r, "id")
	vm, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "migrate virtual machine", err)
		return
	}
	if !h.allowedOn(userID, permMigrate, vm.EnvironmentID, req.TargetEnvironmentID) {
		h.respondErr(w, r, "migrate virtual machine", ErrMigrateForbidden)
		return
	}

	status, err := h.svc.Migrate(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "migrate virtual machine", err)
		return
	}
	h.emit(userID, "plugin.kvm.vm.migration_started", map[string]any{
		"vmId": id, "migrationId": status.ID, "name": vm.Name,
		"from": vm.EnvironmentID, "to": req.TargetEnvironmentID, "mode": status.Mode,
	})
	writeJSON(w, http.StatusAccepted, apiEnvelope{Success: true, Data: status, Code: msgMigrationStarted})
}

// HandleMigrationStatus serves GET /api/vms/{id}/migration: the running or last
// migration of a guest, or null when it has none.
func (h *Handler) HandleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	status, found := h.svc.MigrationStatus(chi.URLParam(r, "id"))
	if !found {
		ok(w, nil)
		return
	}
	ok(w, status)
}

// HandleCancelMigration serves POST /api/vms/{id}/migration/cancel.
func (h *Handler) HandleCancelMigration(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	vm, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "cancel migration", err)
		return
	}
	if !h.allowedOn(userID, permMigrate, vm.EnvironmentID) {
		h.respondErr(w, r, "cancel migration", ErrMigrateForbidden)
		return
	}
	if err := h.svc.CancelMigration(r.Context(), id); err != nil {
		h.respondErr(w, r, "cancel migration", err)
		return
	}
	h.emit(userID, "plugin.kvm.vm.migration_cancelled", map[string]any{"vmId": id, "name": vm.Name})
	okMsg(w, msgMigrationCancelled)
}

// allowedOn reports whether userID holds perm on every named environment. No
// RBAC handle means no answer, and no answer means no.
func (h *Handler) allowedOn(userID, perm string, envs ...string) bool {
	if h.rbac == nil {
		return false
	}
	for _, env := range envs {
		if env == "" {
			continue
		}
		allowed, err := h.rbac.HasPermission(userID, env, perm)
		if err != nil {
			h.logger.Warn("kvm: permission check failed", "permission", perm, "env", env, "error", err)
			return false
		}
		if !allowed {
			return false
		}
	}
	return true
}

// relayToken reads the bearer an agent presents.
func relayToken(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// HandleMigrationUpload serves PUT /api/vms/migrations/internal/{grantId}.
//
// Public because the caller is the source agent, which holds no session; the
// one-time send token is its credential. The body is piped straight into the
// target's download and never touches McHarbor's disk.
func (h *Handler) HandleMigrationUpload(w http.ResponseWriter, r *http.Request) {
	_, relay := h.svc.migrationState()
	grantID := chi.URLParam(r, "grantId")
	slot, err := relay.redeem(grantID, relayToken(r), true)
	if err != nil {
		h.logger.Warn("kvm: migration relay upload refused", "grant", grantID, "remote", r.RemoteAddr)
		failErr(w, r, http.StatusUnauthorized, codeUnauthorized, err)
		return
	}
	defer func() { _ = r.Body.Close() }()
	h.extendTransferDeadlines(w)
	if _, err := relay.upload(r.Context(), slot, r.Header.Get("X-McHarbor-Format"), r.ContentLength, r.Body); err != nil {
		h.logger.Error("kvm: migration relay upload failed", "grant", grantID, "error", err)
		failErr(w, r, http.StatusBadGateway, codeCommandFailed, err)
		return
	}
	okMsg(w, msgMigrationStarted)
}

// HandleMigrationDownload serves GET /api/vms/migrations/internal/{grantId}.
//
// Public because the caller is the target agent; the one-time receive token is
// its credential. It waits for the source's upload and streams it through.
func (h *Handler) HandleMigrationDownload(w http.ResponseWriter, r *http.Request) {
	_, relay := h.svc.migrationState()
	grantID := chi.URLParam(r, "grantId")
	slot, err := relay.redeem(grantID, relayToken(r), false)
	if err != nil {
		h.logger.Warn("kvm: migration relay download refused", "grant", grantID, "remote", r.RemoteAddr)
		failErr(w, r, http.StatusUnauthorized, codeUnauthorized, err)
		return
	}
	h.extendTransferDeadlines(w)
	body, format, _, err := relay.download(r.Context(), slot)
	if err != nil {
		h.logger.Error("kvm: migration relay download failed", "grant", grantID, "error", err)
		failErr(w, r, http.StatusGatewayTimeout, codeCommandFailed, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-McHarbor-Format", format)
	// No Content-Length: a short upload must reach the target as a short read
	// it can refuse, not as a response the server pads or truncates.
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		slot.abort(err)
		h.logger.Error("kvm: streaming a migrating disk", "grant", grantID, "error", err)
	}
}

// HandleSpiceFile serves GET /api/vms/{id}/console.vv, a remote-viewer file
// holding a one-time password. Gated by vm.console like every other console,
// and audited at warning because it hands out a credential.
func (h *Handler) HandleSpiceFile(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	file, err := h.svc.SpiceFile(r.Context(), id, strings.TrimSpace(r.URL.Query().Get("host")))
	if err != nil {
		h.respondErr(w, r, "open SPICE console", err)
		return
	}
	h.emit(userID, "plugin.kvm.vm.spice_ticket_issued", map[string]any{"vmId": id})
	w.Header().Set("Content-Type", "application/x-virt-viewer")
	w.Header().Set("Content-Disposition", `attachment; filename="`+file.Name+`"`)
	w.Header().Set("Cache-Control", "no-store, private")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(file.Content); err != nil {
		h.logger.Warn("kvm: writing the SPICE console file", "vm", id, "error", err)
	}
}
