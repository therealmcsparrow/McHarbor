// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package snapshot

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// SnapshotAdapterRegistry is the sub-registry for filesystem/driver-
// level snapshot adapters (ZFS, btrfs, LVM, Synology DSM, TrueNAS
// middleware, …). A plugin registers one entry per driver it handles.
// Registering the same Driver twice from the same plugin (or from two
// different plugins) is an error — the host uses the Driver as the
// lookup key.
//
// Plugins in this family hook into the host's container-backup
// pipeline: when a backup plan has consistency_mode="snapshot" and
// references a snapshot config whose driver matches an adapter, the
// host invokes Create before reading the archive and Remove after.
// This avoids `docker pause` for plans where the operator has a
// snapshot-capable driver available.
type SnapshotAdapterRegistry interface {
	// Register adds an adapter. The host dispatches snapshot configs
	// with driver == spec.Driver to client. The client must be safe
	// for concurrent use — multiple backup runs can target the same
	// driver concurrently, and the admin UI's "Test" button can
	// fire while a backup is in progress.
	Register(spec SnapshotAdapterSpec, client SnapshotAdapterClient) error
}

// SnapshotAdapterSpec describes a single snapshot driver a plugin
// can handle. The host uses the spec to render the admin UI (form
// fields, capability hints) and to route Create/Remove at backup
// time.
type SnapshotAdapterSpec struct {
	// Driver is the unique key the host uses to dispatch snapshot
	// configs. Built-in / conventional values include "zfs",
	// "btrfs", "lvm", "synology", "truenas". Plugin-contributed
	// drivers should be lowercase, kebab-friendly, and prefixed
	// with the plugin slug when ambiguity is possible.
	Driver string

	// DisplayName is the user-facing label shown in the snapshot
	// config form's driver dropdown.
	DisplayName string

	// Slug is the owning plugin's slug (e.g. "snapshot-zfs"). The
	// host surfaces it alongside the spec so the frontend can resolve
	// icon assets via /api/plugins/loader/icon without a second
	// round-trip. Plugins populate this in RegisterAdapters.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory (resolved by the host's /api/plugins/loader/icon
	// route at render time). Empty string is valid — renderers fall
	// back to IconCamera.
	Icon string

	// Description is the operator-facing short description shown
	// under the tile label in the driver picker. Empty string is
	// valid — the tile renders without a subtitle.
	Description string

	// Capabilities describes which operations the adapter supports.
	// The host uses these to hide unsupported actions in the admin
	// UI and to short-circuit unsupported runtime calls with
	// ErrUnsupported.
	Capabilities SnapshotAdapterCapabilities

	// ConfigSchema is the list of form fields the admin UI renders
	// when an operator picks this driver. The host already knows
	// about the per-driver columns on container_backup_snapshot_configs
	// (pool_name, dataset_prefix, ssh_target, use_lxd); plugins
	// extend the schema by adding custom keys the host stores in
	// the same row's `config` JSON column.
	ConfigSchema []types.ConfigFieldDecl
}

// SnapshotAdapterCapabilities declares which operations the adapter
// supports. The host consults these before invoking the corresponding
// method on SnapshotAdapterClient.
type SnapshotAdapterCapabilities struct {
	// SupportsMount indicates the adapter can take a snapshot and
	// expose it at a stable read-only mount path the host's archive
	// builder reads from. Adapters that can only persist the
	// snapshot name (e.g. some NAS vendors) without exposing a
	// local mount path leave this false — the host will fall back
	// to the legacy `docker pause` path for plans targeting such a
	// driver. Future: a streaming SnapshotRead RPC can fill this gap.
	SupportsMount bool

	// SupportsRemoteTarget indicates the driver can take snapshots
	// on a different host than where the host runs (NAS targets,
	// SSH-reachable ZFS pools). Plugins that wrap CLI tools on the
	// local host only (RequiresAgent=true) leave this false.
	SupportsRemoteTarget bool

	// RequiresAgent indicates the adapter delegates Create/Remove to
	// the host's remote-agent transport (via the SnapshotRPCHandle
	// the host provides at Init time). Local-host CLI wrappers (zfs,
	// btrfs, lvm) declare RequiresAgent=true; HTTP-based NAS plugins
	// (synology, truenas) declare RequiresAgent=false and call the
	// vendor's REST API directly from the host process.
	RequiresAgent bool
}

// SnapshotAdapterClient is the runtime contract for plugin-provided
// snapshot handlers. The host calls these methods when a backup run
// targets a snapshot config whose driver matches the adapter's
// registered Driver.
//
// All methods receive a config map carrying the snapshot config's
// columns (pool_name, dataset_prefix, ssh_target, use_lxd, plus any
// plugin-owned JSON config keys) keyed by column name. Plugins
// decide which keys they need; the host does not pre-validate the
// map.
//
// Implementations MUST be safe for concurrent use. The host invokes
// Capability, Create, and Remove from parallel goroutines for
// multi-target plans and concurrent runs.
type SnapshotAdapterClient interface {
	// Capability probes the driver and returns the available snapshot
	// targets (datasets, subvolumes, LVs, shares, …). The host calls
	// this from the admin UI's "Test connection" button and from the
	// backup-pipeline startup phase to resolve which targets the
	// current run should snapshot. Implementations that wrap a
	// missing CLI tool should return Available=false with an Error
	// string rather than failing with a Go error — the host uses
	// Available=false as a soft signal to fall back.
	Capability(ctx context.Context, config map[string]string) (SnapshotCapability, error)

	// Create takes a snapshot of the named targets. The returned
	// SnapshotRef.SnapshotName is what Remove will see; the host
	// persists it on the backup run row so operators can audit which
	// snapshot backs each archive. ReadOnly is a hint: adapters that
	// support mounting the snapshot RO may set SnapshotRef.MountPath
	// to that path so the archive builder can read directly from the
	// frozen view (no docker pause). Timeout is in seconds; adapters
	// should auto-remove the snapshot if Timeout elapses before the
	// host calls Remove (defensive against orphaned snapshots).
	Create(ctx context.Context, config map[string]string, req SnapshotCreateRequest) (SnapshotRef, error)

	// Remove destroys the snapshot referenced by ref. The host calls
	// Remove from a deferred block after the archive completes (LIFO
	// order: snapshot Remove fires before container unpause). Plugins
	// MUST treat Remove as best-effort from the host's perspective —
	// a non-nil error is logged but does not fail the backup, since
	// the archive is already written and the operator would rather
	// have a leaked snapshot than a failed run. Auto-expiry via
	// Timeout covers the leak window.
	Remove(ctx context.Context, config map[string]string, ref SnapshotRef) error
}

// SnapshotCreateRequest is what the host hands to SnapshotAdapterClient.Create.
type SnapshotCreateRequest struct {
	// Targets is the list of snapshot targets (dataset names, subvol
	// paths, LV paths, share names, …) the host resolved from the
	// container's mount layout + the adapter's Capability reply.
	// Plugins take a snapshot of each; the returned SnapshotRef
	// carries the union.
	Targets []string

	// SnapshotID is the host's correlation token — typically the
	// backup run's xid. Plugins embed it in the snapshot name so
	// Remove can find the right one (and so operators auditing
	// `zfs list -t snapshot` can match a snapshot back to a run).
	SnapshotID string

	// ReadOnly is a hint: true requests the adapter mount the
	// snapshot read-only and stash the mount path in
	// SnapshotRef.MountPath so the archive builder can read directly
	// from the frozen view without `docker pause`. Adapters that
	// declared SupportsMount=false ignore this and return an empty
	// MountPath; the host falls back to the legacy pause path.
	ReadOnly bool

	// Timeout is the upper bound in seconds before the snapshot
	// should be auto-removed. Typical values are 3600 (1h) for
	// large backups. Plugins should honor this defensively — the
	// host's deferred Remove normally fires first, but a crashed
	// agent or process kill can leave the deferred block un-run.
	Timeout int
}

// SnapshotRef is what SnapshotAdapterClient.Create returns. The host
// persists the relevant fields on the backup run row so operators can
// audit which snapshot backed which archive.
type SnapshotRef struct {
	// SnapshotName is the driver-native snapshot identifier (e.g.
	// "tank/data/foo@mcharbor-abc123"). Plugins must make it unique
	// enough to be removable by Remove even after many concurrent
	// runs. The host embeds SnapshotID into the name it suggests
	// via SnapshotCreateRequest.SnapshotID; plugins typically use
	// that.
	SnapshotName string

	// Targets echoes the requested targets so Remove knows what
	// to clean up if the snapshot is dataset-scoped rather than
	// name-scoped (LVM thin snapshots, btrfs snapshots of multiple
	// subvols, …).
	Targets []string

	// MountPath is the read-only mount point the archive builder
	// should read from. Empty when SupportsMount=false or the
	// adapter couldn't mount (caller falls back to docker pause).
	MountPath string

	// CreatedAt is the moment the snapshot was taken on the
	// driver. The host stamps this on the backup run row.
	CreatedAt time.Time
}

// SnapshotCapability is what SnapshotAdapterClient.Capability returns.
// The host uses Available to decide whether to show this driver in
// the admin UI's picker, and uses Targets to populate the snapshot-
// config's "targets" list.
type SnapshotCapability struct {
	// Driver echoes the spec's Driver so log lines and UI rows can
	// render without a separate spec lookup.
	Driver string

	// ToolPath is the path of the CLI tool the adapter would
	// invoke (e.g. "/usr/sbin/zfs", "/usr/sbin/vgs"). Empty when
	// the adapter is HTTP-only.
	ToolPath string

	// Version is the driver / tool version string (e.g. "zfs-2.2.6",
	// "DSM 7.2-64570"). Empty when not applicable.
	Version string

	// Available reports whether the driver is reachable from the
	// host (CLI on PATH, HTTP target responds). The host uses this
	// as a soft signal — Available=false with a populated Error
	// means the operator hasn't installed / configured the driver
	// yet; the host surfaces the error string in the admin UI but
	// doesn't fail the save.
	Available bool

	// Targets is the list of snapshot-capable targets the driver
	// exposes. For zfs: datasets the host can see; for synology:
	// shares; for truenas: datasets.
	Targets []SnapshotTarget

	// Error is a human-readable description of why Available=false.
	// Empty when Available=true.
	Error string
}

// SnapshotTarget describes a single snapshot-capable target returned
// by SnapshotAdapterClient.Capability.
type SnapshotTarget struct {
	// Name is the driver-native identifier (dataset name, subvol
	// path, LV path, share name).
	Name string

	// Mountpoint is the filesystem path the target is currently
	// mounted at. Empty for targets that aren't mounted (zfs
	// datasets with mountpoint=none).
	Mountpoint string

	// UsedBytes is the target's current usage in bytes. Optional —
	// 0 means "unknown" (some HTTP-only adapters don't expose
	// usage cheaply).
	UsedBytes int64

	// Driver echoes the driver name so log lines and admin UI
	// rows can render without a separate lookup.
	Driver string
}
