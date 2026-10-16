// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// ErrReloadUnsupported is returned by Service.Reload when no reload
// function was wired in (e.g. on Windows / macOS or when plugins
// are disabled). Handlers map this to HTTP 503.
var ErrReloadUnsupported = errors.New("plugin reload not supported in this build")

// Response shapes — exported so the handler can reuse them in the
// JSON encoder without redeclaring field tags.

type statusEntry struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Loaded  bool   `json:"loaded"`
	Error   string `json:"error,omitempty"`
	// BundleMtime is the unix-nanosecond mtime of the plugin's UMD
	// bundle file on disk, or 0 when no bundle is present. The
	// frontend appends this as `&v=<mtime>` on bundle fetches so
	// any tarball reinstall invalidates the browser cache without
	// requiring a hard reload — the previous version-pinned
	// cache buster served stale 187 KB bundles after a fix.
	BundleMtime int64 `json:"bundle_mtime,omitempty"`
}

type routeEntry struct {
	Slug               string `json:"plugin_slug"`
	Method             string `json:"method"`
	Path               string `json:"path"`
	RequiresPermission string `json:"requires_permission,omitempty"`
	Public             bool   `json:"public,omitempty"`
}

type menuEntry struct {
	Slug     string `json:"plugin_slug"`
	Parent   string `json:"parent"`
	LabelNS  string `json:"label_ns"`
	LabelKey string `json:"label_key"`
	Path     string `json:"path"`
	Icon     string `json:"icon,omitempty"`
	Required string `json:"required_permission,omitempty"`
}

type widgetEntry struct {
	Slug        string `json:"plugin_slug"`
	Key         string `json:"key"`
	Title       string `json:"title"`
	Category    string `json:"category,omitempty"`
	DefaultSize string `json:"default_size,omitempty"`
	// Settings is the descriptor's free-form map (description, icon,
	// min_width, min_height) as the plugin registered it.
	Settings json.RawMessage `json:"settings,omitempty"`
}

type nodeEntry struct {
	Slug        string `json:"plugin_slug"`
	Key         string `json:"key"`
	Category    string `json:"category"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon,omitempty"`
	// ConfigSchema is the per-node config UI the plugin ships. The
	// loader marshals it to JSON before INSERT; the SPA decodes it
	// directly into the workflow canvas's `ConfigField` type so a
	// plugin node renders the same NodeConfigPanel a built-in node
	// would.
	ConfigSchema []json.RawMessage `json:"config_schema,omitempty"`
	// InputPorts / OutputPorts are flat string arrays (port names).
	// Empty defaults to no ports; the workflow canvas then shows the
	// single fallback port.
	InputPorts         []string `json:"input_ports,omitempty"`
	OutputPorts        []string `json:"output_ports,omitempty"`
	RequiresPermission string   `json:"requires_permission,omitempty"`
}

type themeEntry struct {
	Slug           string `json:"plugin_slug"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	CSSPath        string `json:"css_path"`
	SwatchHue      int    `json:"swatch_hue"`
	DefaultVariant string `json:"default_variant"`
	HasDark        bool   `json:"has_dark"`
	HasLight       bool   `json:"has_light"`
}

type tabEntry struct {
	Slug       string `json:"plugin_slug"`
	PageID     string `json:"page_id"`
	Key        string `json:"key"`
	LabelNS    string `json:"label_ns"`
	LabelKey   string `json:"label_key"`
	Icon       string `json:"icon,omitempty"`
	SortOrder  int    `json:"sort_order,omitempty"`
	Permission string `json:"required_permission,omitempty"`
}

type contextMenuEntry struct {
	Slug       string `json:"plugin_slug"`
	PageID     string `json:"page_id"`
	Key        string `json:"key"`
	LabelNS    string `json:"label_ns"`
	LabelKey   string `json:"label_key"`
	Icon       string `json:"icon,omitempty"`
	SortOrder  int    `json:"sort_order,omitempty"`
	Permission string `json:"required_permission,omitempty"`
}

type topbarEntry struct {
	Slug       string `json:"plugin_slug"`
	Slot       string `json:"slot"`
	LabelNS    string `json:"label_ns"`
	LabelKey   string `json:"label_key"`
	Icon       string `json:"icon,omitempty"`
	Path       string `json:"path,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// formEntry mirrors plugin.FormDecl with the plugin_slug field the
// frontend expects. The host edit dialog queries /plugins/loader/forms
// to decide which plugin's UMD bundle to mount into a given
// (resource, kind) slot.
type formEntry struct {
	Slug     string `json:"plugin_slug"`
	Resource string `json:"resource"`
	Kind     string `json:"kind"`
	Key      string `json:"key,omitempty"`
}

type i18nResponse struct {
	Lang       string                       `json:"lang"`
	Namespaces map[string]map[string]string `json:"namespaces"`
}

func splitSlugVersion(dir string) (string, string, bool) {
	idx := strings.LastIndex(dir, "@")
	if idx <= 0 || idx == len(dir)-1 {
		return "", "", false
	}
	return dir[:idx], dir[idx+1:], true
}

// Service contains all the read-side queries and filesystem probes
// the plugin_runtime handler exposes. Handlers must call into this
// service rather than touching the database or the on-disk plugin
// directory directly.
type Service struct {
	db          *sql.DB
	dataDir     string
	catalogDirs []string
	// reloadFn re-scans contribution tables. It is nil when the
	// plugin runtime is not wired (Windows / macOS, or all plugins
	// disabled); the corresponding handler then returns 503.
	reloadFn func() error
}

// NewService wires the service. dataDir is the McHarbor data
// directory (where $DATA_DIR/plugins/ lives). catalogDirs is a list
// of read-only catalog directories the icon endpoint falls back to
// for uninstalled plugins — each entry is scanned in order, the
// first directory containing a matching icons/<slug>/<name> wins.
// Empty list disables the fallback. reloadFn is the runtime's
// contribution rescan; pass nil when not supported.
func NewService(db *sql.DB, dataDir string, catalogDirs []string, reloadFn func() error) *Service {
	return &Service{db: db, dataDir: dataDir, catalogDirs: catalogDirs, reloadFn: reloadFn}
}

// ListStatuses returns the per-plugin roster pulled from
// appstore_plugins. Each entry's Loaded field is filled in based
// on whether the on-disk .so file is present.
func (s *Service) ListStatuses(ctx context.Context) ([]statusEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT slug, version, enabled FROM appstore_plugins ORDER BY slug ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin list: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()

	out := make([]statusEntry, 0, 32)
	for rows.Next() {
		var e statusEntry
		if err := rows.Scan(&e.Slug, &e.Version, &e.Enabled); err != nil {
			return nil, fmt.Errorf("scanning plugin row: %w", err)
		}
		e.Loaded = s.soExists(e.Slug, e.Version)
		if bf := s.BundleFile(e.Slug); bf != "" {
			if st, err := os.Stat(bf); err == nil {
				e.BundleMtime = st.ModTime().UnixNano()
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListRoutes returns every route contributed by every plugin,
// ordered by origin then path. The is_public column is decoded
// into the boolean Public field.
func (s *Service) ListRoutes(ctx context.Context) ([]routeEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, method, path, COALESCE(requires_permission, ''), is_public
		  FROM plugin_routes ORDER BY origin ASC, path ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin routes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]routeEntry, 0, 64)
	for rows.Next() {
		var e routeEntry
		var pub int
		if err := rows.Scan(&e.Slug, &e.Method, &e.Path, &e.RequiresPermission, &pub); err != nil {
			return nil, fmt.Errorf("scanning plugin route: %w", err)
		}
		e.Public = pub == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListMenu returns the sidebar entries contributed by every plugin.
func (s *Service) ListMenu(ctx context.Context) ([]menuEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, COALESCE(parent, ''), label_ns, label_key, path,
		       COALESCE(icon, ''), COALESCE(permission, '')
		  FROM plugin_menu ORDER BY priority DESC, origin ASC, sort_order ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin menu: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]menuEntry, 0, 32)
	for rows.Next() {
		var e menuEntry
		if err := rows.Scan(&e.Slug, &e.Parent, &e.LabelNS, &e.LabelKey,
			&e.Path, &e.Icon, &e.Required); err != nil {
			return nil, fmt.Errorf("scanning plugin menu: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListWidgets returns the dashboard widgets contributed by every plugin.
func (s *Service) ListWidgets(ctx context.Context) ([]widgetEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, key, title, COALESCE(category, ''), default_size, COALESCE(settings_json, '{}')
		  FROM plugin_widgets ORDER BY origin ASC, key ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin widgets: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]widgetEntry, 0, 32)
	for rows.Next() {
		var e widgetEntry
		var settings string
		if err := rows.Scan(&e.Slug, &e.Key, &e.Title, &e.Category, &e.DefaultSize, &settings); err != nil {
			return nil, fmt.Errorf("scanning plugin widget: %w", err)
		}
		if json.Valid([]byte(settings)) {
			e.Settings = json.RawMessage(settings)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListNodes returns the workflow-canvas nodes contributed by every
// plugin. The per-node config_schema / input_ports / output_ports
// columns are JSON-encoded by the loader (see plugin.NodeDescriptor
// + nodeRegistry.Add); this function unmarshals them into the JSON
// shape the workflow canvas expects.
func (s *Service) ListNodes(ctx context.Context) ([]nodeEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, key, category, display_name, COALESCE(icon, ''),
		       COALESCE(config_schema_json, '[]'), COALESCE(input_ports_json, '[]'),
		       COALESCE(output_ports_json, '[]'), COALESCE(requires_permission, '')
		  FROM plugin_nodes ORDER BY origin ASC, key ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin nodes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]nodeEntry, 0, 32)
	for rows.Next() {
		var e nodeEntry
		var configJSON, inputsJSON, outputsJSON string
		if err := rows.Scan(&e.Slug, &e.Key, &e.Category, &e.DisplayName, &e.Icon,
			&configJSON, &inputsJSON, &outputsJSON, &e.RequiresPermission); err != nil {
			return nil, fmt.Errorf("scanning plugin node: %w", err)
		}
		if configJSON != "" && configJSON != "[]" {
			if err := json.Unmarshal([]byte(configJSON), &e.ConfigSchema); err != nil {
				// A row written by a plugin built against the older
				// map-shaped descriptor holds an object. Read its
				// "fields" member when there is one; otherwise ship
				// the node with no config rather than failing the
				// whole palette for every plugin at once.
				var legacy struct {
					Fields []json.RawMessage `json:"fields"`
				}
				if legacyErr := json.Unmarshal([]byte(configJSON), &legacy); legacyErr != nil {
					return nil, fmt.Errorf("decoding config_schema for %s.%s: %w", e.Slug, e.Key, err)
				}
				e.ConfigSchema = legacy.Fields
			}
		}
		if inputsJSON != "" && inputsJSON != "[]" {
			if err := json.Unmarshal([]byte(inputsJSON), &e.InputPorts); err != nil {
				return nil, fmt.Errorf("decoding input_ports for %s.%s: %w", e.Slug, e.Key, err)
			}
		}
		if outputsJSON != "" && outputsJSON != "[]" {
			if err := json.Unmarshal([]byte(outputsJSON), &e.OutputPorts); err != nil {
				return nil, fmt.Errorf("decoding output_ports for %s.%s: %w", e.Slug, e.Key, err)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListThemes returns themes contributed by installed plugins. The
// frontend resolves CSS paths through the plugin asset endpoint so a
// theme becomes selectable as soon as its plugin is installed and loaded.
func (s *Service) ListThemes(ctx context.Context) ([]themeEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, theme_id, name, description, css_path, swatch_hue,
		       default_variant, has_dark, has_light
		  FROM plugin_themes ORDER BY origin ASC, name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin themes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]themeEntry, 0, 32)
	for rows.Next() {
		var e themeEntry
		var hasDark, hasLight int
		if err := rows.Scan(&e.Slug, &e.ID, &e.Name, &e.Description, &e.CSSPath,
			&e.SwatchHue, &e.DefaultVariant, &hasDark, &hasLight); err != nil {
			return nil, fmt.Errorf("scanning plugin theme: %w", err)
		}
		e.HasDark = hasDark == 1
		e.HasLight = hasLight == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListTabs returns per-page tab descriptors contributed by every
// plugin. The frontend's usePageTabs(pageId) hook filters this list
// down to the page currently being viewed.
func (s *Service) ListTabs(ctx context.Context) ([]tabEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, page_id, tab_key, label_ns, label_key,
		       COALESCE(icon, ''), COALESCE(sort_order, 0),
		       COALESCE(requires_permission, '')
		  FROM plugin_tabs ORDER BY origin ASC, page_id ASC, sort_order ASC, tab_key ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin tabs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]tabEntry, 0, 32)
	for rows.Next() {
		var e tabEntry
		if err := rows.Scan(&e.Slug, &e.PageID, &e.Key, &e.LabelNS,
			&e.LabelKey, &e.Icon, &e.SortOrder, &e.Permission); err != nil {
			return nil, fmt.Errorf("scanning plugin tab: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListContextMenus returns per-page context-menu entries contributed
// by every plugin. The frontend's usePageContextMenus(pageId) hook
// filters this list down to the page currently being viewed.
func (s *Service) ListContextMenus(ctx context.Context) ([]contextMenuEntry, error) {
	// The plugin_context_menus table has no separate `key` column;
	// uniqueness is implicit via (origin, page_id, label_key) and the
	// row id is the de-facto key. Project id AS key so the JSON
	// payload matches the frontend's PluginContextMenuItem shape.
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, page_id, id, label_ns, label_key,
		       COALESCE(icon, ''), COALESCE(sort_order, 0),
		       COALESCE(requires_permission, '')
		  FROM plugin_context_menus ORDER BY origin ASC, page_id ASC, sort_order ASC, id ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin context menus: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]contextMenuEntry, 0, 32)
	for rows.Next() {
		var e contextMenuEntry
		if err := rows.Scan(&e.Slug, &e.PageID, &e.Key, &e.LabelNS,
			&e.LabelKey, &e.Icon, &e.SortOrder, &e.Permission); err != nil {
			return nil, fmt.Errorf("scanning plugin context menu: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListTopbar returns the topbar slot entries contributed by every
// plugin. The frontend's PluginTopbarBridge mounts one button per
// item into #header-slot or #header-right-slot.
func (s *Service) ListTopbar(ctx context.Context) ([]topbarEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, COALESCE(slot, 'left'), label_ns, label_key,
		       COALESCE(icon, ''), COALESCE(path, ''),
		       COALESCE(visibility, 'always')
		  FROM plugin_topbar ORDER BY origin ASC, label_key ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("loading plugin topbar: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin runtime: closing rows", "error", err)
		}
	}()
	out := make([]topbarEntry, 0, 16)
	for rows.Next() {
		var e topbarEntry
		if err := rows.Scan(&e.Slug, &e.Slot, &e.LabelNS,
			&e.LabelKey, &e.Icon, &e.Path, &e.Visibility); err != nil {
			return nil, fmt.Errorf("scanning plugin topbar row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Reload invokes the contribution rescan wired in by the caller.
// Returns ErrReloadUnsupported when no reload function was wired.
//
// When MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS is set to a truthy
// value, Reload additionally walks every installed plugin's
// migrations/ directory and applies any *.sql file whose name has
// not yet been recorded in plugin_migrations. This lets a plugin
// ship a schema fix and have it take effect on the next Reload
// without requiring the operator to uninstall + reinstall. Off by
// default for production safety (a misbehaving plugin migration
// can poison the host DB; opt-in only).
func (s *Service) Reload(ctx context.Context) error {
	if s.reloadFn == nil {
		return ErrReloadUnsupported
	}
	if err := s.reloadFn(); err != nil {
		return err
	}
	if shouldAutoApplyMigrations() {
		if err := s.applyPendingMigrations(ctx); err != nil {
			return fmt.Errorf("applying pending plugin migrations: %w", err)
		}
	}
	return nil
}

// shouldAutoApplyMigrations honors MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS
// (truthy values: 1, true, yes, on). The flag is intentionally opt-in.
func shouldAutoApplyMigrations() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MCHARBOR_AUTO_APPLY_PLUGIN_MIGRATIONS")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// applyPendingMigrations walks <dataDir>/plugins/<slug>@<version>/migrations
// for every installed plugin and applies *.sql files not already in the
// plugin_migrations tracking table. The default installer's
// runMigrationsTx helper is duplicated here as a private function so
// plugin_runtime does not need to import the installer package (it
// would create an import cycle through modules/appstore).
func (s *Service) applyPendingMigrations(ctx context.Context) error {
	if s.db == nil || s.dataDir == "" {
		return nil
	}
	root := filepath.Join(s.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug, _, ok := splitSlugVersion(e.Name())
		if !ok {
			continue
		}
		if err := s.applyPluginMigrations(ctx, root, e.Name(), slug); err != nil {
			return fmt.Errorf("plugin %s: %w", slug, err)
		}
	}
	return nil
}

// applyPluginMigrations applies any new *.sql under the plugin's
// migrations/ directory. We track applied migrations in
// plugin_migrations (origin, filename) so re-running is idempotent.
func (s *Service) applyPluginMigrations(ctx context.Context, root, pluginDir, slug string) error {
	migDir := filepath.Join(root, pluginDir, "migrations")
	entries, err := os.ReadDir(migDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// One applied-check per migration file, bounded by the plugin's own
	// migrations directory and run once at install or upgrade.
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		var seen int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM plugin_migrations WHERE origin = ? AND filename = ?`,
			slug, f.Name(),
		).Scan(&seen)
		if err != nil {
			return fmt.Errorf("checking plugin_migrations: %w", err)
		}
		if seen > 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migDir, f.Name()))
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Name(), err)
		}
		migCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		if err := runPluginMigrationTx(migCtx, s.db, string(b)); err != nil {
			cancel()
			return fmt.Errorf("applying %s: %w", f.Name(), err)
		}
		cancel()
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO plugin_migrations
				(id, origin, filename, applied_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), slug, f.Name(), now, now, now,
		); err != nil {
			return fmt.Errorf("recording %s: %w", f.Name(), err)
		}
	}
	return nil
}

// runPluginMigrationTx executes one SQL file inside a transaction.
// Duplicated from the installer's runMigrationTx to avoid an
// import cycle through modules/appstore.
func runPluginMigrationTx(ctx context.Context, db *sql.DB, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Warn("plugin runtime: rolling back a migration", "error", err)
		}
	}()
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return err
	}
	return tx.Commit()
}

// LoadI18n walks every installed plugin's i18n/<lang>/ directory
// and returns a flat namespace map keyed by slug then namespace.
//
// File layout per plugin: <dataDir>/plugins/<slug>@<version>/i18n/<lang>/<ns>.json
//
// EN fallback: when the requested lang is not "en" and a plugin
// ships a translation for a key in i18n/en but not in i18n/<lang>,
// the English value is used. The merge happens per-key so a partial
// translation is preserved (the keys the translator did ship win
// over the English fallback). This matches i18next's `fallbackLng:
// 'en'` behaviour on the client and avoids showing English copy
// next to translated copy for plugin-owned UI strings.
func (s *Service) LoadI18n(ctx context.Context, lang string) (*i18nResponse, error) {
	resp := &i18nResponse{Lang: lang, Namespaces: make(map[string]map[string]string)}
	if s.dataDir == "" {
		return resp, nil
	}
	root := filepath.Join(s.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return resp, nil
		}
		return nil, fmt.Errorf("reading plugin dir: %w", err)
	}
	fallback := strings.EqualFold(strings.TrimSpace(lang), "en")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug, _, ok := splitSlugVersion(e.Name())
		if !ok {
			continue
		}
		// Load requested-language bundle first.
		merged := make(map[string]string)
		if err := s.loadPluginLang(root, e.Name(), slug, lang, merged); err != nil {
			return nil, err
		}
		// Then layer English on top with fallback semantics: any
		// key already present in `merged` wins. Only the missing
		// keys are filled from en/ — see the per-key merge below.
		if !fallback {
			enMerged := make(map[string]string)
			if err := s.loadPluginLang(root, e.Name(), slug, "en", enMerged); err != nil {
				return nil, err
			}
			for k, v := range enMerged {
				if _, present := merged[k]; !present {
					merged[k] = v
				}
			}
		}
		if len(merged) == 0 {
			continue
		}
		// The slug is the default namespace — the convention every
		// bundled plugin's i18n bundle follows, and the one
		// menuRegistry.Add falls back to when a menu row names none.
		resp.Namespaces[slug] = merged
		// A plugin may also name its own namespace. addon-kubernetes
		// serves "kubernetes" and addon-reporting serves "reporting":
		// their menu rows' label_ns and every useTranslation() call
		// inside their bundles ask for that name, not for the slug.
		// Without this the whole plugin renders raw key paths.
		for _, ns := range s.declaredI18nNamespaces(root, e.Name()) {
			ns = strings.TrimSpace(ns)
			if ns == "" || ns == slug {
				continue
			}
			// addResourceBundle runs with overwrite=true on the
			// client, so a plugin claiming a host namespace would
			// silently replace the host's own strings.
			if reservedI18nNamespaces[ns] {
				continue
			}
			resp.Namespaces[ns] = merged
		}
	}
	return resp, nil
}

// reservedI18nNamespaces are the namespaces the host ships itself
// (staticNamespaces in core/i18n/i18n.ts, plus the two built at
// runtime from the widget and node catalogs). A plugin cannot claim
// one of these through contributes.i18n_namespaces.
var reservedI18nNamespaces = map[string]bool{
	"common": true, "auth": true, "containers": true, "images": true,
	"volumes": true, "networks": true, "stacks": true, "environments": true,
	"settings": true, "dashboard": true, "terminal": true, "security": true,
	"docker": true, "host": true, "gitops": true, "abac": true, "ai": true,
	"plugins": true, "enterprise": true, "scim": true,
	"nodes": true,
}

// declaredI18nNamespaces reads the plugin's manifest and returns the
// namespaces it declares under contributes.i18n_namespaces. Per-plugin
// read and parse failures are non-fatal — a broken manifest costs that
// plugin its named namespace, not the whole /plugins/loader/i18n
// response (same policy as ListForms).
func (s *Service) declaredI18nNamespaces(root, pluginDir string) []string {
	mb, err := os.ReadFile(filepath.Join(root, pluginDir, "plugin.yaml"))
	if err != nil {
		return nil
	}
	var m struct {
		Contribs struct {
			I18nNS []string `yaml:"i18n_namespaces,omitempty"`
		} `yaml:"contributes"`
	}
	if err := yaml.Unmarshal(mb, &m); err != nil {
		return nil
	}
	return m.Contribs.I18nNS
}

// loadPluginLang reads every JSON file under
// <root>/<pluginDir>/i18n/<lang>/ into the provided `into` map.
// Each file is the bundle for one i18next namespace (the filename
// minus .json). Plugin bundles are nested objects (e.g. {"menu":
// {"root": "..."}}), so the tree is flattened into "<namespace>.<dotted.path>"
// keys with string leaf values. Non-string leaves (arrays, objects)
// are skipped with a console log so a malformed translation file
// does not 500 the entire /plugins/loader/i18n response. Missing
// directories are silently skipped so the caller can layer the
// English fallback on top.
func (s *Service) loadPluginLang(root, pluginDir, slug, lang string, into map[string]string) error {
	nsDir := filepath.Join(root, pluginDir, "i18n", lang)
	files, err := os.ReadDir(nsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading i18n dir %s: %w", nsDir, err)
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		ns := strings.TrimSuffix(f.Name(), ".json")
		b, err := os.ReadFile(filepath.Join(nsDir, f.Name()))
		if err != nil {
			return fmt.Errorf("reading i18n file %s: %w", f.Name(), err)
		}
		// Strip UTF-8 BOM if a Windows-saved file slipped one in.
		// json.Unmarshal refuses on the BOM byte, but the rest of the
		// file is valid UTF-8 JSON.
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
		var raw map[string]interface{}
		if err := json.Unmarshal(b, &raw); err != nil {
			return fmt.Errorf("parsing i18n file %s: %w", f.Name(), err)
		}
		flattenInto(ns, raw, into)
	}
	return nil
}

// flattenInto walks a nested translation object and writes each leaf
// string into `into` keyed by "<dotted.path>". The namespace prefix
// is omitted because the caller already passes the namespace as the
// i18next namespace name (the plugin slug) when registering the
// bundle via addResourceBundle. Arrays and non-string scalars are
// skipped silently — the i18n loader only serves strings, and a
// stray object in a translation file should not 500 the whole API.
func flattenInto(ns string, node map[string]interface{}, into map[string]string) {
	for k, v := range node {
		switch vv := v.(type) {
		case string:
			into[k] = vv
		case map[string]interface{}:
			flattenChild(k, vv, into)
		}
	}
	_ = ns
}

// flattenChild recurses one level deeper, prefixing the dotted path.
func flattenChild(prefix string, node map[string]interface{}, into map[string]string) {
	for k, v := range node {
		switch vv := v.(type) {
		case string:
			into[prefix+"."+k] = vv
		case map[string]interface{}:
			flattenChild(prefix+"."+k, vv, into)
		}
	}
}

func (s *Service) soExists(slug, version string) bool {
	if s.dataDir == "" {
		return false
	}
	path := filepath.Join(s.dataDir, "plugins", slug+"@"+version, "backend", slug+".so")
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// BundleURL returns the relative URL the host's frontend should
// fetch to load a plugin's UMD bundle. Returns an empty string when
// no data dir is configured. The path is always inside the data
// dir; the host chi router serves it from `/api/plugins/loader/bundle`
// after looking up the on-disk file.
func (s *Service) BundleURL(slug string) string {
	if s.dataDir == "" || slug == "" {
		return ""
	}
	return filepath.Join(s.dataDir, "plugins", slug, "frontend", slug+".umd.cjs")
}

// BundleFile returns the absolute on-disk path of a plugin's UMD
// bundle, or "" if the slug is unsafe or no data dir is configured.
// Walks every installed version dir (`<slug>@<version>`) and returns
// the first matching file. Caller is responsible for guarding
// against path traversal — slug must already match
// `^[a-z][a-z0-9-]{1,40}$`.
func (s *Service) BundleFile(slug string) string {
	if s.dataDir == "" || slug == "" {
		return ""
	}
	root := filepath.Join(s.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirSlug, _, ok := splitSlugVersion(e.Name())
		if !ok || dirSlug != slug {
			continue
		}
		candidate := filepath.Join(root, e.Name(), "frontend", slug+".umd.cjs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// IconFile returns the absolute on-disk path of a plugin's icon
// asset, or "" if the icon is missing or the inputs are unsafe.
//
//	slug — plugin slug (must already match
//	       ^[a-z][a-z0-9-]{1,40}$, the caller checks).
//	name — filename relative to the plugin's icons/ dir (e.g.
//	       "plugin.svg", "identity-google.svg"). The caller is
//	       responsible for stripping path separators; this method
//	       also re-checks via filepath.Base + stat.
//
// Lookup order:
//
//  1. Installed plugin's icons/ dir: <dataDir>/plugins/<slug>@<version>/icons/<name>
//  2. Catalog per-slug icons dir: <catalogDir>/icons/<slug>/<name> —
//     when the catalog organizes icons under per-slug subdirectories.
//  3. Catalog flat icons dir: <catalogDir>/icons/<name> — when the
//     catalog drops all icons directly into icons/ (current layout).
//  4. Catalog root fallback: <catalogDir>/<name> — legacy.
//  4. Catalog root fallback: <catalogDir>/<name> — icons dropped
//     directly in the catalog root (legacy).
func (s *Service) IconFile(slug, name string) string {
	return s.IconFileVariant(slug, name, "")
}

// DarkIconName returns the dark-mode filename for an icon asset —
// "plugin.svg" becomes "plugin_dark.svg". A name with no extension
// gets the suffix appended. Returns "" when there is nothing to
// derive from.
func DarkIconName(name string) string {
	if name == "" {
		return ""
	}
	ext := filepath.Ext(name)
	if ext == "" {
		return name + "_dark"
	}
	return strings.TrimSuffix(name, ext) + "_dark" + ext
}

// IconFileVariant resolves an icon the way IconFile does, but prefers
// the theme variant when one is asked for. `variant` is "dark" (the
// only variant today) or "" for the default asset.
//
// A plugin that ships no `<base>_dark.<ext>` falls back to the plain
// asset, so the dark variant is opt-in per plugin and a catalog that
// carries none behaves exactly as before.
func (s *Service) IconFileVariant(slug, name, variant string) string {
	if variant == "dark" {
		if dark := DarkIconName(name); dark != "" {
			if path := s.iconFile(slug, dark); path != "" {
				return path
			}
		}
	}
	return s.iconFile(slug, name)
}

func (s *Service) iconFile(slug, name string) string {
	if slug == "" || name == "" {
		return ""
	}
	// Defense in depth: the handler already validates `name`, but
	// rejecting path-traversal attempts here too means a future
	// caller that forgets the check cannot reach outside icons/.
	clean := filepath.Base(name)
	if clean != name || clean == "." || clean == ".." {
		return ""
	}

	// 1. Installed plugin's icons/ dir.
	if s.dataDir != "" {
		root := filepath.Join(s.dataDir, "plugins")
		entries, err := os.ReadDir(root)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dirSlug, _, ok := splitSlugVersion(e.Name())
				if !ok || dirSlug != slug {
					continue
				}
				candidate := filepath.Join(root, e.Name(), "icons", clean)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}

	// 2 + 3. Catalog fallback (per-slug-icons subdir, then icons/<name>
	// flat, then catalog root flat) across every configured catalog
	// dir. Each entry is scanned in order: the first directory
	// containing a matching icon wins. Lets us host a single runtime
	// with multiple catalog mounts (dev catalog + future production
	// catalogs) without baking any one of them into the image.
	for _, catalogDir := range s.catalogDirs {
		catalogDir = strings.TrimSpace(catalogDir)
		if catalogDir == "" {
			continue
		}
		if path := filepath.Join(catalogDir, "icons", slug, clean); path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		if path := filepath.Join(catalogDir, "icons", clean); path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		if path := filepath.Join(catalogDir, clean); path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	return ""
}

// AssetFile returns a plugin-relative asset path from the installed
// plugin directory. The handler only passes sanitized CSS paths.
func (s *Service) AssetFile(slug, rel string) string {
	if s.dataDir == "" || slug == "" || rel == "" {
		return ""
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return ""
	}
	root := filepath.Join(s.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirSlug, _, ok := splitSlugVersion(e.Name())
		if !ok || dirSlug != slug {
			continue
		}
		candidate := filepath.Join(root, e.Name(), clean)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// ListForms reads every installed plugin's plugin.yaml at request
// time and flattens the contributes.forms entries. The mirror logic
// that populates contribution SQL tables doesn't track forms yet
// (they post-date the mirror) so this is filesystem-direct. Once
// forms land in the SQL mirror the filesystem path can be replaced
// with a SELECT from plugin_forms.
func (s *Service) ListForms(ctx context.Context) ([]formEntry, error) {
	out := make([]formEntry, 0, 32)
	if s.dataDir == "" {
		return out, nil
	}
	root := filepath.Join(s.dataDir, "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("reading plugin dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug, _, ok := splitSlugVersion(e.Name())
		if !ok {
			continue
		}
		mb, err := os.ReadFile(filepath.Join(root, e.Name(), "plugin.yaml"))
		if err != nil {
			// Per-plugin read failures are non-fatal — skip silently
			// so a single broken manifest doesn't black out the
			// whole form catalog.
			continue
		}
		var m struct {
			Contribs struct {
				Forms []struct {
					Resource string `yaml:"resource"`
					Kind     string `yaml:"kind"`
					Key      string `yaml:"key,omitempty"`
				} `yaml:"forms,omitempty"`
			} `yaml:"contributes"`
		}
		if err := yaml.Unmarshal(mb, &m); err != nil {
			continue
		}
		for _, f := range m.Contribs.Forms {
			out = append(out, formEntry{
				Slug:     slug,
				Resource: f.Resource,
				Kind:     f.Kind,
				Key:      f.Key,
			})
		}
	}
	return out, nil
}
