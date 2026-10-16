// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	"github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"
)

// NewPluginCatalogResolver returns a plugins.CatalogResolver backed by
// the settings_catalogs table. The returned value reads every row
// where kind='plugin' AND enabled=1, sorted readonly DESC, name ASC,
// so the official GitHub catalog appears first and user-added catalogs
// follow. The plugins package never imports the catalogs package; the
// type assertion is satisfied through the plugins.CatalogResolver
// interface, keeping the dependency one-way.
//
// A nil db is allowed in unit tests: the resolver returns no sources.
// The host never passes nil — main.go always holds a live DB handle.
func NewPluginCatalogResolver(db *sql.DB) plugins.CatalogResolver {
	return pluginCatalogResolverFunc(func(ctx context.Context, kind string) ([]plugins.PluginSource, error) {
		rows, err := enabledCatalogSources(ctx, db, kind)
		if err != nil {
			return nil, err
		}

		out := make([]plugins.PluginSource, 0, len(rows))
		for _, row := range rows {
			out = append(out, plugins.PluginSource{Name: row.name, URL: row.url})
		}
		return out, nil
	})
}

// NewAppCatalogResolver returns an appstore.CatalogResolver backed by
// enabled settings_catalogs rows of the requested kind.
func NewAppCatalogResolver(db *sql.DB) appstore.CatalogResolver {
	return appCatalogResolverFunc(func(ctx context.Context, kind string) ([]appstore.CatalogSource, error) {
		rows, err := enabledCatalogSources(ctx, db, kind)
		if err != nil {
			return nil, err
		}

		out := make([]appstore.CatalogSource, 0, len(rows))
		for _, row := range rows {
			out = append(out, appstore.CatalogSource{Name: row.name, URL: row.url, ReadOnly: row.readOnly})
		}
		return out, nil
	})
}

// NewPluginCatalogHandle returns the plugin.CatalogHandle the runtime hands
// to plugins that declare `catalog.source.register`. It reads the same
// enabled settings_catalogs rows the app and plugin stores resolve from, so a
// contributed category behaves like the two that were always there.
func NewPluginCatalogHandle(db *sql.DB) plugin.CatalogHandle {
	return pluginCatalogHandle{db: db}
}

type pluginCatalogHandle struct {
	db *sql.DB
}

func (h pluginCatalogHandle) Available() bool { return h.db != nil }

func (h pluginCatalogHandle) Sources(ctx context.Context, kind string) ([]plugin.CatalogSourceRef, error) {
	rows, err := enabledCatalogSources(ctx, h.db, kind)
	if err != nil {
		return nil, err
	}
	out := make([]plugin.CatalogSourceRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, plugin.CatalogSourceRef{Name: row.name, URL: row.url, ReadOnly: row.readOnly})
	}
	return out, nil
}

type catalogSourceRow struct {
	name string
	url  string
	// readOnly marks a built-in row. Only the app store consumes it, to
	// decide whether a source may be fetched over the network; the
	// plugin store gates remote reads on the scheme alone because a
	// plugin entry is an indirection the installer re-checks, not a
	// payload the host executes as-is.
	readOnly bool
}

func enabledCatalogSources(ctx context.Context, db *sql.DB, kind string) ([]catalogSourceRow, error) {
	if db == nil {
		return nil, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT name, url, readonly
		FROM settings_catalogs
		WHERE kind = ? AND enabled = 1
		ORDER BY readonly DESC, name ASC
		LIMIT 1000
	`, kind)
	if err != nil {
		return nil, fmt.Errorf("querying enabled %s catalogs: %w", kind, err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	out := make([]catalogSourceRow, 0, 4)
	for rows.Next() {
		var row catalogSourceRow
		if err := rows.Scan(&row.name, &row.url, &row.readOnly); err != nil {
			return nil, fmt.Errorf("scanning enabled %s catalog: %w", kind, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating enabled %s catalogs: %w", kind, err)
	}
	return out, nil
}

// pluginCatalogResolverFunc adapts a closure to the
// plugins.CatalogResolver interface without forcing every site to spell
// out the method dispatch.
type pluginCatalogResolverFunc func(ctx context.Context, kind string) ([]plugins.PluginSource, error)

// EnabledPluginSources is the single method on plugins.CatalogResolver.
func (f pluginCatalogResolverFunc) EnabledPluginSources(ctx context.Context, kind string) ([]plugins.PluginSource, error) {
	return f(ctx, kind)
}

type appCatalogResolverFunc func(ctx context.Context, kind string) ([]appstore.CatalogSource, error)

func (f appCatalogResolverFunc) EnabledAppSources(ctx context.Context, kind string) ([]appstore.CatalogSource, error) {
	return f(ctx, kind)
}
