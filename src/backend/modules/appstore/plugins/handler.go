// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// Handler is the HTTP boundary for the appstore plugins surface.
// Handlers are thin: parse, validate, call service, audit, respond.
// All SQL lives in service.go. Mirrors .references/plugins/
// architecture.md §8.2 routes.
type Handler struct {
	installer Installer
	svc       *Service
	audit     *audit.Logger
	dataDir   string
	// onChange fires after a successful Install, Uninstall, Enable,
	// or Disable so the host can rebuild contribution tables +
	// adapter registries without a process restart. nil is fine —
	// the host simply skips the callback.
	onChange func(slug string)
}

// Installer exposes the installer so the host can attach a package store to it.
func (h *Handler) Installer() Installer { return h.installer }

// NewHandler builds the handler with its dependencies.
//
// narrow deps: no *router.AppDeps on purpose. The plugin host reaches none of
// the shared app services — no Docker pool, no encryption service, no RBAC —
// and taking the whole container would hand this package access it has no use
// for. Installer, Service and the audit logger are the entire surface.
func NewHandler(installer Installer, svc *Service, auditLog *audit.Logger) *Handler {
	return &Handler{
		installer: installer,
		svc:       svc,
		audit:     auditLog,
	}
}

// NewHandlerWithDataDir returns a handler that also exposes the
// installed plugin's manifest via GET /api/appstore/plugins/{slug}.
// The data directory is needed to read plugin.yaml from disk.
func NewHandlerWithDataDir(installer Installer, svc *Service, auditLog *audit.Logger, dataDir string) *Handler {
	return &Handler{
		installer: installer,
		svc:       svc,
		audit:     auditLog,
		dataDir:   dataDir,
	}
}

// SetRuntimeLoaded passes the plugin runtime's view of which plugins are loaded
// through to the service, so the installed list reports the loader's answer
// instead of false for every row.
func (h *Handler) SetRuntimeLoaded(fn func() map[string]bool) {
	h.svc.SetRuntimeLoaded(fn)
}

// SetOnChange registers a callback fired after a successful
// Install / Uninstall / Enable / Disable. The host wires this to the
// runtime reload path so newly installed plugins become visible
// (routes registered, storage/AI/comm/identity/email adapter specs
// picked up) without restarting the process. slug is the affected
// plugin; pass "" for changes that don't pertain to a single plugin
// (none today, reserved for future global rescans).
func (h *Handler) SetOnChange(fn func(slug string)) {
	h.onChange = fn
}

func (h *Handler) notifyChange(slug string) {
	if h.onChange != nil {
		h.onChange(slug)
	}
}

// The routes this handler serves are registered in routes.go, where
// every one of them is wrapped in its plugins.* permission gate. An
// ungated Mount used to live here and was what the host actually
// called, which left plugin install, configure, update and uninstall
// reachable by any authenticated account. It is gone on purpose: two
// mount functions for one route set is how the gated one ends up
// unused again.

// --- List ----------------------------------------------------------

func (h *Handler) handleListInstalled(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListInstalled(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.OK(w, rows)
}

// handleListCatalog returns the browsable plugin catalog: every
// entry in the local catalog directory (mounted at the path
// configured via MCHARBOR_PLUGIN_CATALOG_DIR). Pure read; no
// audit emit (matches /app-store catalog reads).
//
// Empty result (no catalog configured) returns []PluginEntry{}
// with HTTP 200 so the UI can render its empty/unconfigured
// state without an error toast.
func (h *Handler) handleListCatalog(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListCatalog(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if rows == nil {
		rows = []PluginEntry{}
	}
	response.OK(w, rows)
}

// --- Get -----------------------------------------------------------

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	rows, err := h.svc.ListInstalled(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var match *InstalledPlugin
	for i := range rows {
		if rows[i].Slug == slug {
			match = &rows[i]
			break
		}
	}
	if match == nil {
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
		return
	}
	resp := PluginDetail{InstalledPlugin: *match}
	if h.dataDir != "" {
		m, err := h.svc.LookupManifest(r.Context(), h.dataDir, slug)
		if err != nil {
			// Not fatal — the row is still worth returning — but never again
			// silent. Swallowing this is what let the manifest be parsed with
			// encoding/json for as long as it was: every plugin simply looked
			// like one that declares no settings.
			slog.Warn("plugin detail: manifest unreadable", "slug", slug, "error", err)
		}
		resp.Manifest = m
	}
	response.OK(w, resp)
}

// --- Install -------------------------------------------------------

// withCatalogStamp takes the build stamp and target release from the catalog version
// the install URL names. Without it a fresh install records no build, and the store
// offers an "update" to the very artifact that was just installed.
func (h *Handler) withCatalogStamp(ctx context.Context, src Source) Source {
	if src.URL == "" || src.Build != "" || h.svc == nil {
		return src
	}
	entries, err := h.svc.ListCatalog(ctx)
	if err != nil {
		return src
	}
	for _, e := range entries {
		for _, v := range e.Versions {
			if v.DownloadURL == src.URL {
				src.Build, src.BuiltFor = v.Build, v.BuiltFor
				if src.Signature == "" {
					src.Signature = v.Signature
				}
				return src
			}
		}
		if e.DownloadURL == src.URL {
			src.Build, src.BuiltFor = e.Build, e.BuiltFor
			return src
		}
	}
	return src
}

// handleInstallStream is the SSE variant of handleInstall. The catalog
// install UI (useStreamPlugin.startInstall) expects progress events;
// the JSON endpoint above blocks for the full duration with no
// feedback. The plumbing was scaffolded in stream.go and installer.go
// (the ProgressFunc parameter + the SSEWriter); only the route
// registration + this wrapper were missing.
//
// Wire shape mirrors the docker-app install stream at
// /api/app-store/install/stream so the same frontend InstallProgress
// component renders plugin installs without per-plugin branches.
func (h *Handler) handleInstallStream(w http.ResponseWriter, r *http.Request) {
	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.URL == "" && len(req.Tarball) == 0 {
		response.BadRequestCode(w, r, i18n.ErrPluginURLOrTarballRequired)
		return
	}
	src := h.withCatalogStamp(r.Context(), Source{
		URL:           req.URL,
		Checksum:      req.Checksum,
		Signature:     req.Signature,
		AllowUnsigned: req.AllowUnsigned,
	})
	if len(req.Tarball) > 0 {
		src.Tarball = req.Tarball
	}

	sw, err := newSSEWriter(w)
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	inst, err := h.installer.Install(r.Context(), src, sw.progress)
	if err != nil {
		slog.Warn("plugin install failed", "error", err)
		sw.writeError(streamErrorCode(err, i18n.ErrPluginInstallFailed), failureText("plugin install failed", err))
		return
	}
	// An upgrade a restart cut short may have left this plugin's grants in a
	// snapshot; installing it again is when they can come back.
	h.applyPendingSnapshot(r.Context(), inst.Slug)
	h.notifyChange(inst.Slug)
	sw.writeDone(InstallResult{
		Slug:        inst.Slug,
		Version:     inst.Version,
		Path:        inst.Path,
		NeedsEnable: inst.NeedsEnable,
	})
}

// handleUpdateStream re-resolves a plugin's catalog entry by slug and
// triggers the same install pipeline. The catalog is the source of
// truth for which version to install — we don't track the source URL
// per installed row. SSE mirrors the install stream so the UI shows
// the same progress events during an update.
//
// The Install pipeline refuses an already-installed slug
// (ErrAlreadyInstalled), so an update is an uninstall followed by an
// install — but only after the new artifact passed preflight, and with
// the previous version set aside so a failed install is rolled back.
// Configured plugin_config rows persist across the cycle because the
// host's appstore_plugins row is what drives Enable/Disable state, not
// plugin_config. The user keeps any values they entered into the
// plugin's ConfigForm.
func (h *Handler) handleUpdateStream(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	// The body is optional: an empty one is an update that accepts only a
	// signed release, which is what every pre-existing caller sends.
	var req UpdateRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}

	entries, err := h.svc.ListCatalog(r.Context())
	if err != nil {
		response.InternalErrorCause(w, r, i18n.ErrPluginListFailed, err)
		return
	}
	var match *PluginEntry
	for i := range entries {
		if entries[i].Slug == slug {
			match = &entries[i]
			break
		}
	}
	if match == nil || match.DownloadURL == "" {
		sw, sErr := newSSEWriter(w)
		if sErr == nil {
			sw.writeError(i18n.ErrPluginUpdateNoCatalogEntry, "no catalog entry for "+slug)
		}
		return
	}

	sw, sErr := newSSEWriter(w)
	if sErr != nil {
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, sErr)
		return
	}

	src := Source{
		URL:           match.DownloadURL,
		Checksum:      match.Checksum,
		Signature:     match.Signature,
		Build:         match.Build,
		BuiltFor:      match.BuiltFor,
		AllowUnsigned: req.AllowUnsigned,
	}
	// The new artifact is validated before the installed one is touched, the
	// grant snapshot is persisted, and a failed install rolls back; see upgrade.
	inst, err := h.upgrade(r.Context(), slug, src, sw.progress)
	if err != nil {
		slog.Warn("plugin update failed", "slug", slug, "error", err)
		sw.writeError(streamErrorCode(err, i18n.ErrPluginUpdateFailed), failureText(upgradeOutcome(err), err))
		return
	}
	h.notifyChange(inst.Slug)
	sw.writeDone(InstallResult{
		Slug:        inst.Slug,
		Version:     inst.Version,
		Path:        inst.Path,
		NeedsEnable: inst.NeedsEnable,
	})
}

// --- Uninstall -----------------------------------------------------

func (h *Handler) handleUninstall(w http.ResponseWriter, r *http.Request) {
	var req UninstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Slug == "" {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	res, err := h.installer.Uninstall(r.Context(), req.Slug, UninstallOptions{PurgeData: req.PurgeData}, nil)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.notifyChange(req.Slug)
	roles := res.RolesStripped
	if roles == nil {
		roles = []string{}
	}
	response.OK(w, map[string]any{
		"slug":           req.Slug,
		"status":         "uninstalled",
		"roles_stripped": roles,
	})
}

// --- Enable / Disable ---------------------------------------------

// --- Configure -----------------------------------------------------

// --- Config (Phase A4) --------------------------------------------

// --- error mapping -------------------------------------------------

// streamErrorCode picks the i18n code for a failed install or update stream.
// The two refusals an operator can act on get their own code, so the dialog
// can ask for consent instead of reporting a generic failure.
func streamErrorCode(err error, fallback i18n.MsgCode) i18n.MsgCode {
	switch {
	case errors.Is(err, ErrUnsignedNotConfirmed):
		return i18n.ErrPluginUnsignedUnconfirmed
	case errors.Is(err, ErrSignatureInvalid):
		return i18n.ErrPluginSignatureInvalid
	case errors.Is(err, ErrManifestInvalid):
		return i18n.ErrPluginManifestInvalid
	case errors.Is(err, ErrBuiltForMismatch):
		return i18n.ErrPluginBuiltForMismatch
	default:
		return fallback
	}
}

// fail maps installer errors to i18n error codes per api-routes rule
// §6 (never expose the raw error text to clients). It is logged
// server-side for debugging but the client only receives a stable
// machine-readable code.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrSignatureInvalid):
		response.BadRequestCode(w, r, i18n.ErrPluginSignatureInvalid)
	case errors.Is(err, ErrUnsignedNotConfirmed):
		response.BadRequestCode(w, r, i18n.ErrPluginUnsignedUnconfirmed)
	case errors.Is(err, ErrManifestInvalid):
		// The raw error text is intentionally not returned to the client;
		// it is logged below via the default branch.
		response.BadRequestCode(w, r, i18n.ErrPluginManifestInvalid)
	case errors.Is(err, ErrAlreadyInstalled):
		response.ConflictCode(w, r, i18n.ErrPluginAlreadyInstalled)
	case errors.Is(err, ErrPermissionInUse):
		response.ConflictCode(w, r, i18n.ErrPluginPermissionInUse)
	case errors.Is(err, ErrPluginNotFound):
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
	case errors.Is(err, ErrPlatformUnsupported):
		response.BadRequestCode(w, r, i18n.ErrPlatformUnsupported)
	case errors.Is(err, ErrBuiltForMismatch):
		slog.Error("plugin built for another release", "error", err)
		response.BadRequestCause(w, r, i18n.ErrPluginBuiltForMismatch, err)
	default:
		// Untyped error: the installer bubble-wrapped something we
		// didn't enumerate above. Log the raw error server-side so the
		// operator can see the actual cause (the previous default
		// returned ErrPluginInstallFailed for uninstall failures too,
		// which both lied to the user and silently dropped the
		// diagnostic). Default to the remove code — this branch is
		// reached by both install and uninstall paths, and remove is
		// the safer of the two to surface for an unknown failure.
		slog.Error("plugin operation failed",
			"path", r.URL.Path,
			"method", r.Method,
			"error", err,
		)
		response.BadRequestCause(w, r, i18n.ErrPluginRemoveFailed, err)
	}
}
