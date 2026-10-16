// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package plugins installs, updates and reconciles McHarbor plugins from the local and remote plugin catalogs.
package plugins

import (
	"context"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/catalogindex"
)

// PluginSource describes one catalog the plugin store should read.
// URL is interpreted by readSource: file:// (or bare absolute
// path) → readLocalCatalogDir, http(s):// → readRemoteCatalog,
// anything else → error logged and source skipped.
type PluginSource struct {
	Name string
	URL  string
}

// CatalogResolver returns the enabled plugin sources for a given
// kind (today only "plugin"). The production implementation reads
// the settings_catalogs registry; cmd/testmirror passes nil and
// falls back to the legacy single-directory reader.
type CatalogResolver interface {
	EnabledPluginSources(ctx context.Context, kind string) ([]PluginSource, error)
}

// readSource dispatches a single PluginSource to the right reader.
// All errors are returned to the caller, which logs and continues
// so one broken catalog doesn't blank out the whole list.
func (s *Service) readSource(ctx context.Context, src PluginSource) ([]PluginEntry, error) {
	url := strings.TrimSpace(src.URL)
	if url == "" {
		return nil, nil
	}
	switch {
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"):
		// Built-in rows store a browsable GitHub tree URL because that
		// is what an operator wants to click in Settings; HTTP cannot
		// list a directory. Resolve it to the raw index.json the same
		// way the App Store reader and the Settings connection probe
		// do, so all three read one document.
		if indexURL, ok := catalogindex.Resolve(url); ok {
			url = indexURL
		}
		entries, err := readRemoteCatalog(ctx, url)
		if err != nil {
			return nil, err
		}
		s.assignRemoteIcons(url, entries)
		return entries, nil
	default:
		// Strip optional file:// scheme so "file:///mnt/plugins"
		// and "/mnt/plugins" both resolve to the same path.
		path := strings.TrimPrefix(url, "file://")
		return readLocalCatalogDir(path)
	}
}
