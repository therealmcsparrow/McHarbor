// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import "github.com/therealmcsparrow/mcharbor/core/plugin"

// PluginEntry is one row in the appstore plugin catalog. One entry
// per plugin slug — not per version. The Versions slice carries
// every release of that plugin the catalog has on hand, sorted
// descending by semver so Versions[0] is always the latest.
//
// Wire format on disk accepts two shapes:
//
//  1. Legacy (one file per version):
//
//     {
//     "slug": "snapshot-zfs",
//     "name": "ZFS Snapshot Driver",
//     "version": "1.0.0",
//     "download_url": "file:///.../snapshot-zfs-1.0.0.tar.gz",
//     ...
//     }
//
//  2. Grouped (one file per plugin, multiple versions inside):
//
//     {
//     "slug": "snapshot-zfs",
//     "name": "ZFS Snapshot Driver",
//     "versions": [
//     {"version": "1.0.1", "download_url": "..."},
//     {"version": "1.0.0", "download_url": "..."}
//     ],
//     ...
//     }
//
// readLocalCatalog normalises both shapes into the grouped wire
// format below before returning, so the rest of the host and the
// frontend never see anything but PluginEntry-with-Versions.
//
// The top-level Version / DownloadURL / Checksum / Signature fields
// are populated from Versions[0] (the latest release) as a
// convenience — single-version catalog files and legacy callers
// reading `entry.download_url` keep working unchanged.
type PluginEntry struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Top-level Version mirrors Versions[0].Version (the latest
	// release). The frontend and the install/update handlers both
	// prefer Versions[0] when present and otherwise fall back to
	// this field for the legacy single-version shape.
	Version string `json:"version"`
	// Build mirrors Versions[0].Build, for the same reason Version does.
	Build       string `json:"build,omitempty"`
	Author      string `json:"author"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	// Icon is a Tabler icon name (e.g. "IconBrandSlack",
	// "IconFolder"). The frontend resolves it through its plugin-
	// icons map at render time; unknown names fall back to
	// IconPuzzle. Optional — empty string renders the fallback.
	Icon string `json:"icon,omitempty"`
	// IconURL is set only for an entry read from a remote catalog whose Icon is an asset file
	// name; the frontend then loads it through GET /appstore/plugins/{slug}/icon.
	IconURL string `json:"icon_url,omitempty"`
	// Versions is the list of every release of this plugin the
	// catalog can serve, sorted descending by semver. Always
	// non-empty for a valid entry.
	Versions []PluginVersion `json:"versions"`
	// Top-level DownloadURL mirrors Versions[0].DownloadURL so the
	// legacy "one entry per version" callers (handleUpdateStream)
	// can keep reading `entry.DownloadURL` without changes.
	DownloadURL string `json:"download_url,omitempty"`
	// Top-level Checksum / Signature / BuiltFor likewise mirror Versions[0].
	Checksum  string `json:"checksum,omitempty"`
	Signature string `json:"signature,omitempty"`
	BuiltFor  string `json:"built_for,omitempty"`
}

// PluginVersion is one release of a plugin the catalog can serve.
type PluginVersion struct {
	// Version is a semver-style identifier. Compared lexicographically
	// against the major/minor/patch triple; pre-release suffixes
	// (e.g. "1.0.0-rc1") are not yet distinguished from the
	// matching release tag — the loader punts pre-release handling
	// to plugin authors and the host's own upgrade policy.
	Version string `json:"version"`
	// Build is the artifact's yyyyMMddHHmm compile stamp. Two artifacts at one
	// version differ only by this, which is what lets a rebuild be offered as
	// an update without moving the version.
	Build string `json:"build,omitempty"`
	// DownloadURL is the tarball the installer fetches. Local
	// dev-mode catalogs use file://, prod uses https://.
	DownloadURL string `json:"download_url"`
	// Checksum is a hex-encoded sha256 the installer verifies
	// before extracting. Optional; the signer audit log records
	// the integrity regardless.
	Checksum string `json:"checksum,omitempty"`
	// Signature is an ed25519 signature over the catalog entry's
	// canonical form. Verified by the installer before install.
	Signature string `json:"signature,omitempty"`
	// BuiltFor names the McHarbor release this artifact was compiled
	// against. Empty means a legacy entry, which the installer accepts.
	BuiltFor string `json:"built_for,omitempty"`
}

// InstalledPlugin is one row in the appstore_plugins table plus
// its current on-disk status.
type InstalledPlugin struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Version string `json:"version"`
	// Build is the compile stamp of the installed artifact, empty for one
	// installed before builds existed.
	Build       string `json:"build,omitempty"`
	Enabled     bool   `json:"enabled"`
	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at"`
	Loaded      bool   `json:"loaded"`
	// Signed is true when the installed tarball carried a valid
	// signature from a configured trust key. False means the operator
	// installed an unsigned third-party plugin and accepted the risk —
	// the UI badges it rather than letting the two look alike.
	Signed bool `json:"signed"`
	// PermissionsReferenced counts the roles that currently grant at
	// least one of this plugin's keys; RolesReferencing names them.
	// The uninstall confirm dialog uses both to warn that those grants
	// will be stripped. Filled by ListInstalled.
	PermissionsReferenced int      `json:"permissions_referenced"`
	RolesReferencing      []string `json:"roles_referencing"`
	// BuiltFor names the McHarbor release this install was compiled
	// against. A value other than the running release means the .so
	// cannot load and the reconciler must refetch it.
	BuiltFor string `json:"built_for"`
	// RepairError is the last reason an automatic repair failed, kept
	// so the store can explain a disabled plugin without re-deriving it.
	RepairError string `json:"repair_error"`
}

// PluginDetail extends InstalledPlugin with the parsed manifest so
// the admin UI can render the config_schema and other contributions.
type PluginDetail struct {
	InstalledPlugin
	Manifest *plugin.Manifest `json:"manifest,omitempty"`
}

// InstallRequest is the body of POST /api/appstore/plugins/install.
// Exactly one of URL or Tarball is set.
type InstallRequest struct {
	URL      string `json:"url,omitempty"`
	Tarball  []byte `json:"tarball_b64,omitempty"`
	Checksum string `json:"checksum,omitempty"`
	// Signature is the catalog entry's base64 ed25519 signature for
	// this archive. Optional — omitted means "none offered", not
	// "skip the check": the installer still tries the .sig sidecar.
	Signature string `json:"signature,omitempty"`
	// AllowUnsigned is the operator's explicit consent to install a tarball
	// no trust key verifies. Omitted, an unsigned tarball is refused with
	// err.plugin.unsigned_confirmation_required and nothing is installed.
	AllowUnsigned bool `json:"allow_unsigned,omitempty"`
}

// UpdateRequest is the optional body of POST
// /api/appstore/plugins/{slug}/update/stream. An empty body is a signed-only
// update; AllowUnsigned carries the same consent as InstallRequest's.
type UpdateRequest struct {
	AllowUnsigned bool `json:"allow_unsigned,omitempty"`
}

// InstallResult is the success envelope.
type InstallResult struct {
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	Path        string `json:"path"`
	NeedsEnable bool   `json:"needs_enable"`
}

// UninstallRequest is the body of POST /api/appstore/plugins/uninstall.
type UninstallRequest struct {
	Slug string `json:"slug"`
	// PurgeData also clears what an ordinary uninstall keeps: the plugin's
	// config rows, encrypted secrets included, and catalog sources left behind
	// for a kind no remaining plugin declares. Defaults to false — an absent
	// field must never destroy data.
	PurgeData bool `json:"purge_data,omitempty"`
}
