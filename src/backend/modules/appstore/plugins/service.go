// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

// Service is the DB layer for the appstore plugins surface. All
// queries and mutations live here; the HTTP handlers in handler.go
// call into this type. Returns *sql.ErrNoRows when a single row is
// expected but missing — handlers translate that to 404.
type Service struct {
	db         *sql.DB
	resolver   CatalogResolver
	catalogDir string
	// runtimeLoadedFn, when non-nil, returns slug → loaded (true if
	// the runtime has the .so resident in memory). ListInstalled uses
	// it to populate InstalledPlugin.Loaded so the admin UI's "Not
	// loaded" badge reflects the runtime, not just the DB row.
	runtimeLoadedFn func() map[string]bool
	icons           remoteIcons
}

// NewService constructs a Service backed by db, an optional catalog
// resolver, and a legacy local catalog directory.
//
//   - When resolver is non-nil, ListCatalog asks it for enabled
//     plugin sources (typically the settings_catalogs registry). This
//     is the production path.
//   - When resolver is nil and catalogDir is non-empty, ListCatalog
//     falls back to the single-directory reader — used by
//     cmd/testmirror and by early-stage code that hasn't wired the
//     resolver yet.
//   - When both are empty, ListCatalog returns []PluginEntry{}
//     (UI renders the unconfigured state).
func NewService(db *sql.DB, resolver CatalogResolver, catalogDir string) *Service {
	return &Service{db: db, resolver: resolver, catalogDir: catalogDir}
}

// SetRuntimeLoaded wires a callback that reports the runtime's view
// of which plugin .so files are currently dlopen'd. The runtime
// lives in core/pluginruntime and uses a sync.RWMutex on the
// in-memory map, so passing a getter avoids pulling the runtime into
// the plugins package. Called from the host after pluginruntime.Boot
// returns.
func (s *Service) SetRuntimeLoaded(fn func() map[string]bool) {
	s.runtimeLoadedFn = fn
}

// --- catalog (read-only) ----------------------------------------

// ListCatalog reads every enabled plugin catalog source and returns
// a merged, slug-sorted slice. Sources come from the resolver when
// one is wired; the legacy single-directory path is used as a
// fallback when the resolver is nil. Per-source errors are logged
// at warn level and skipped — one broken catalog must not blank out
// the whole list.
func (s *Service) ListCatalog(ctx context.Context) ([]PluginEntry, error) {
	sources, err := s.resolveSources(ctx)
	if err != nil {
		return nil, err
	}

	bySlug := make(map[string]*PluginEntry)
	order := make([]string, 0, len(sources)*4)
	for _, src := range sources {
		entries, err := s.readSource(ctx, src)
		if err != nil {
			slog.Warn("plugin catalog: source read failed",
				"url", src.URL, "name", src.Name, "error", err)
			continue
		}
		for i := range entries {
			slug := entries[i].Slug
			if slug == "" {
				continue
			}
			existing, ok := bySlug[slug]
			if !ok {
				e := entries[i]
				bySlug[slug] = &e
				order = append(order, slug)
				continue
			}
			// Combine version lists and dedupe by version string;
			// later sources lose to earlier sources on duplicate
			// metadata fields.
			combined := make([]PluginVersion, 0, len(existing.Versions)+len(entries[i].Versions))
			incoming := make(map[string]struct{}, len(entries[i].Versions))
			for _, v := range entries[i].Versions {
				incoming[v.Version] = struct{}{}
				combined = append(combined, v)
			}
			for _, ev := range existing.Versions {
				if _, dup := incoming[ev.Version]; dup {
					continue
				}
				combined = append(combined, ev)
			}
			existing.Versions = dedupeVersions(combined)
			sortVersionsDesc(existing.Versions)
			if len(existing.Versions) > 0 {
				top := existing.Versions[0]
				existing.Version = top.Version
				existing.Build = top.Build
				existing.DownloadURL = top.DownloadURL
				existing.Checksum = top.Checksum
				existing.Signature = top.Signature
				existing.BuiltFor = top.BuiltFor
			}
		}
	}

	out := make([]PluginEntry, 0, len(order))
	for _, slug := range order {
		p := bySlug[slug]
		if p.Name == "" {
			p.Name = slug
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// resolveSources returns the catalog sources ListCatalog should
// read. Order: resolver first; legacy single-directory fallback
// only when the resolver is nil.
func (s *Service) resolveSources(ctx context.Context) ([]PluginSource, error) {
	if s.resolver != nil {
		pluginSources, err := s.resolver.EnabledPluginSources(ctx, "plugin")
		if err != nil {
			return nil, err
		}
		themeSources, err := s.resolver.EnabledPluginSources(ctx, "theme")
		if err != nil {
			return nil, err
		}
		return append(pluginSources, themeSources...), nil
	}
	if strings.TrimSpace(s.catalogDir) != "" {
		return []PluginSource{{Name: "legacy", URL: s.catalogDir}}, nil
	}
	return nil, nil
}

// ListInstalled returns every row in appstore_plugins ordered by
// install time (newest first). The Loaded field is filled from the
// runtime if SetRuntimeLoaded was called; otherwise every row
// reports Loaded=false (legacy path — the .so is on disk but the
// runtime didn't surface its view to this layer).
func (s *Service) ListInstalled(ctx context.Context) ([]InstalledPlugin, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, slug, version, COALESCE(build, ''), enabled, signed,
		       installed_at, updated_at, built_for, repair_error
		  FROM appstore_plugins
		 ORDER BY installed_at DESC, slug ASC
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing installed plugin rows failed", "error", err)
		}
	}()

	out := make([]InstalledPlugin, 0, 32)
	for rows.Next() {
		var p InstalledPlugin
		var enabled int
		var signed int
		if err := rows.Scan(&p.ID, &p.Slug, &p.Version, &p.Build, &enabled, &signed,
			&p.InstalledAt, &p.UpdatedAt, &p.BuiltFor, &p.RepairError); err != nil {
			return nil, fmt.Errorf("scanning plugin row: %w", err)
		}
		p.Enabled = enabled == 1
		p.Signed = signed == 1
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Stamp Loaded from the runtime view once at the end so each
	// list response reflects the current in-memory state, not a stale
	// per-row snapshot from the moment the SQL returned.
	if s.runtimeLoadedFn != nil {
		loaded := s.runtimeLoadedFn()
		for i := range out {
			out[i].Loaded = loaded[out[i].Slug]
		}
	}
	// Name the roles that grant each plugin's keys so the uninstall
	// confirm dialog can warn that those grants will be stripped.
	// Bounded work: a handful of plugins times a handful of roles.
	for i := range out {
		names, err := s.rolesGrantingPlugin(ctx, out[i].Slug)
		if err != nil {
			return nil, err
		}
		out[i].RolesReferencing = names
		out[i].PermissionsReferenced = len(names)
	}
	return out, nil
}

// rolesGrantingPlugin returns the sorted, de-duplicated names of every
// role that grants at least one of the plugin's permission keys. An
// empty (non-nil) slice when nothing does.
func (s *Service) rolesGrantingPlugin(ctx context.Context, slug string) ([]string, error) {
	keys, err := s.PermissionsForPlugin(ctx, slug)
	if err != nil {
		return nil, err
	}
	refs, err := s.RolesReferencingPermissions(ctx, keys)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, roles := range refs {
		for _, name := range roles {
			set[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// --- install/uninstall --------------------------

// InsertInstalledRow creates the appstore_plugins row with enabled=1
// so the plugin is immediately active on next boot or runtime
// rescan. The legacy flow left enabled=0 and required an explicit
// Enable call, but the runtime loader ignores the flag (it loads
// every row in appstore_plugins on boot — see
// core/pluginruntime/loader.go:loadInstalledSlugs). With the flag
// no longer gating anything, defaulting to enabled matches what the
// loader actually does and removes the "installed but inert" trap.
//
// signed records whether the tarball carried a valid signature from a
// configured trust key. Unsigned installs are allowed; the flag is how
// every later view (API, plugin card, audit trail) can still say which
// is which instead of presenting both as equally vouched for.
func (s *Service) InsertInstalledRow(ctx context.Context, slug, version, name, author, description string, signed bool) (string, error) {
	return s.InsertInstalledRowWithBuild(ctx, slug, version, "", name, author, description, signed)
}

// InsertInstalledRowWithBuild is InsertInstalledRow plus the artifact's compile
// stamp. The two exist separately because a repair re-inserts a row for an
// artifact it did not download and so has no build to record.
func (s *Service) InsertInstalledRowWithBuild(ctx context.Context, slug, version, build, name, author, description string, signed bool) (string, error) {
	id := uuid.NewString()
	signedFlag := 0
	if signed {
		signedFlag = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO appstore_plugins (id, slug, name, version, build, author, description, source, enabled, signed)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'plugin', 1, ?)`,
		id, slug, name, version, nullIfBlank(build), author, description, signedFlag,
	)
	if err != nil {
		return "", fmt.Errorf("inserting appstore_plugins row: %w", err)
	}
	return id, nil
}

// nullIfBlank keeps an unstamped build out of the column as NULL rather than as
// an empty string, so "never had one" and "recorded as empty" cannot diverge.
func nullIfBlank(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// SetRepairState records the release an installed plugin was built for and why
// its last repair failed, so the store can explain a disabled plugin without
// re-deriving the reason on every request.
func (s *Service) SetRepairState(ctx context.Context, slug, builtFor, repairErr string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE appstore_plugins SET built_for = ?, repair_error = ?, updated_at = datetime('now') WHERE slug = ?`,
		builtFor, repairErr, slug)
	if err != nil {
		return fmt.Errorf("recording repair state for %s: %w", slug, err)
	}
	_ = db.RowsAffected(res)
	return nil
}

// MarkInstalledAsDependency flags a row as one the installer pulled in.
//
// Only ever set, never cleared. A dependency the operator later installs
// deliberately does not go through Install again — it is already there — so
// there is no moment at which the flag could honestly be cleared, and clearing
// it on some other signal would guess at intent.
func (s *Service) MarkInstalledAsDependency(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE appstore_plugins SET installed_as_dependency = 1 WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("flagging plugin as a dependency: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// InstalledAsDependency reports whether the installer pulled this plugin in
// rather than an operator asking for it. A missing row is false: nothing that
// is not installed is the host's to remove.
func (s *Service) InstalledAsDependency(ctx context.Context, slug string) (bool, error) {
	var flag int
	err := s.db.QueryRowContext(ctx,
		`SELECT installed_as_dependency FROM appstore_plugins WHERE slug = ?`, slug).Scan(&flag)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loading dependency flag: %w", err)
	}
	return flag == 1, nil
}

// DeleteRow removes the appstore_plugins row. Called as the last
// step of the uninstall gate chain (after files are deleted).
func (s *Service) DeleteRow(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM appstore_plugins WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("deleting appstore_plugins row: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// LookupEnabled returns whether the plugin row is currently marked
// enabled. Returns (false, nil) when no row exists for the slug —
// callers in the Update flow use the zero value as "stay disabled".
func (s *Service) LookupEnabled(ctx context.Context, slug string) (bool, error) {
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled FROM appstore_plugins WHERE slug = ?`, slug).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("loading enabled flag: %w", err)
	}
	return enabled == 1, nil
}

// MarkEnabled sets the enabled flag on the plugin row. The runtime
// reads this flag at boot to decide which plugins to load; flipping
// it after boot has no immediate effect (the loaded plugin keeps
// running until the next restart).
func (s *Service) MarkEnabled(ctx context.Context, slug string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE appstore_plugins SET enabled = ? WHERE slug = ?`, val, slug)
	if err != nil {
		return fmt.Errorf("setting enabled flag: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrPluginNotFound
	}
	return nil
}

// LookupVersion returns the installed version for slug, or empty
// string if the plugin is not installed.
func (s *Service) LookupVersion(ctx context.Context, slug string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT version FROM appstore_plugins WHERE slug = ?`, slug).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// --- uninstall gate chain helpers -------------------------------

// PermissionsForPlugin returns the permission keys the plugin
// claimed. Used by Uninstall to refuse if any role still references
// them.
func (s *Service) PermissionsForPlugin(ctx context.Context, slug string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT permission_key FROM plugin_permissions WHERE origin = ?`, slug)
	if err != nil {
		return nil, fmt.Errorf("loading plugin permissions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing plugin permission rows failed", "error", err)
		}
	}()
	out := make([]string, 0, 16)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RolesReferencingPermissions returns the role IDs whose permissions
// JSON contains any of the supplied keys. Used to gate uninstall.
func (s *Service) RolesReferencingPermissions(ctx context.Context, keys []string) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(keys) == 0 {
		return out, nil
	}
	// SQLite doesn't have a clean JSON contains operator. We do a
	// LIKE scan for each key inside the role row's permissions
	// column. Cheap because role counts are bounded. Returns role
	// names, not ids — the only consumer renders them to the admin.
	for _, key := range keys {
		names, err := s.roleNamesHolding(ctx, key)
		if err != nil {
			return nil, err
		}
		out[key] = append(out[key], names...)
	}
	return out, nil
}

// roleNamesHolding lists the role names whose permissions JSON mentions key, closing its cursor before the next key is scanned.
func (s *Service) roleNamesHolding(ctx context.Context, key string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM roles WHERE permissions LIKE ? LIMIT 1000`, "%\""+key+"\"%")
	if err != nil {
		return nil, fmt.Errorf("scanning roles for %s: %w", key, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing role name rows failed", "error", err)
		}
	}()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanning roles for %s: %w", key, err)
	}
	return names, nil
}

// StripKeysFromRoles removes every key in keys from the permissions
// array of every role that carries one, system presets included, and
// returns the sorted names of the roles it changed. Runs in one
// transaction so a half-stripped set of roles cannot survive a
// failure. The caller is expected to invalidate the RBAC cache.
//
// A role that named only keys the plugin owns is left with an empty
// array rather than deleted: the role is the operator's, only the
// grants were the plugin's.
func (s *Service) StripKeysFromRoles(ctx context.Context, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return []string{}, nil
	}
	drop := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		drop[k] = struct{}{}
	}
	type roleRow struct {
		id, name string
		perms    []string
	}
	var changed []string
	err := withTx(ctx, s.db, func(tx *sql.Tx) error {
		// No LIMIT: every role is rewritten here. A cap would leave the roles
		// past it holding a permission key the plugin no longer declares.
		pending, err := func() ([]roleRow, error) {
			rows, err := tx.QueryContext(ctx, `SELECT id, name, permissions FROM roles`)
			if err != nil {
				return nil, fmt.Errorf("listing roles: %w", err)
			}
			defer func() {
				if err := rows.Close(); err != nil {
					slog.Warn("plugin service: closing role rows failed", "error", err)
				}
			}()
			var pending []roleRow
			for rows.Next() {
				var id, name, raw string
				if err := rows.Scan(&id, &name, &raw); err != nil {
					return nil, fmt.Errorf("scanning role: %w", err)
				}
				var perms []string
				if err := json.Unmarshal([]byte(raw), &perms); err != nil {
					// A permissions column that is not a JSON array is
					// not granting plugin keys in a form the authorizer
					// reads; leave it alone.
					continue
				}
				kept := make([]string, 0, len(perms))
				touched := false
				for _, p := range perms {
					if _, gone := drop[p]; gone {
						touched = true
						continue
					}
					kept = append(kept, p)
				}
				if touched {
					pending = append(pending, roleRow{id: id, name: name, perms: kept})
				}
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return pending, nil
		}()
		if err != nil {
			return err
		}
		for _, r := range pending {
			encoded, err := json.Marshal(r.perms)
			if err != nil {
				return fmt.Errorf("encoding permissions for role %s: %w", r.id, err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE roles SET permissions = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				string(encoded), r.id); err != nil {
				return fmt.Errorf("stripping plugin keys from role %s: %w", r.id, err)
			}
			changed = append(changed, r.name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed == nil {
		changed = []string{}
	}
	sort.Strings(changed)
	return changed, nil
}

// GrantKeysToRoles adds each listed key to the named role's permissions
// array when it is not already there. Add-only and idempotent: keys a
// role already holds are untouched, and a role id that does not exist
// is skipped rather than created. Used at install time to apply a
// manifest's `presets` declarations and by the update flow to restore
// the grants the intermediate uninstall stripped. The caller is
// expected to invalidate the RBAC cache.
func (s *Service) GrantKeysToRoles(ctx context.Context, grants map[string][]string) error {
	if len(grants) == 0 {
		return nil
	}
	return withTx(ctx, s.db, func(tx *sql.Tx) error {
		// One read per role, bounded by the roles named in this grant call, and
		// each role is rewritten individually anyway.
		for roleID, keys := range grants {
			if len(keys) == 0 {
				continue
			}
			var raw string
			err := tx.QueryRowContext(ctx, `SELECT permissions FROM roles WHERE id = ?`, roleID).Scan(&raw)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("reading role %s: %w", roleID, err)
			}
			var perms []string
			if err := json.Unmarshal([]byte(raw), &perms); err != nil {
				perms = []string{}
			}
			have := make(map[string]struct{}, len(perms))
			for _, p := range perms {
				have[p] = struct{}{}
			}
			added := false
			for _, k := range keys {
				if _, ok := have[k]; ok {
					continue
				}
				perms = append(perms, k)
				have[k] = struct{}{}
				added = true
			}
			if !added {
				continue
			}
			sort.Strings(perms)
			encoded, err := json.Marshal(perms)
			if err != nil {
				return fmt.Errorf("encoding permissions for role %s: %w", roleID, err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE roles SET permissions = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				string(encoded), roleID); err != nil {
				return fmt.Errorf("granting plugin keys to role %s: %w", roleID, err)
			}
		}
		return nil
	})
}

// RoleGrantsForKeys returns, per role id, the subset of keys that
// role currently grants. The update flow snapshots this before the
// intermediate uninstall and hands it back to GrantKeysToRoles after
// the reinstall so a custom role's grants survive an update.
func (s *Service) RoleGrantsForKeys(ctx context.Context, keys []string) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(keys) == 0 {
		return out, nil
	}
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[k] = struct{}{}
	}
	// No LIMIT: every role is inspected here; a cap would report a plugin as
	// unreferenced because the role that references it fell past the cap.
	rows, err := s.db.QueryContext(ctx, `SELECT id, permissions FROM roles`)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing role rows failed", "error", err)
		}
	}()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		var perms []string
		if err := json.Unmarshal([]byte(raw), &perms); err != nil {
			continue
		}
		for _, p := range perms {
			if _, ok := want[p]; ok {
				out[id] = append(out[id], p)
			}
		}
	}
	return out, rows.Err()
}

// withTx runs fn inside a transaction, committing on nil and rolling
// back on error or panic.
func withTx(ctx context.Context, sqlDB *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			db.RollbackTx(tx)
			panic(rec) //nolint:forbidigo // re-raises the recovered panic once the transaction is rolled back
		}
		if err != nil {
			db.RollbackTx(tx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// DropABACRules removes every rbac_policies row whose origin=<slug>.
func (s *Service) DropABACRules(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rbac_policies WHERE origin = ?`, slug)
	return err
}

// DropRoutes removes every plugin_routes row whose origin=<slug>.
func (s *Service) DropRoutes(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_routes WHERE origin = ?`, slug)
	return err
}

// DropMenu removes every plugin_menu row whose origin=<slug>.
func (s *Service) DropMenu(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_menu WHERE origin = ?`, slug)
	return err
}

// DropNotificationAdapters removes every plugin_notification_adapters
// row whose origin=<slug>.
func (s *Service) DropNotificationAdapters(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM plugin_notification_adapters WHERE origin = ?`, slug)
	return err
}

// RecordNotificationAdapter mirrors one notification adapter's declared
// capabilities into plugin_notification_adapters.
//
// The flags already live in the in-memory adapter registry; this copy
// exists so callers outside the backend process — the reporting
// scheduler's target picker, above all — can ask whether a channel can
// carry a file without holding the registry. Written on every load, so
// a flag that flips in a rebuilt plugin is picked up without a
// reinstall.
func (s *Service) RecordNotificationAdapter(
	ctx context.Context,
	origin, channelType, displayName string,
	supportsFiles, supportsRich, encryptedCredentials bool,
	recipient plugin.NotificationRecipientOverride,
) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_notification_adapters
		   (origin, channel_type, display_name,
		    supports_file_attachments, supports_rich_attachments, encrypted_credentials,
		    recipient_key, recipient_kind, recipient_label, recipient_placeholder,
		    recipient_multiple)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (origin, channel_type) DO UPDATE SET
		   display_name              = excluded.display_name,
		   supports_file_attachments = excluded.supports_file_attachments,
		   supports_rich_attachments = excluded.supports_rich_attachments,
		   encrypted_credentials     = excluded.encrypted_credentials,
		   recipient_key             = excluded.recipient_key,
		   recipient_kind            = excluded.recipient_kind,
		   recipient_label           = excluded.recipient_label,
		   recipient_placeholder     = excluded.recipient_placeholder,
		   recipient_multiple        = excluded.recipient_multiple`,
		origin, channelType, displayName,
		supportsFiles, supportsRich, encryptedCredentials,
		strings.TrimSpace(recipient.Key), strings.TrimSpace(recipient.Kind),
		recipient.LabelEN, recipient.PlaceholderEN, recipient.Multiple,
	)
	return err
}

// DropWidgets removes every plugin_widgets row whose origin=<slug>.
func (s *Service) DropWidgets(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_widgets WHERE origin = ?`, slug)
	return err
}

// DropNodes removes every plugin_nodes row whose origin=<slug>.
func (s *Service) DropNodes(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_nodes WHERE origin = ?`, slug)
	return err
}

// DropThemes removes every plugin_themes row whose origin=<slug>.
func (s *Service) DropThemes(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_themes WHERE origin = ?`, slug)
	return err
}

// DropCatalogs removes the catalog sources this plugin declared. The
// settings_catalogs rows projected from them are reaped separately by
// catalogs.SeedPluginCatalogs, which runs after this.
func (s *Service) DropCatalogs(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_catalogs WHERE origin = ?`, slug)
	return err
}

// DropPermissions removes the plugin_permissions rows that this
// plugin claimed. Permissions themselves live inside the roles table
// as JSON; the Uninstall gate chain has already verified via
// RolesReferencingPermissions that no role still references them, so
// nothing else needs scrubbing here.
func (s *Service) DropPermissions(ctx context.Context, slug string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM plugin_permissions WHERE origin = ?`, slug); err != nil {
		return err
	}
	return nil
}

// TablePrefixFor is the table prefix a plugin owns by convention.
// Migrations are expected to create `plugin_<slug>_*` and nothing else,
// which is what makes an uninstall able to clean up after itself.
//
// Hyphens become underscores because SQL identifiers are not kebab-case:
// `storage-google-drive` creates `plugin_storage_google_drive_*`. Deriving
// with the raw slug produced `plugin_storage-google-drive_%`, which matches
// no table any migration ever wrote, so every hyphenated plugin — that is,
// nearly all of them — left its tables behind on uninstall. This must stay
// in step with pluginTablePrefix in core/pluginruntime/loader.go, which
// gates the same names at runtime.
func TablePrefixFor(slug string) string {
	return "plugin_" + strings.ReplaceAll(slug, "-", "_") + "_"
}

// DropTables removes every table under `prefix` that the plugin created
// via migrations. SQLite has no `DROP TABLE IF EXISTS` for dynamic names
// so we enumerate via sqlite_master.
//
// The prefix is passed in rather than derived because a plugin that has
// been renamed keeps the table names its migrations wrote — deriving
// from the current slug would walk straight past them and leave the
// tables behind. Callers pass the manifest's `table_prefix` when it
// declares one and `TablePrefixFor(slug)` when it does not.
func (s *Service) DropTables(ctx context.Context, prefix string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ?`, prefix+"%")
	if err != nil {
		return fmt.Errorf("enumerating plugin tables: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin service: closing plugin table rows failed", "error", err)
		}
	}()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// bounded by the tables one plugin uninstall owns; DROP TABLE IF EXISTS makes a retry safe
	for _, t := range tables {
		if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+t); err != nil {
			return fmt.Errorf("dropping table %s: %w", t, err)
		}
	}
	return nil
}

// --- ContributionTracker implementation --------------------------

// RecordRoute persists one plugin-registered route.
func (s *Service) RecordRoute(ctx context.Context, origin, method, path, requiresPermission string, isPublic bool) error {
	pub := 0
	if isPublic {
		pub = 1
	}
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_routes (id, origin, method, path, requires_permission, is_public)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, origin, method, path, requiresPermission, pub,
	)
	return err
}

// RecordMenu persists one plugin-registered menu item.
//
// sort_order is derived from how many rows this origin already has, so a
// plugin contributing several entries keeps the order its RegisterMenu
// calls them in. Without it every row sorts at 0 and the sidebar order
// is whatever SQLite happens to return.
func (s *Service) RecordMenu(ctx context.Context, origin, parent, labelNS, labelKey, path, icon, permission string, priority int) error {
	var order int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plugin_menu WHERE origin = ?`, origin,
	).Scan(&order); err != nil {
		order = 0
	}
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_menu (id, origin, parent, label_ns, label_key, path, icon, permission, sort_order, priority)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, origin, parent, labelNS, labelKey, path, icon, permission, order, priority,
	)
	return err
}

// RecordTab persists one plugin-registered page tab. Re-registration on a
// later boot replaces the row rather than failing on the unique key.
func (s *Service) RecordTab(ctx context.Context, origin, pageID, key, labelNS, labelKey, icon, permission string, sortOrder int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_tabs (id, origin, page_id, tab_key, label_ns, label_key, icon, requires_permission, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(origin, page_id, tab_key) DO UPDATE SET
		   label_ns = excluded.label_ns, label_key = excluded.label_key,
		   icon = excluded.icon, requires_permission = excluded.requires_permission,
		   sort_order = excluded.sort_order`,
		uuid.NewString(), origin, pageID, key, labelNS, labelKey, icon, permission, sortOrder,
	)
	return err
}

// DropTabs removes every plugin_tabs row whose origin=<slug>.
func (s *Service) DropTabs(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_tabs WHERE origin = ?`, slug)
	return err
}

// RecordWidget persists one plugin-registered widget.
func (s *Service) RecordWidget(ctx context.Context, origin, key, title, category, defaultSize, settingsJSON string) error {
	if settingsJSON == "" {
		settingsJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_widgets (origin, key, title, category, default_size, settings_json)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (origin, key) DO UPDATE SET
		   title = excluded.title, category = excluded.category, default_size = excluded.default_size,
		   settings_json = excluded.settings_json`,
		origin, key, title, category, defaultSize, settingsJSON,
	)
	return err
}

// RecordNode persists one plugin-registered workflow node. The
// configSchema / inputPorts / outputPorts / requiresPermission
// arguments are JSON-encoded by the loader; this service writes
// them through verbatim.
func (s *Service) RecordNode(ctx context.Context, origin, key, category, displayName, icon, configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_nodes (origin, key, category, display_name, icon,
		                          config_schema_json, input_ports_json, output_ports_json, requires_permission)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (origin, key) DO UPDATE SET
		   category = excluded.category,
		   display_name = excluded.display_name,
		   icon = excluded.icon,
		   config_schema_json = excluded.config_schema_json,
		   input_ports_json = excluded.input_ports_json,
		   output_ports_json = excluded.output_ports_json,
		   requires_permission = excluded.requires_permission`,
		origin, key, category, displayName, icon,
		configSchemaJSON, inputPortsJSON, outputPortsJSON, requiresPermission,
	)
	return err
}

// RecordTheme persists one plugin-declared theme.
func (s *Service) RecordTheme(ctx context.Context, origin, id, name, description, cssPath string, swatchHue int, defaultVariant string, hasDark, hasLight bool) error {
	dark := 0
	if hasDark {
		dark = 1
	}
	light := 0
	if hasLight {
		light = 1
	}
	if !hasDark && !hasLight {
		dark = 1
		light = 1
	}
	if defaultVariant != "light" {
		defaultVariant = "dark"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_themes (origin, theme_id, name, description, css_path, swatch_hue,
		                           default_variant, has_dark, has_light)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (origin, theme_id) DO UPDATE SET
		   name = excluded.name,
		   description = excluded.description,
		   css_path = excluded.css_path,
		   swatch_hue = excluded.swatch_hue,
		   default_variant = excluded.default_variant,
		   has_dark = excluded.has_dark,
		   has_light = excluded.has_light`,
		origin, id, name, description, cssPath, swatchHue, defaultVariant, dark, light,
	)
	return err
}

// RecordCatalog persists one plugin-declared catalog source.
func (s *Service) RecordCatalog(ctx context.Context, origin, catalogID, kind, name, url string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_catalogs (origin, catalog_id, kind, name, url)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (origin, catalog_id) DO UPDATE SET
		   kind = excluded.kind,
		   name = excluded.name,
		   url = excluded.url`,
		origin, catalogID, kind, name, url,
	)
	return err
}

// RecordCLIRoot persists a plugin's one top-level CLI command group. The name
// is globally unique, so a second plugin claiming it fails here rather than
// silently shadowing the first in every operator's command tree.
func (s *Service) RecordCLIRoot(ctx context.Context, origin, name, short, aliasesJSON string) error {
	if aliasesJSON == "" {
		aliasesJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_cli_roots (id, origin, name, short, aliases)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   origin = excluded.origin, short = excluded.short, aliases = excluded.aliases,
		   updated_at = CURRENT_TIMESTAMP`,
		uuid.NewString(), origin, name, short, aliasesJSON,
	)
	return err
}

// RecordCLICommand persists one declared command. The path is unique per plugin
// so a reload rewrites a command rather than accumulating duplicates of it.
func (s *Service) RecordCLICommand(ctx context.Context, origin string, cmd pluginruntime.CLICommandRow) error {
	envScoped := 0
	if cmd.EnvScoped {
		envScoped = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plugin_cli_commands
		   (id, origin, path, kind, method, route, args, flags, columns, body, needs, risk,
		    guest_kind, short, long, example, env_scoped)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(origin, path) DO UPDATE SET
		   kind = excluded.kind, method = excluded.method, route = excluded.route,
		   args = excluded.args, flags = excluded.flags, columns = excluded.columns,
		   body = excluded.body, needs = excluded.needs, risk = excluded.risk,
		   guest_kind = excluded.guest_kind, short = excluded.short, long = excluded.long,
		   example = excluded.example, env_scoped = excluded.env_scoped,
		   updated_at = CURRENT_TIMESTAMP`,
		uuid.NewString(), origin, cmd.Path, cmd.Kind, cmd.Method, cmd.Route,
		jsonOrEmptyArray(cmd.ArgsJSON), jsonOrEmptyArray(cmd.FlagsJSON),
		jsonOrEmptyArray(cmd.ColumnsJSON), cmd.Body, cmd.Needs, cmd.Risk,
		cmd.GuestKind, cmd.Short, cmd.Long, cmd.Example, envScoped,
	)
	return err
}

// DropCLI removes every CLI root and command row whose origin is slug, so an
// uninstalled plugin's root disappears from the next spec every operator fetches.
func (s *Service) DropCLI(ctx context.Context, slug string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plugin_cli_commands WHERE origin = ?`, slug); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugin_cli_roots WHERE origin = ?`, slug)
	return err
}

// jsonOrEmptyArray keeps a NOT NULL JSON column valid when a declaration left
// the field unset.
func jsonOrEmptyArray(raw string) string {
	if raw == "" {
		return "[]"
	}
	return raw
}

// ClearAll removes every plugin_<slug>_* contribution row. Called
// by the loader at the start of every Boot to avoid stale entries.
func (s *Service) ClearAll(ctx context.Context, origin string) error {
	for _, fn := range []func(context.Context, string) error{
		s.DropRoutes,
		s.DropMenu,
		s.DropTabs,
		s.DropWidgets,
		s.DropNodes,
		s.DropThemes,
		s.DropCatalogs,
		s.DropNotificationAdapters,
		s.DropCLI,
	} {
		if err := fn(ctx, origin); err != nil {
			return err
		}
	}
	return nil
}

// --- plugin config (key/value) -----------------------

// DropConfig removes every config row this plugin stored, encrypted secrets
// included, and reports how many went. Only the PurgeData uninstall path calls
// it — an ordinary uninstall keeps config so a reinstall finds its settings.
func (s *Service) DropConfig(ctx context.Context, slug string) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugin_config WHERE origin = ?`, slug)
	if err != nil {
		return 0, fmt.Errorf("purging plugin config for %s: %w", slug, err)
	}
	return int(db.RowsAffected(res)), nil
}

// LookupManifest reads plugin.yaml from the installed plugin's data
// directory and returns the parsed manifest. Returns (nil, nil) when the
// plugin is not installed or the file is missing, which callers render as a
// plugin whose manifest details are simply unavailable.
func (s *Service) LookupManifest(ctx context.Context, dataDir, slug string) (*plugin.Manifest, error) {
	_ = ctx
	version, err := s.LookupVersion(ctx, slug)
	if err != nil {
		return nil, err
	}
	if version == "" {
		return nil, nil
	}
	manifestPath := filepath.Join(dataDir, "plugins", slug+"@"+version, "plugin.yaml")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest for %s: %w", slug, err)
	}
	// yaml, not json: every manifest in this repo opens with the `#` copyright
	// header, so encoding/json failed on the first byte of all of them and the
	// caller silently returned a plugin with no manifest.
	var m plugin.Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest for %s: %w", slug, err)
	}
	return &m, nil
}

// MirrorManifest is a stub used only by cmd/testmirror that would mirror the manifest into
// the SQL contribution tables once the install-time mirror logic lands.
func (s *Service) MirrorManifest(_ context.Context, _ string, slug string, _ string) error {
	return fmt.Errorf("mirror manifest not yet implemented for %s (task #15)", slug)
}
