// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import (
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/backup"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/logging"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/notification"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/scanner"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/snapshot"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/storage"
)

// StorageAdapterSource is the read-only slice of the plugin runtime
// the host uses to resolve a plugin-owned LocationType to its
// StorageAdapterClient + spec. The runtime satisfies this interface
// implicitly; consumers (storage_locations, container_backups, …)
// type-assert via `_, ok := runtime.(host.StorageAdapterSource)`.
//
// Hoisted from the previously duplicated local definitions in
// modules/storage_locations and modules/storage_locations/storage_locations.
// Adding a method here is a single-point change; the local duplicates
// had drifted silently when the SDK grew (see memory
// `mcharbor-storage-adapter-interface-arity`). One definition, one
// signature, one place to update.
//
// Why host: the interface is implemented by the plugin runtime and
// consumed only by host modules. Plugins never import this — the
// plugin linker never reaches this package, so this interface
// cannot contribute to the host ↔ plugin ABI hash mismatch.
type StorageAdapterSource interface {
	StorageAdapter(locationType string) (storage.StorageAdapterClient, storage.StorageAdapterSpec, bool)
}

// NotificationAdapterSource is the read-only slice of the plugin
// runtime the host uses to resolve a plugin-owned ChannelType to its
// NotificationAdapterClient + spec. Mirrors StorageAdapterSource.
//
// Imported only by modules/communications; not by plugins.
type NotificationAdapterSource interface {
	NotificationAdapter(channelType string) (notification.NotificationAdapterClient, notification.NotificationAdapterSpec, bool)
}

// SnapshotAdapterSource is the read-only slice of the plugin runtime
// the host uses to resolve a snapshot_config's driver to its
// SnapshotAdapterClient + spec. The runtime satisfies this interface
// implicitly; consumers (container_backups, settings) type-assert via
// `_, ok := runtime.(host.SnapshotAdapterSource)`.
//
// Mirrors StorageAdapterSource: one definition, one signature, one
// place to update.
//
// Imported only by modules/container_backups; not by plugins.
type SnapshotAdapterSource interface {
	SnapshotAdapter(driver string) (snapshot.SnapshotAdapterClient, snapshot.SnapshotAdapterSpec, bool)
}

// LogForwarderAdapterSource is the read-only slice of the plugin
// runtime the host uses to resolve audit log forwarders. Mirrors
// StorageAdapterSource, with one deliberate difference: the audit
// emit path does not know a forwarder type up front — it fans every
// event out to *every* registered forwarder — so this source exposes
// an enumeration (LogForwarderAdapters) alongside the by-key lookup
// (LogForwarderAdapter) the Settings UI needs.
//
// Imported only by modules/audit and modules/plugin_runtime; not by
// plugins.
type LogForwarderAdapterSource interface {
	// LogForwarderAdapter resolves one forwarder by its
	// ForwarderType ("siem", "syslog"). Used by the Logging tab to
	// route Test and config saves.
	LogForwarderAdapter(forwarderType string) (logging.LogForwarderAdapterClient, logging.LogForwarderAdapterSpec, bool)

	// LogForwarderAdapters enumerates every registered forwarder, in
	// registration order. Used by the audit emit path to fan out and
	// by the Logging tab to render one tile per installed plugin.
	LogForwarderAdapters() []logging.LogForwarderAdapterSpec
}

// AlertSourceAdapterSource is the read-only slice of the plugin
// runtime the host's alert engine uses to resolve a stored rule's
// source_type to the code that evaluates it.
//
// Mirrors LogForwarderAdapterSource rather than the by-key-only
// sources, and for the same reason: the engine needs both halves.
// The by-key lookup resolves one rule at evaluation time; the
// enumeration builds the condition catalog the rule builder renders,
// which has to list every condition of every installed source
// without knowing any of their keys in advance.
//
// Imported only by modules/alerts; not by plugins.
type AlertSourceAdapterSource interface {
	// AlertSourceAdapter resolves one source by its SourceType.
	// Returns ok=false when no plugin has registered it — which is
	// the normal state for a rule whose plugin was uninstalled, and
	// must be handled by skipping the rule, never by deleting it.
	AlertSourceAdapter(sourceType string) (alerting.AlertSourceAdapterClient, alerting.AlertSourceAdapterSpec, bool)

	// AlertSourceAdapters enumerates every registered source spec in
	// registration order, for the condition catalog.
	AlertSourceAdapters() []alerting.AlertSourceAdapterSpec
}

// ImageScannerAdapterSource is the read-only slice of the plugin
// runtime the host uses to resolve a `scanner_configs` row's
// scanner_type to the engine that runs it.
//
// Mirrors LogForwarderAdapterSource, and for the same reason: both
// halves are needed. The by-key lookup runs one configured scanner;
// the enumeration builds the Add picker in the Scanners tab, which
// has to list every installed engine without knowing any of their
// keys in advance.
//
// Imported only by modules/scans; not by plugins.
type ImageScannerAdapterSource interface {
	// ImageScannerAdapter resolves one scanner by its ScannerType
	// ("trivy", "grype", "clair"). Returns ok=false when no plugin
	// has registered it — the normal state for a config row whose
	// plugin was uninstalled, which the host renders as an
	// unavailable row rather than deleting.
	ImageScannerAdapter(scannerType string) (scanner.ImageScannerAdapterClient, scanner.ImageScannerAdapterSpec, bool)

	// ImageScannerAdapters enumerates every registered scanner spec
	// in registration order, for the Add picker.
	ImageScannerAdapters() []scanner.ImageScannerAdapterSpec
}

// BackupProviderSource is the read-only slice of the plugin runtime the
// Backups page narrows the adapter registry to.
//
// It lists rather than resolves by key, because the page aggregates every
// provider at once rather than dispatching to one — the opposite shape from
// StorageAdapterSource, and the reason it is its own interface.
type BackupProviderSource interface {
	BackupProviders() []BackupProviderEntry
}

// BackupProviderEntry is one registered provider and its spec.
type BackupProviderEntry struct {
	Spec   backup.BackupProviderSpec
	Client backup.BackupProviderClient
}
