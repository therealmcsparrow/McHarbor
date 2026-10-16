// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// pluginAssetURL returns the host URL that streams an asset shipped
// inside an installed plugin's tarball. The route is mounted under
// the protected middleware group at
// `/api/plugins/loader/icon?slug=<slug>&name=<name>` and serves
// `<dataDir>/plugins/<slug>@<version>/icons/<name>` from disk.
//
// Return values:
//   - "" when slug or name is falsy — renderers fall back to
//     IconPuzzle / cloud / etc.
//   - the full path otherwise — renderers drop it into <img src>
//     directly.
//
// The route is registered alongside `/plugins/loader/bundle` in
// `plugin_runtime.handler.go` and reads the same on-disk layout the
// installer writes, so any plugin tarball that ships an icons/ dir
// shows up here without further wiring.
//
// `variant` asks the host for a theme-specific mark: 'dark' resolves
// `<base>_dark.<ext>` (e.g. `plugin_dark.svg`) and silently serves the
// plain asset when the plugin ships none, so callers never have to
// probe for the variant or handle a 404. Prefer `<PluginAssetImage>`
// over calling this directly — it reads the resolved theme for you.
export function pluginAssetURL(slug: string, itemName: string, variant?: 'dark'): string {
  if (!slug || !itemName) return '';
  const params = new URLSearchParams({ slug, name: itemName });
  if (variant) params.set('variant', variant);
  return `/api/plugins/loader/icon?${params.toString()}`;
}

/**
 * The icon a remote catalog publishes for a plugin that is not installed. The host fetches
 * and caches it, so the browser never contacts the catalog host itself.
 */
export function catalogIconURL(slug: string, variant?: 'dark'): string {
  if (!slug) return '';
  const query = variant ? `?variant=${variant}` : '';
  return `/api/appstore/plugins/${encodeURIComponent(slug)}/icon${query}`;
}

export function pluginThemeAssetURL(slug: string, path: string): string {
  if (!slug || !path) return '';
  const params = new URLSearchParams({ slug, path });
  return `/api/plugins/loader/asset?${params.toString()}`;
}
