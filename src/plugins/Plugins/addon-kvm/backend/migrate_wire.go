// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// Wire shapes for migration and the remote-viewer ticket.
//
// They mirror VMMigrateSpec, VMMigrateDisk, VMMigrateTLS and VMSpiceTicket in
// the agent protocol without importing it, the way agentNIC and the rest of
// agentPayload do: the host carries the JSON through GuestCommand untouched,
// so adding these never changes a spec struct every installed plugin links.

// Message types this file adds to the plugin's vocabulary.
const (
	msgVMMigrate     = "vm_migrate"
	msgVMSpiceTicket = "vm_spice_ticket"
	msgVMCancel      = "vm_cancel"
)

// Migration phases, one host's half each.
const (
	migratePhaseSend        = "send"
	migratePhaseReceive     = "receive"
	migratePhaseLiveCheck   = "live_check"
	migratePhaseLivePrepare = "live_prepare"
	migratePhaseLiveSend    = "live_send"
	migratePhaseLiveFinish  = "live_finish"
	migratePhaseAbort       = "abort"
)

// Agent feature keys, carried in QEMUCapability.Features beside the QEMU ones.
// Q18 keeps the agent at one version, so the agent says what it can do instead
// of McHarbor inferring it from a version gate.
const (
	FeatureSPICE        = "spice"
	FeatureClone        = "clone"
	FeatureMigrateCold  = "migrate_cold"
	FeatureMigrateLive  = "migrate_live"
	FeatureSerialResize = "serial_resize"
)

// migrateNVRAMDiskID names the UEFI variable store in a migration's disk list;
// both agents derive its path from the guest id.
const migrateNVRAMDiskID = "nvram"

type agentMigrate struct {
	TargetHost     string             `json:"targetHost,omitempty"`
	TargetPort     int                `json:"targetPort,omitempty"`
	Live           bool               `json:"live,omitempty"`
	MaxBandwidthMB int64              `json:"maxBandwidthMb,omitempty"`
	MaxDowntimeMS  int64              `json:"maxDowntimeMs,omitempty"`
	Phase          string             `json:"phase,omitempty"`
	MigrationID    string             `json:"migrationId,omitempty"`
	Token          string             `json:"token,omitempty"`
	Disks          []agentMigrateDisk `json:"disks,omitempty"`
	TLS            *agentMigrateTLS   `json:"tls,omitempty"`
	NBDPort        int                `json:"nbdPort,omitempty"`
	SPICE          bool               `json:"spice,omitempty"`
	HotplugLayout  bool               `json:"hotplugLayout,omitempty"`
	ListenAddrs    []string           `json:"listenAddrs,omitempty"`
	BytesMoved     int64              `json:"bytesMoved,omitempty"`
}

type agentMigrateDisk struct {
	DiskID       string `json:"diskId"`
	Slot         int    `json:"slot"`
	SourcePath   string `json:"sourcePath,omitempty"`
	Format       string `json:"format,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	PoolID       string `json:"poolId,omitempty"`
	PoolBackend  string `json:"poolBackend,omitempty"`
	PoolTarget   string `json:"poolTarget,omitempty"`
	URL          string `json:"url,omitempty"`
	Token        string `json:"token,omitempty"`
	VolumePath   string `json:"volumePath,omitempty"`
	TargetFormat string `json:"targetFormat,omitempty"`
}

type agentMigrateTLS struct {
	CACert     string `json:"caCert"`
	Cert       string `json:"cert"`
	Key        string `json:"key"`
	ServerName string `json:"serverName,omitempty"`
}

type agentSpiceTicket struct {
	Port      int      `json:"port"`
	Password  string   `json:"password"`
	Addresses []string `json:"addresses,omitempty"`
	ExpiresAt string   `json:"expiresAt,omitempty"`
}
