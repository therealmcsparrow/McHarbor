// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// StorageAdapterRegistry is the sub-registry for storage destination
// adapters. A plugin registers one entry per location type it handles.
// Registering the same LocationType twice from the same plugin (or
// from two different plugins) is an error — the host uses the
// LocationType as the lookup key.
type StorageAdapterRegistry interface {
	// Register adds an adapter. The host dispatches destinations with
	// location_type == spec.LocationType to client. The client must be
	// safe for concurrent use — the host invokes it from multiple
	// goroutines for parallel uploads.
	Register(spec StorageAdapterSpec, client StorageAdapterClient) error
}

// StorageAdapterSpec describes a single destination type a plugin
// can handle. The host uses the spec to render the admin UI (form
// fields, capability hints) and to route destinations at upload time.
type StorageAdapterSpec struct {
	// LocationType is the unique key the host uses to dispatch
	// destinations. Built-in types include "local"; plugin-contributed
	// types should be lowercase, kebab-friendly, and prefixed with the
	// plugin slug when ambiguity is possible (e.g., "minio-s3" vs the
	// built-in "s3").
	LocationType string

	// DisplayName is the user-facing label shown in the destination
	// form's location_type dropdown.
	DisplayName string

	// Slug is the owning plugin's slug (e.g. "storage-onedrive-business").
	// The host surfaces it alongside the spec so the frontend can
	// resolve icon assets via /api/plugins/loader/icon without a
	// second round-trip. Plugins populate this in RegisterAdapters.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory (resolved by the host's /api/plugins/loader/icon
	// route at render time). Empty string is valid — renderers
	// fall back to their default tile (IconPuzzle, cloud, …).
	// The host trusts the value because it is declared in the
	// plugin's own plugin.yaml — an attacker who can edit a
	// plugin's manifest already controls the .so bundle.
	Icon string

	// Capabilities describes which operations the adapter supports.
	// The host hides unsupported operations from the admin UI and
	// returns ErrUnsupported for runtime calls to capabilities the
	// adapter did not declare.
	Capabilities StorageAdapterCapabilities

	// ConfigSchema is the list of form fields the admin UI renders
	// when an operator picks this destination type. The host already
	// knows about the per-type columns on storage_locations; plugins
	// extend the schema by adding custom keys that the host stores in
	// the same row's columns (host, port, base_path, etc.) when they
	// line up, or in a plugin-owned column for anything truly
	// plugin-specific. This is intentionally simple for v1 — a richer
	// JSON-typed config column is a follow-up.
	ConfigSchema []types.ConfigFieldDecl
}

// StorageAdapterCapabilities declares which operations the adapter
// supports. The host consults these before invoking the corresponding
// method on StorageAdapterClient.
type StorageAdapterCapabilities struct {
	// EncryptedCredentials indicates the adapter accepts credential
	// fields that the host should encrypt at rest via the
	// EncryptionHandle. Adapters that store only metadata (e.g., the
	// "local" destination) leave this false.
	EncryptedCredentials bool

	// OAuth indicates the adapter implements the OAuth handshake the
	// host exposes via /storage-locations/{id}/oauth/authorize. The
	// adapter owns the redirect target, token exchange, and refresh.
	OAuth bool

	// SupportsListing indicates the adapter implements List. Some
	// object stores (e.g., S3 Glacier) can't list cheaply; those
	// adapters leave this false.
	SupportsListing bool

	// SupportsDelete indicates the adapter implements Delete.
	// Append-only destinations leave this false.
	SupportsDelete bool
}

// ErrUnsupported is returned by StorageAdapterClient methods that the
// adapter's spec declared as not supported. The host translates this
// into a 400 with an i18n code so the admin UI can hide the action.
var ErrUnsupported = errors.New("plugin storage adapter: operation not supported by spec")

// StorageAdapterClient is the runtime contract for plugin-provided
// destination handlers. The host calls these methods when a backup
// run targets a destination whose location_type matches the adapter's
// registered LocationType.
//
// All methods receive a config map carrying the destination's row
// columns (host, port, base_path, region, bucket, etc.) keyed by
// column name. Plugins decide which keys they need; the host does
// not pre-validate the map.
//
// Implementations MUST be safe for concurrent use. The host invokes
// Test, Upload, Download, Delete, and List from parallel goroutines
// during multi-destination runs.
type StorageAdapterClient interface {
	// Test verifies the destination is reachable and the credentials
	// work. The host calls this from the admin UI's "Test connection"
	// button. Implementations should validate every config field they
	// will use at runtime and return a descriptive error on failure.
	Test(ctx context.Context, config map[string]string) error

	// Upload writes r to key under the destination. key is relative
	// to the destination's base_path (or the equivalent plugin-defined
	// root). Implementations must create intermediate directories as
	// needed. The host calls Upload with a fresh io.Reader; the
	// adapter does not own the reader past Upload's return.
	Upload(ctx context.Context, config map[string]string, key string, r io.Reader) error

	// Download returns a reader for key. The caller is responsible
	// for closing the returned ReadCloser. Adapters that don't
	// support download return ErrUnsupported.
	Download(ctx context.Context, config map[string]string, key string) (io.ReadCloser, error)

	// Delete removes key. No-op if the key doesn't exist. Adapters
	// that declared SupportsListing=false should still implement
	// Delete (the absence of Delete is rare).
	Delete(ctx context.Context, config map[string]string, key string) error

	// List returns objects under prefix. Adapters that declared
	// SupportsListing=false return ErrUnsupported. The prefix is
	// always a non-empty path; the adapter returns entries whose key
	// starts with prefix.
	List(ctx context.Context, config map[string]string, prefix string) ([]StorageObject, error)

	// ConsentURL builds the provider's OAuth authorize URL for the
	// given location. Plugins that need delegated authorization
	// (Microsoft, Google Drive, etc.) implement this; the host's
	// /storage-locations/{id}/oauth/authorize handler routes through
	// it when the adapter's LocationType is plugin-owned. Adapters
	// whose spec declared OAuth=false or whose LocationType doesn't
	// use delegated authorization (e.g. local, SFTP) return
	// ErrUnsupported.
	//
	//   locationID    — host's storage location row id
	//   locationType  — same value as the destination's location_type
	//                   column ("onedrive_personal",
	//                   "onedrive_business", "sharepoint", …)
	//   publicBaseURL — host's externally-visible origin
	//                   (scheme + host + port); the plugin uses it to
	//                   construct the redirect_uri parameter
	//   state         — host's opaque correlation token; the plugin
	//                   MUST echo it through to the provider's
	//                   authorize URL so HandleOAuthCallback can
	//                   correlate the returning request back to the
	//                   originating location
	//   cfg           — current config map (read-only). Plugins
	//                   decide which keys they need to build the URL
	//                   (client_id, tenant_id, …); the host does not
	//                   pre-validate.
	//
	// The returned URL is what the host hands back to the admin UI,
	// which then redirects the operator's browser to it.
	ConsentURL(ctx context.Context, locationID, locationType, publicBaseURL, state string, cfg map[string]string) (string, error)

	// ConsentCallback completes the provider's OAuth handshake after
	// the operator has consented. The host has already validated
	// `state`; plugins do not re-validate. The plugin exchanges the
	// authorization code for an access token, persists any new
	// tokens (refresh_token, access_token, …) into the returned
	// config map, and returns it. The host merges those keys back
	// into the storage_locations row so the next Test / Upload /
	// Download / Delete call sees them in cfg.
	//
	// Plugins MUST return ErrUnsupported when the adapter's
	// LocationType doesn't use OAuth, matching ConsentURL. Plugins
	// MAY return a partial map (only the keys that changed); the
	// host merges with the existing cfg rather than replacing it.
	ConsentCallback(ctx context.Context, locationID, locationType, publicBaseURL, code, state string, cfg map[string]string) (map[string]string, error)
}

// TestStep is one named phase of a storage-connection probe.
//
// Mirrors modules/storage_locations.TestStep field-for-field. Declared
// here so plugins can return it directly without importing a host
// module — host modules alias it via `type TestStep = storage.TestStep`
// (see modules/storage_locations/test.go) so the JSON shape is
// identical end-to-end.
//
// Status is one of "pass" | "warn" | "fail" | "skip". The host rolls
// the worst step into TestStorageLocationResult.OverallStatus. Latency
// is a Go-formatted duration string ("6.58ms") — pre-formatted on the
// plugin side because plugin SDK can't depend on the host's duration
// helpers.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// RicherStorageTester is the OPTIONAL richer-test contract. Plugins
// that implement it surface multi-step write/change/delete output
// (matching the built-in local destination's test) instead of the
// single "plugin_test" step the host's runPluginStorageTest emits
// when only the SDK's Test method exists.
//
// Discovery is via type assertion at the dispatch boundary — adding
// this interface does not break plugins that don't implement it. They
// keep the existing single-step Test result.
//
// Implementation notes:
//
//   - Each step's Latency is measured by the plugin (the SDK has no
//     duration helper). Format via time.Since(start).String() to match
//     the host's local-path output ("6.58ms", "27.82ms").
//   - On a step failure the plugin returns a slice with the
//     successful steps followed by one failing step. Aborting early
//     (after the first fail) is the conventional pattern and matches
//     modules/storage_locations.testLocalRoundTrip's behavior.
//   - The returned slice should NOT include a "credentials" step —
//     the host adds that automatically before the plugin dispatch,
//     matching the order and structure operators see from the local
//     destination.
//   - cfg["location_type"] identifies which LocationType the plugin
//     is being asked to test; one plugin can implement TestSteps for
//     multiple LocationTypes when those locations intentionally share
//     a client.
type RicherStorageTester interface {
	TestSteps(ctx context.Context, config map[string]string) ([]TestStep, error)
}

// StorageObject describes a single object returned by a
// StorageAdapterClient.List call.
type StorageObject struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// StorageQuota is the used / total capacity a destination reports.
// Total is 0 when the provider has no hard ceiling (e.g. object
// stores billed per byte); Used still carries the consumed bytes so
// the host can show an absolute figure without a percentage.
type StorageQuota struct {
	UsedBytes  int64 `json:"usedBytes"`
	TotalBytes int64 `json:"totalBytes"`
}

// StorageQuotaReporter is the OPTIONAL capacity contract. Plugins
// that implement it let the host render a used-of-total bar on the
// storage location card (Settings → Storage). Discovery is via type
// assertion at the dispatch boundary, so plugins that do not
// implement it keep working — the host shows "usage unavailable".
//
// Implementations should return the provider's own quota view (Graph
// `/drive` quota for OneDrive, `space_usage` for Dropbox, statvfs on
// a mounted share, …) and must not walk the tree to sum object sizes
// on every call — the host queries this on each settings page load.
type StorageQuotaReporter interface {
	Quota(ctx context.Context, config map[string]string) (StorageQuota, error)
}
