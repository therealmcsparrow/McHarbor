// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Shared TypeScript shapes for the plugin store. These mirror the Go
// types in src/backend/modules/appstore/plugins/model.go so the
// admin UI can render the same catalog and installed views the
// backend publishes.
//
// Field names follow snake_case to match the JSON wire format the
// backend emits — we don't translate to camelCase at the boundary.

export interface InstalledPlugin {
  id: string;
  slug: string;
  version: string;
  /** The artifact's compile stamp, absent on a plugin installed before
   *  builds existed. Two installs at one version differ only by this. */
  build?: string;
  enabled: boolean;
  installed_at: string;
  updated_at: string;
  loaded: boolean;
  /** True when the installed tarball carried a valid signature from a
   *  configured trust key. False is an ordinary third-party install the
   *  operator accepted the risk on — badged, not blocked. */
  signed: boolean;
  // Roles that currently grant at least one of this plugin's keys.
  // Uninstall strips those grants; the confirm dialog names them.
  permissions_referenced: number;
  roles_referencing: string[];
  built_for: string;
  repair_error: string;
}

// One row in the local plugin catalog. Returned by
// GET /api/appstore/plugins/catalog. Empty catalog returns [].
//
// One entry per plugin slug — not per version. `versions` carries
// every release of the plugin the catalog has, sorted descending by
// semver. `versions[0]` is the latest release the host will install
// if no explicit version is chosen.
//
// The flat `version` / `download_url` / `checksum` / `signature`
// fields mirror `versions[0]` so legacy single-version consumers
// don't have to special-case the array.
export interface PluginEntry {
  slug: string;
  name: string;
  /** `versions[0].version` — the latest release. Convenience
   *  duplicate of the top of the versions array so legacy
   *  single-version consumers (compare-to-installed, update badge,
   *  install trigger) don't have to special-case missing arrays. */
  version: string;
  /** `versions[0].build` — the compile stamp of the latest artifact. */
  build?: string;
  author: string;
  license: string;
  homepage: string;
  description: string;
  category: string;
  /** Tabler icon name (e.g. "IconBrandSlack"). Resolved client-side
   *  via appstore/components/plugin-icons. Unknown names render the
   *  IconPuzzle fallback so a stale catalog never breaks the card. */
  icon?: string;
  /** Set when the entry came from a remote catalog and `icon` is an asset file; the host
   *  then serves it from `/api/appstore/plugins/{slug}/icon`. */
  icon_url?: string;
  signature?: string;
  checksum?: string;
  /** `versions[0].download_url` — the URL the installer fetches
   *  when the host picks the latest version on the user's behalf. */
  download_url: string;
  /** Sorted descending by semver; the catalog loader guarantees
   *  this. Always non-empty for a valid entry. */
  versions: PluginVersion[];
}

// One release of a plugin the catalog can serve.
export interface PluginVersion {
  version: string;
  /** Local dev-mode catalogs use file://, prod uses https://.
   *  The installer accepts both schemes. */
  download_url: string;
  /** Optional sha256 in hex. Verified by the installer before
   *  extracting. */
  checksum?: string;
  /** Optional ed25519 signature over the catalog entry's canonical
   *  form. Verified by the installer before install. */
  signature?: string;
}

// POST /api/appstore/plugins/install body. For local catalog
// installs the UI sends `url: "file:///mnt/...tar.gz"` (the
// resolver accepts file://, https://, and loopback http://).
export interface PluginInstallRequest {
  url: string;
  checksum?: string;
}

export interface PluginInstallResult {
  slug: string;
  version: string;
  path: string;
  needs_enable: boolean;
}

// Plugin manifest returned by GET /api/appstore/plugins/{slug}
// alongside the installed row when the data dir is configured.
export interface PluginManifest {
  slug: string;
  name: string;
  version: string;
  author: string;
  description?: string;
  permissions?: string[];
}
