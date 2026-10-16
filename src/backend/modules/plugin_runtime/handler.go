// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package plugin_runtime exposes the runtime status of plugin
// contributions to the admin UI. All routes are read-only except
// /reload, which requires plugins.manage (gated in routes.go) and
// re-scans metadata — it does NOT hot-reload plugin code. See
// .references/plugins/architecture.md §10 Phase A scope.
package plugin_runtime

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/snapshotdrivers"
)

// adapterSource is the read-only subset of the plugin runtime that
// the diagnostic surface needs to expose registration state. The
// runtime is captured by interface assertion at request time so the
// plugin_runtime module does not need to hold a setter (the runtime
// itself is constructed by the bootstrap layer, not by this module).
// When the runtime is missing (Windows / macOS dev builds, plugins
// disabled, or boot has not yet completed), the type assertion fails
// and the handler returns an empty map — the same nil-safe precedent
// followed by handleStatus / handleRoutes for the DB+dataDir path.
type adapterSource interface {
	NotificationAdapterSpecs() map[string]plugin.NotificationAdapterSpec
	StorageAdapterSpecs() map[string]plugin.StorageAdapterSpec
	AIAdapterSpecs() map[string]plugin.AIAdapterSpec
	IdentityAdapterSpecs() map[string]plugin.IdentityAdapterSpec
	SnapshotAdapterSpecs() map[string]plugin.SnapshotAdapterSpec
}

// Handler serves the /api/plugins/loader/* diagnostic surface.
// It is a thin pass-through: parse → call Service → respond.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler builds the handler from the shared dependency
// container. The data directory is read from app.Config.DataDir
// to keep this module on the standard single-argument Mount()
// signature used by every other backend module. The plugin catalog
// directories are read from app.Config.PluginCatalogDirs (comma-
// separated MCHARBOR_PLUGIN_CATALOG_DIRS env var) so the icon
// endpoint can serve logos for uninstalled plugins across multiple
// catalog mounts (dev catalog + production catalogs) without
// bundling plugin assets into the runtime image.
func NewHandler(app *router.AppDeps) *Handler {
	var dataDir string
	var catalogDirs []string
	if app != nil && app.Config != nil {
		dataDir = app.Config.DataDir
		catalogDirs = app.Config.PluginCatalogDirs
	}
	return &Handler{
		app: app,
		svc: NewService(app.DB, dataDir, catalogDirs, nil),
	}
}

// SetReloadFn attaches the runtime contribution rescan. It is set
// by the bootstrap layer after construction because it captures
// the plugin registry, which the runtime module does not own.
func (h *Handler) SetReloadFn(fn func() error) {
	h.svc.reloadFn = fn
}

// Mount attaches the diagnostic routes to the supplied router.
// Mount registers the read-only contribution surface. Every route
// here is authenticated but carries no permission gate, deliberately:
// the frontend loader fetches the menu, translations, widgets, themes
// and plugin bundles for every signed-in user, so gating them behind
// plugins.view would leave a user without that key facing an
// unrenderable interface rather than a smaller one.
//
// The one mutating route, POST /plugins/loader/reload, is registered
// in routes.go instead, where the AppDeps needed for its
// plugins.manage gate is in scope. It used to be registered here with
// a comment claiming a handler-level check that handleReload never
// performed.
//
// Paths are relative because the host router already wraps the
// protected group under `/api`; an absolute `/api/plugins/loader/...`
// here would produce `/api/api/plugins/loader/...` and silently 404
// every request.
func (h *Handler) Mount(r *apispec.Router) {
	r.Get("/plugins/loader/status", h.handleStatus)
	r.Get("/plugins/loader/routes", h.handleRoutes)
	r.Get("/plugins/loader/menu", h.handleMenu)
	r.Get("/plugins/loader/widgets", h.handleWidgets)
	r.Get("/plugins/loader/themes", h.handleThemes)
	r.Get("/plugins/loader/nodes", h.handleNodes)
	r.Get("/plugins/loader/i18n", h.handleI18n)
	r.Get("/plugins/loader/tabs", h.handleTabs)
	r.Get("/plugins/loader/cli", h.handleCLI, apispec.Response(CLISpec{}))
	r.Get("/plugins/loader/capabilities", h.handleCapabilities, apispec.Response([]plugincaps.Capability{}))
	r.Get("/plugins/loader/context-menus", h.handleContextMenus)
	r.Get("/plugins/loader/topbar", h.handleTopbar)
	r.Get("/plugins/loader/forms", h.handleForms)
	r.Get("/plugins/loader/notification-adapters", h.handleNotificationAdapters)
	r.Get("/plugins/loader/storage-adapters", h.handleStorageAdapters)
	r.Get("/plugins/loader/ai-adapters", h.handleAIAdapters)
	r.Get("/plugins/loader/identity-adapters", h.handleIdentityAdapters)
	r.Get("/plugins/loader/snapshot-adapters", h.handleSnapshotAdapters)
	r.Get("/plugins/loader/bundle", h.handleBundle)
	r.Get("/plugins/loader/icon", h.handleIcon)
	r.Get("/plugins/loader/asset", h.handleAsset)
}

// --- Handlers -----------------------------------------------------

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListStatuses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin list", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListRoutes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin routes", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleMenu(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListMenu(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin menu", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleWidgets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListWidgets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin widgets", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleThemes(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListThemes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin themes", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin nodes", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "en"
	}
	out, err := h.svc.LoadI18n(r.Context(), lang)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin i18n", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleTabs(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListTabs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin tabs", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleContextMenus(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListContextMenus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin context menus", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleTopbar(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListTopbar(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin topbar", err)
		return
	}
	response.OK(w, out)
}

// handleForms returns the per-plugin form-extension list (config_form,
// setup_guide, selector) keyed by (resource, kind). The frontend's
// PluginFormOutlet component reads this list when rendering an edit
// dialog and picks the highest-priority plugin to mount into the
// slot. Empty list is a valid response — it means no plugin owns
// that resource and the host falls back to its built-in form.
func (h *Handler) handleForms(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListForms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin forms", err)
		return
	}
	response.OK(w, out)
}

func (h *Handler) handleReload(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Reload(r.Context())
	if err != nil {
		if errors.Is(err, ErrReloadUnsupported) {
			writeError(w, http.StatusServiceUnavailable, "reload not supported in this build", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleNotificationAdapters returns the runtime-tracked map of
// registered notification adapter specs (channelType → spec). The
// admin UI uses this to filter the "Add Communication" dialog's
// channel-type picker down to plugin-contributed types only.
//
// Note: this is the *installed plugin* surface. The existing
// /api/communication-channels/capabilities endpoint returns
// channel_types that already have at least one configured channel
// — those two surfaces are orthogonal.
func (h *Handler) handleNotificationAdapters(w http.ResponseWriter, r *http.Request) {
	out := map[string]plugin.NotificationAdapterSpec{}
	if h.app != nil && h.app.PluginAdapters != nil {
		if src, ok := h.app.PluginAdapters.(adapterSource); ok {
			out = src.NotificationAdapterSpecs()
		}
	}
	if out == nil {
		out = map[string]plugin.NotificationAdapterSpec{}
	}
	response.OK(w, out)
}

// handleStorageAdapters returns the runtime-tracked map of
// registered storage adapter specs keyed by LocationType. The
// storage-location dialog uses this to render its type-picker
// grid: "Local" plus one card per installed storage plugin.
// "local" itself is not in the map — the host always renders it
// first because the dialog falls back to it for the empty-state
// case (no plugins installed).
func (h *Handler) handleStorageAdapters(w http.ResponseWriter, r *http.Request) {
	out := map[string]plugin.StorageAdapterSpec{}
	if h.app != nil && h.app.PluginAdapters != nil {
		if src, ok := h.app.PluginAdapters.(adapterSource); ok {
			out = src.StorageAdapterSpecs()
		}
	}
	if out == nil {
		out = map[string]plugin.StorageAdapterSpec{}
	}
	response.OK(w, out)
}

// handleAIAdapters returns the runtime-tracked map of registered
// AI provider adapter specs keyed by ProviderType. The AI provider
// dialog uses this to render its install-aware provider picker:
// every entry corresponds to an installed plugin, and the host's
// hardcoded built-in types are merged client-side.
func (h *Handler) handleAIAdapters(w http.ResponseWriter, r *http.Request) {
	out := map[string]plugin.AIAdapterSpec{}
	if h.app != nil && h.app.PluginAdapters != nil {
		if src, ok := h.app.PluginAdapters.(adapterSource); ok {
			out = src.AIAdapterSpecs()
		}
	}
	if out == nil {
		out = map[string]plugin.AIAdapterSpec{}
	}
	response.OK(w, out)
}

// handleIdentityAdapters returns the runtime-tracked map of
// registered identity provider adapter specs keyed by ProviderType.
// The CreateProviderDialog uses this for its provider-type picker
// when a plugin is the source of a provider type (e.g. identity-oidc,
// identity-saml); built-in types still come from the host's hardcoded
// list and are merged client-side.
func (h *Handler) handleIdentityAdapters(w http.ResponseWriter, r *http.Request) {
	out := map[string]plugin.IdentityAdapterSpec{}
	if h.app != nil && h.app.PluginAdapters != nil {
		if src, ok := h.app.PluginAdapters.(adapterSource); ok {
			out = src.IdentityAdapterSpecs()
		}
	}
	if out == nil {
		out = map[string]plugin.IdentityAdapterSpec{}
	}
	response.OK(w, out)
}

// handleSnapshotAdapters returns the runtime-tracked map of
// registered snapshot adapter specs keyed by Driver. The Snapshots
// tab uses this to render its plugin-aware driver picker: every
// entry corresponds to an installed snapshot plugin (e.g.
// snapshot-zfs, snapshot-synology, snapshot-truenas). Picking one
// auto-fills plugin_slug + driver on the form and renders the
// plugin-declared ConfigSchema fields.
func (h *Handler) handleSnapshotAdapters(w http.ResponseWriter, r *http.Request) {
	// Start from the drivers McHarbor ships itself, then let plugins
	// override by driver name. An operator who installed snapshot-zfs
	// gets that plugin's form and filters; everyone else still sees
	// ZFS, btrfs and LVM without installing anything.
	out := map[string]plugin.SnapshotAdapterSpec{}
	for driver, spec := range snapshotdrivers.Specs() {
		out[driver] = spec
	}
	if h.app != nil && h.app.PluginAdapters != nil {
		if src, ok := h.app.PluginAdapters.(adapterSource); ok {
			for driver, spec := range src.SnapshotAdapterSpecs() {
				out[driver] = spec
			}
		}
	}
	response.OK(w, out)
}

// handleBundle serves a plugin's UMD bundle from the data dir.
// The slug must match `^[a-z][a-z0-9-]{1,40}$`; any other value
// returns 400 so a malicious query string can't escape the data
// dir. The handler reads the file and streams it as
// `application/javascript`. When the bundle is absent the host
// returns 404 and the frontend surfaces an i18n message.
func (h *Handler) handleBundle(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if !validPluginSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid plugin slug", nil)
		return
	}
	file := h.svc.BundleFile(slug)
	if file == "" {
		writeError(w, http.StatusServiceUnavailable, "plugin data dir not configured", nil)
		return
	}
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "bundle not found", nil)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "opening bundle", err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("plugin runtime: closing a served file", "error", err)
		}
	}()
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func validPluginSlug(slug string) bool {
	if slug == "" || len(slug) > 42 {
		return false
	}
	if slug[0] < 'a' || slug[0] > 'z' {
		return false
	}
	for i := 0; i < len(slug); i++ {
		c := slug[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return false
		}
	}
	return true
}

// validPluginIconName accepts a simple icon filename
// (lowercase ASCII letters, digits, dot, underscore, hyphen). It
// rejects path separators, "..", and anything non-ASCII so a
// hostile `name` query parameter cannot escape icons/ or trigger
// a filesystem probe outside the plugin tree.
func validPluginIconName(name string) bool {
	if name == "" || len(name) > 80 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

func validPluginAssetPath(path string) bool {
	if path == "" || len(path) > 200 || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(path), ".css") {
		return false
	}
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-' || c == '/':
		default:
			return false
		}
	}
	return true
}

// handleAsset serves constrained plugin assets used by host-owned
// contribution surfaces. It currently allows CSS theme files only.
func (h *Handler) handleAsset(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if !validPluginSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid plugin slug", nil)
		return
	}
	if !validPluginAssetPath(path) {
		writeError(w, http.StatusBadRequest, "invalid asset path", nil)
		return
	}
	file := h.svc.AssetFile(slug, path)
	if file == "" {
		writeError(w, http.StatusNotFound, "asset not found", nil)
		return
	}
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "asset not found", nil)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "opening asset", err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("plugin runtime: closing a served file", "error", err)
		}
	}()
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// handleIcon serves a plugin's icons/<name> file as
// image/svg+xml (or image/png when the name ends in .png). Slug
// and name are validated the same way as handleBundle — see
// validPluginSlug and validPluginIconName. The icon filename
// originates from the plugin's own StorageAdapterSpec.Icon (or
// plugin.yaml menu/widget entry), so a path-traversal attempt
// here is equivalent to having compromised the plugin tarball
// itself; we still guard against it for defense-in-depth.
func (h *Handler) handleIcon(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !validPluginSlug(slug) {
		writeError(w, http.StatusBadRequest, "invalid plugin slug", nil)
		return
	}
	if !validPluginIconName(name) {
		writeError(w, http.StatusBadRequest, "invalid icon name", nil)
		return
	}
	// `variant=dark` asks for the plugin's dark-mode mark
	// (`<base>_dark.<ext>`); the service falls back to `name` when the
	// plugin ships none, so the caller never has to probe first. The
	// derived name goes through the same validator — a name that only
	// becomes invalid once the suffix is appended (too long) drops back
	// to the default asset rather than reaching the filesystem.
	variant := strings.TrimSpace(r.URL.Query().Get("variant"))
	if variant != "dark" || !validPluginIconName(DarkIconName(name)) {
		variant = ""
	}
	file := h.svc.IconFileVariant(slug, name, variant)
	if file == "" {
		writeError(w, http.StatusNotFound, "icon not found", nil)
		return
	}
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "icon not found", nil)
		return
	}
	contentType := "image/svg+xml; charset=utf-8"
	if strings.HasSuffix(strings.ToLower(name), ".png") {
		contentType = "image/png"
	}
	f, err := os.Open(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "opening icon", err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("plugin runtime: closing a served file", "error", err)
		}
	}()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("plugin runtime: writing a response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string, err error) {
	body := map[string]any{"error": msg}
	if err != nil {
		body["detail"] = err.Error()
	}
	writeJSON(w, status, body)
}
