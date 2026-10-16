// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// ErrNotFound is returned when a catalog row does not exist.
var ErrNotFound = errors.New("catalog not found")

// ErrReadOnly is returned when a write targets a read-only (built-in) row.
var ErrReadOnly = errors.New("catalog is read-only")

// ErrInvalidKind is returned when an unknown kind is requested.
var ErrInvalidKind = errors.New("invalid catalog kind")

// ErrInvalidInput is returned when required fields are missing.
var ErrInvalidInput = errors.New("invalid catalog input")

// ErrDuplicateURL is returned, alongside the existing row, when the kind and URL are already
// configured. Answering success instead told the operator a catalog was added when none was.
var ErrDuplicateURL = errors.New("catalog url already configured")

// Service owns the SQL for the settings_catalogs table. Handlers
// stay thin and delegate every read/write to this type.
type Service struct {
	db                *sql.DB
	pluginCatalogRoot string
	themeCatalogRoot  string
}

// NewService returns a Service that reads and writes catalog sources through db.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// SetCatalogRoots points the plugin and theme catalog probes at this install's
// layout. It affects SeedLocalCatalogs only; empty values keep the container paths.
func (s *Service) SetCatalogRoots(pluginRoot, themeRoot string) {
	s.pluginCatalogRoot = pluginRoot
	s.themeCatalogRoot = themeRoot
}

// List returns every catalog grouped by kind. Read-only rows come
// first within each kind so the admin sees "official" catalogs on
// top of the list.
func (s *Service) List(ctx context.Context) ([]Catalog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, url, kind, enabled, readonly, origin, created_at, updated_at
		FROM settings_catalogs
		ORDER BY kind ASC, readonly DESC, name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("listing catalogs: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	var out []Catalog
	for rows.Next() {
		c, err := scanCatalog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one catalog by id.
func (s *Service) Get(ctx context.Context, id string) (Catalog, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, url, kind, enabled, readonly, origin, created_at, updated_at
		FROM settings_catalogs WHERE id = ?
	`, id)
	c, err := scanCatalog(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Catalog{}, ErrNotFound
	}
	return c, err
}

// Create inserts a new catalog. Returns ErrInvalidInput / ErrInvalidKind on bad input, and
// the existing row with ErrDuplicateURL when (kind, url) is already configured.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Catalog, error) {
	name := strings.TrimSpace(req.Name)
	url := strings.TrimSpace(req.URL)
	if name == "" || url == "" {
		return Catalog{}, fmt.Errorf("%w: name and url are required", ErrInvalidInput)
	}
	if !ValidKind(req.Kind) {
		return Catalog{}, fmt.Errorf("%w: %q", ErrInvalidKind, req.Kind)
	}

	now := time.Now().UTC()
	c := Catalog{
		ID:        xid.New().String(),
		Name:      name,
		URL:       url,
		Kind:      Kind(req.Kind),
		Enabled:   true,
		ReadOnly:  false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings_catalogs
			(id, name, url, kind, enabled, readonly, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Name, c.URL, string(c.Kind), 1, 0, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			existing, getErr := s.findByKindURL(ctx, c.Kind, c.URL)
			if getErr != nil {
				return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
			}
			return existing, ErrDuplicateURL
		}
		return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
	}
	return c, nil
}

// Update applies a partial update. Read-only rows permit enable/disable
// changes, but their names and URLs remain immutable.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Catalog, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Catalog{}, err
	}
	if existing.ReadOnly && (req.Name != nil || req.URL != nil) {
		return Catalog{}, ErrReadOnly
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.URL != nil {
		existing.URL = strings.TrimSpace(*req.URL)
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if existing.Name == "" || existing.URL == "" {
		return Catalog{}, fmt.Errorf("%w: name and url cannot be empty", ErrInvalidInput)
	}
	existing.UpdatedAt = time.Now().UTC()

	_, err = s.db.ExecContext(ctx, `
		UPDATE settings_catalogs
		SET name = ?, url = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, existing.Name, existing.URL, boolToInt(existing.Enabled), existing.UpdatedAt, id)
	if err != nil {
		return Catalog{}, fmt.Errorf("updating catalog: %w", err)
	}
	return existing, nil
}

// Delete removes a row by id. Read-only rows are rejected.
func (s *Service) Delete(ctx context.Context, id string) error {
	c, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if c.ReadOnly {
		return ErrReadOnly
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM settings_catalogs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting catalog: %w", err)
	}
	return nil
}

// SeedDefaults is idempotent: it canonicalizes the official built-in rows by
// stable ID, preserving their enabled state, then registers mounted local
// catalogs when present.
// Called from main.go at startup.
func (s *Service) SeedDefaults(ctx context.Context) error {
	now := time.Now().UTC()
	// bounded by the built-in catalog list; the upsert is idempotent and repeats on the next boot
	for _, c := range builtinCatalogs() {
		// A hand-added row can already sit on this (kind, url) — an
		// operator who pointed a catalog at the same document the
		// built-in names, which is exactly what someone does after the
		// built-in URL turns out to be wrong. UNIQUE(kind, url) then
		// makes the built-in's URL repair below fail, and because the
		// error is tolerated the row stays stuck on the stale URL for
		// every boot after. Absorb the duplicate first: it is the same
		// source, so its enabled flag carries over and the copy goes.
		adopted, err := s.absorbBuiltinDuplicate(ctx, c.ID, c.Kind, c.URL)
		if err != nil {
			return err
		}

		// Seeded disabled — deliberately, and not because the upstream is
		// missing: both rows resolve today (plugins/index.json and
		// apps/index.json are live). They stay off because these
		// sources install executable code, and turning that on is an
		// operator's decision rather than something a first boot should
		// make for them.
		//
		// MAX(existing, excluded) is what keeps that decision: the seed
		// value is 0 on an ordinary boot, so an operator who switched a
		// row on keeps it on, and it is 1 only when the duplicate this
		// seed just absorbed was enabled — losing that flag would take a
		// working source away without saying so.
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO settings_catalogs
				(id, name, url, kind, enabled, readonly, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				url = excluded.url,
				kind = excluded.kind,
				enabled = MAX(settings_catalogs.enabled, excluded.enabled),
				readonly = 1,
				updated_at = excluded.updated_at
		`, c.ID, c.Name, c.URL, string(c.Kind), boolToInt(adopted), now, now)
		if err != nil {
			if !strings.Contains(err.Error(), "UNIQUE") {
				return fmt.Errorf("seeding default catalog %s/%s: %w", c.Kind, c.URL, err)
			}
			// Tolerated so a boot is never blocked, but never again
			// silent: this is the failure that left builtin-docker-apps
			// pointing at the old index long after the constant moved.
			slog.Warn("catalogs: built-in catalog not canonicalized, another row holds this URL",
				"id", c.ID, "kind", string(c.Kind), "url", c.URL, "error", err)
		}
	}
	if err := s.SeedLocalCatalogs(ctx); err != nil {
		return err
	}
	return s.SeedPluginCatalogs(ctx)
}

// SeedPluginCatalogs projects plugin_catalogs into settings_catalogs as
// read-only rows and removes the contributed rows whose declaration is gone,
// so a category tracks its plugin's install state without a restart.
//
// Only rows carrying a non-empty origin are ever reaped: a host built-in and
// anything an operator added both carry the empty string, and neither is this
// function's to delete.
func (s *Service) SeedPluginCatalogs(ctx context.Context) error {
	decls, err := s.pluginCatalogDecls(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	live := make(map[string]bool, len(decls))
	// bounded by the catalogs the installed plugins declare; the upsert is idempotent and repeats on the next boot
	for _, d := range decls {
		if !ValidKind(string(d.Kind)) {
			// Rejected rather than stored: the manifest schema already
			// refuses an unknown kind, so reaching here means the row
			// predates a kind the host has since dropped, and a category
			// nothing can label is worse than a missing one.
			slog.Warn("catalogs: ignoring plugin catalog of unknown kind",
				"origin", d.Origin, "kind", string(d.Kind), "url", d.URL)
			continue
		}
		live[d.ID] = true

		// The same duplicate-absorb the built-ins need, for the same reason:
		// an operator who added this URL by hand before installing the plugin
		// would otherwise hold UNIQUE(kind, url) and wedge the projection on
		// every boot. Their enabled flag carries over.
		adopted, err := s.absorbBuiltinDuplicate(ctx, d.ID, d.Kind, d.URL)
		if err != nil {
			return err
		}

		// Seeded disabled for the same reason the built-ins are: a guest
		// image is executable content, so switching the source on is an
		// operator's decision. MAX() keeps that decision across boots.
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO settings_catalogs
				(id, name, url, kind, enabled, readonly, origin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				url = excluded.url,
				kind = excluded.kind,
				enabled = MAX(settings_catalogs.enabled, excluded.enabled),
				readonly = 1,
				origin = excluded.origin,
				updated_at = excluded.updated_at
		`, d.ID, d.Name, d.URL, string(d.Kind), boolToInt(adopted), d.Origin, now, now)
		if err != nil {
			if !strings.Contains(err.Error(), "UNIQUE") {
				return fmt.Errorf("seeding plugin catalog %s/%s: %w", d.Origin, d.URL, err)
			}
			slog.Warn("catalogs: plugin catalog not canonicalized, another row holds this URL",
				"id", d.ID, "origin", d.Origin, "kind", string(d.Kind), "url", d.URL, "error", err)
		}
	}

	return s.reapPluginCatalogs(ctx, live)
}

// PurgeKinds removes every catalog row of the given kinds, for the uninstall
// option that clears a plugin's remaining database entries. Returns how many
// rows went.
//
// A kind is skipped unless it is genuinely orphaned: no plugin_catalogs row
// still declares it, and no read-only host built-in uses it. That second test
// is the safety rail — nothing stops a plugin manifest from naming
// "docker_app", and without it such a plugin would take the App Store's
// catalog sources with it. docker_app and plugin both carry built-ins and so
// can never be purged; vm_image, once addon-compute is gone, carries none.
func (s *Service) PurgeKinds(ctx context.Context, kinds []string) (int, error) {
	purged := 0
	// bounded by the kinds of one uninstall; each purge is independent
	for _, kind := range kinds {
		orphaned, err := s.kindIsOrphaned(ctx, kind)
		if err != nil {
			return purged, err
		}
		if !orphaned {
			slog.Info("catalogs: kind still in use, not purging", "kind", kind)
			continue
		}
		res, err := s.db.ExecContext(ctx, `DELETE FROM settings_catalogs WHERE kind = ?`, kind)
		if err != nil {
			return purged, fmt.Errorf("purging %s catalogs: %w", kind, err)
		}
		affected := db.RowsAffected(res)
		if affected > 0 {
			slog.Info("catalogs: purged catalogs of an uninstalled plugin's kind",
				"kind", kind, "rows", affected)
		}
		purged += int(affected)
	}
	return purged, nil
}

// kindIsOrphaned reports whether no plugin still declares kind and no host
// built-in row uses it.
func (s *Service) kindIsOrphaned(ctx context.Context, kind string) (bool, error) {
	var declared int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plugin_catalogs WHERE kind = ?`, kind).Scan(&declared); err != nil {
		return false, fmt.Errorf("counting declarations of %s: %w", kind, err)
	}
	if declared > 0 {
		return false, nil
	}
	var builtins int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM settings_catalogs WHERE kind = ? AND readonly = 1 AND origin = ''`,
		kind).Scan(&builtins); err != nil {
		return false, fmt.Errorf("counting built-ins of %s: %w", kind, err)
	}
	return builtins == 0, nil
}

// pluginCatalogDecl is one row of plugin_catalogs, already carrying the
// settings_catalogs id the projection will write it under.
type pluginCatalogDecl struct {
	ID     string
	Origin string
	Kind   Kind
	Name   string
	URL    string
}

func (s *Service) pluginCatalogDecls(ctx context.Context) ([]pluginCatalogDecl, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT origin, catalog_id, kind, name, url
		FROM plugin_catalogs
		ORDER BY origin ASC, catalog_id ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("listing plugin catalogs: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	out := make([]pluginCatalogDecl, 0, 8)
	for rows.Next() {
		var d pluginCatalogDecl
		var catalogID, kind string
		if err := rows.Scan(&d.Origin, &catalogID, &kind, &d.Name, &d.URL); err != nil {
			return nil, fmt.Errorf("scanning plugin catalog: %w", err)
		}
		d.Kind = Kind(kind)
		d.ID = pluginCatalogRowID(d.Origin, catalogID)
		out = append(out, d)
	}
	return out, rows.Err()
}

// pluginCatalogRowID is the stable settings_catalogs id a contributed row
// lands on, namespaced by origin so two plugins may both declare "images".
func pluginCatalogRowID(origin, catalogID string) string {
	return "plugin-" + origin + "-" + catalogID
}

// reapPluginCatalogs deletes the contributed rows no manifest declares any
// more. live holds the ids this seed just wrote.
func (s *Service) reapPluginCatalogs(ctx context.Context, live map[string]bool) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, origin, url FROM settings_catalogs
		WHERE origin <> ''
		LIMIT 1000
	`)
	if err != nil {
		return fmt.Errorf("listing contributed catalogs: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	type stale struct{ id, origin, url string }
	var doomed []stale
	for rows.Next() {
		var row stale
		if err := rows.Scan(&row.id, &row.origin, &row.url); err != nil {
			return fmt.Errorf("scanning contributed catalog: %w", err)
		}
		if !live[row.id] {
			doomed = append(doomed, row)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating contributed catalogs: %w", err)
	}

	// bounded by the contributed catalogs of uninstalled plugins; each delete is independent
	for _, row := range doomed {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM settings_catalogs WHERE id = ?`, row.id); err != nil {
			return fmt.Errorf("reaping contributed catalog %s: %w", row.id, err)
		}
		slog.Info("catalogs: removed catalog of an uninstalled plugin",
			"id", row.id, "origin", row.origin, "url", row.url)
	}
	return nil
}

// absorbBuiltinDuplicate removes the operator-added row that already
// occupies (kind, url) for a built-in or plugin-contributed catalog,
// reporting whether that row was enabled so the caller can carry the
// flag onto the canonical row.
//
// Read-only rows are left alone: two built-ins claiming one URL is a
// bug in builtinCatalogs, and deleting one would hide it behind a
// self-healing boot.
func (s *Service) absorbBuiltinDuplicate(ctx context.Context, builtinID string, kind Kind, url string) (bool, error) {
	var id string
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, enabled FROM settings_catalogs
		WHERE kind = ? AND url = ? AND id <> ? AND readonly = 0
		LIMIT 1
	`, string(kind), url, builtinID).Scan(&id, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("finding duplicate of built-in catalog %s: %w", builtinID, err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM settings_catalogs WHERE id = ?`, id); err != nil {
		return false, fmt.Errorf("removing duplicate catalog %s: %w", id, err)
	}
	slog.Info("catalogs: absorbed hand-added duplicate of a built-in catalog",
		"removed_id", id, "builtin_id", builtinID, "kind", string(kind), "url", url, "was_enabled", enabled == 1)
	return enabled == 1, nil
}

// localCatalogEntry is one auto-registered catalog source.
type localCatalogEntry struct {
	Path string
	Name string
	Kind Kind
}

// localCatalogPaths lists the catalog sources probed at boot. Plugins now come
// from the remote built-in catalog, so a local plugin root is registered only
// when an operator mounts one; the /mnt entries are development bind mounts.
func localCatalogPaths(pluginRoot, themeRoot string) []localCatalogEntry {
	if themeRoot == "" {
		themeRoot = "/mnt/themes-catalog"
	}
	entries := []localCatalogEntry{
		{Path: "/mnt/app-catalog", Name: "Local app catalog", Kind: KindDockerApp},
		{Path: themeRoot, Name: "McHarbor themes", Kind: KindTheme},
		{Path: "/mnt/plugins", Name: "Local dev plugins", Kind: KindPlugin},
		{Path: "/mnt/plugin-catalog", Name: "Local dev catalog", Kind: KindPlugin},
	}
	if pluginRoot != "" {
		entries = append(entries,
			localCatalogEntry{Path: pluginRoot, Name: "Local plugin catalog", Kind: KindPlugin})
	}
	return entries
}

// SeedLocalCatalogs auto-registers each mounted source that exists and
// contains at least one JSON file. The plugin paths retain the development
// convenience described in
// .references/backup/2026-08-14-0315-dev-mnt-plugins-catalog/
// CHANGES.md and .references/backup/2026-08-14-1100-plugin-catalog-dev-mount/
// CHANGES.md — dropping JSON files into ./mnt/plugins or
// ./mnt/plugin-catalog on the host makes them appear in the Plugin
// Store on next boot without manual POST /api/catalogs.
//
// Idempotent: the Create method is already a no-op on UNIQUE(kind, url)
// so re-runs of SeedDefaults do not produce duplicates.
func (s *Service) SeedLocalCatalogs(ctx context.Context) error {
	for _, entry := range localCatalogPaths(s.pluginCatalogRoot, s.themeCatalogRoot) {
		// Errors are logged inside the helper; never bubble up
		// because the official catalog is already seeded by the
		// caller and a loud failure here would block boot on a
		// misconfigured host.
		s.seedLocalCatalogAtPath(ctx, entry.Path, entry.Name, entry.Kind)
	}
	return nil
}

// seedLocalCatalogAtPath probes one mounted path. Missing / wrong-type /
// empty-dir are silent no-ops. Permission or transient FS errors are
// logged at warn. The actual insert is delegated to s.Create so the
// UNIQUE(kind, url) idempotency contract is preserved.
func (s *Service) seedLocalCatalogAtPath(ctx context.Context, path, displayName string, kind Kind) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		slog.Warn("local catalog probe failed", "path", path, "kind", kind, "err", err)
		return
	}
	if !info.IsDir() {
		return
	}

	files, err := os.ReadDir(path)
	if err != nil {
		slog.Warn("local catalog readdir failed", "path", path, "kind", kind, "err", err)
		return
	}
	hasJSON := false
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(f.Name()), ".json") {
			hasJSON = true
			break
		}
	}
	if !hasJSON {
		return
	}

	created, err := s.Create(ctx, CreateRequest{
		Name: displayName,
		URL:  path,
		Kind: string(kind),
	})
	if errors.Is(err, ErrDuplicateURL) {
		return // registered on an earlier boot
	}
	if err != nil {
		slog.Warn("local catalog seed failed", "path", path, "kind", kind, "err", err)
		return
	}
	if created.ID != "" {
		slog.Info("local catalog seeded",
			"name", created.Name, "url", created.URL, "id", created.ID)
	}
}

func (s *Service) findByKindURL(ctx context.Context, kind Kind, url string) (Catalog, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, url, kind, enabled, readonly, origin, created_at, updated_at
		FROM settings_catalogs WHERE kind = ? AND url = ?
	`, string(kind), url)
	c, err := scanCatalog(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Catalog{}, ErrNotFound
	}
	return c, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCatalog(row scanner) (Catalog, error) {
	var c Catalog
	var enabled, readonly int
	var createdAt, updatedAt string
	var kind string
	if err := row.Scan(&c.ID, &c.Name, &c.URL, &kind, &enabled, &readonly, &c.Origin, &createdAt, &updatedAt); err != nil {
		return Catalog{}, err
	}
	c.Kind = Kind(kind)
	c.Enabled = enabled != 0
	c.ReadOnly = readonly != 0
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		c.CreatedAt = t
	} else {
		c.CreatedAt = time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		c.UpdatedAt = t
	} else {
		c.UpdatedAt = c.CreatedAt
	}
	return c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// builtinAppCatalogURL is the official app catalog. It names the index document
// itself rather than a directory, because the host serves no directory listing
// (a bare /plugins/ answers 403) and because catalogindex.Resolve only passes a
// non-GitHub URL through when it already names a .json file — a directory URL
// here would resolve to nothing fetchable for both the loader and the connection probe.
//
// Moved from /addons/apps to /catalog/apps, beside the plugin and agent catalogs.
// An existing install repairs itself: SeedDefaults rewrites a built-in row's URL
// by stable ID on every boot, and absorbBuiltinDuplicate clears a hand-added row
// already sitting on the new URL first, so the UNIQUE(kind, url) repair cannot wedge.
const builtinAppCatalogURL = "https://www.mcharbor.eu/catalog/apps/index.json"

// builtinPluginCatalogURL is the only place plugins come from now that the
// image ships none. SeedDefaults rewrites the built-in row by stable ID on
// every boot, so an install that predates the move repairs itself.
const builtinPluginCatalogURL = "https://www.mcharbor.eu/catalog/plugins/index.json"

// builtinCatalogs seeds the two official McHarbor-AddOns catalogs. Their
// stable IDs let SeedDefaults repair names, kinds, and URLs while preserving
// each row's enabled state.
//
// The widget and node catalogs that used to sit here are gone: widgets and
// nodes ship as plugin bundles, so they come from the plugin catalog.
// Migration 090 removes the rows they left behind.
func builtinCatalogs() []Catalog {
	return []Catalog{
		{ID: "builtin-docker-apps", Name: "McHarbor Apps", URL: builtinAppCatalogURL, Kind: KindDockerApp},
		{ID: "builtin-plugins", Name: "McHarbor Plugins", URL: builtinPluginCatalogURL, Kind: KindPlugin},
	}
}
