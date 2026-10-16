// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"time"
)

// The plugin's own row shapes.
//
// These are separate from the compute model (compute_model.go) and the agent's
// wire structs (wire_agent.go) on purpose. Those describe what a host reports;
// these describe what is stored. A
// capability report, for instance, travels as a structured value and is stored
// as JSON, because it is a snapshot of someone else's machine whose shape
// follows the agent's and would otherwise need a migration per new check.

// ComputeEnvironment is one host's compute settings.
type ComputeEnvironment struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	// Name and ConnectionType are joined from the host's environments table for
	// display. They are never written back — the environment belongs to the
	// host, not to this plugin.
	Name           string `json:"name,omitempty"`
	ConnectionType string `json:"connectionType,omitempty"`

	ComputeEnabled bool `json:"computeEnabled"`
	// Capability is the last probe. Nil when the host has never been probed,
	// which the UI shows as "not yet checked" rather than as a failure.
	Capability *Capability `json:"capability,omitempty"`
	ProbedAt   *time.Time  `json:"probedAt,omitempty"`

	DefaultPoolID    string `json:"defaultPoolId,omitempty"`
	DefaultNetworkID string `json:"defaultNetworkId,omitempty"`

	// SharedLibrary says this host's agent mounts the operator's image library
	// on its own cache root, so a file downloaded once is already present here.
	// It is the operator's statement about their storage, not a probe: McHarbor
	// cannot see the far side of that mount from this process.
	SharedLibrary bool `json:"sharedLibrary"`

	// Available and UnavailableReason report whether the agent can be reached
	// right now. They are computed per request rather than stored, because a
	// stored answer would be a guess about the present.
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailableReason,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Pool is one storage pool row.
type Pool struct {
	ID             string `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	Name           string `json:"name"`
	Backend        string `json:"backend"`
	Target         string `json:"target"`
	IsDefault      bool   `json:"isDefault"`
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int64  `json:"usedBytes"`
	AllocatedBytes int64  `json:"allocatedBytes"`
	VolumeCount    int    `json:"volumeCount"`
	// Instant is derived from the backend rather than stored: it is a property
	// of zfs, btrfs and lvm, not of this particular pool, and duplicating it
	// into a column would let the two disagree.
	Instant   bool       `json:"instant"`
	SampledAt *time.Time `json:"sampledAt,omitempty"`
	State     string     `json:"state"`
	LastError string     `json:"lastError,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// InstantBackends are the pool backends whose clones and snapshots are metadata
// operations.
//
// This is the single most consequential choice an operator makes in the setup
// wizard: on one of these, "create ten containers from this template" is under
// a second, and guest snapshots reuse driver code that already exists and is
// already exercised in production.
var InstantBackends = map[string]bool{
	BackendZFS:   true,
	BackendBtrfs: true,
	BackendLVM:   true,
	BackendDir:   false,
}

// Network is one guest network row.
//
// The direct mode — macvlan — carries no bridge: its guests stack on
// UplinkLink itself, and LinkMode says how.
type Network struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Bridge        string `json:"bridge"`
	VLANTag       int    `json:"vlanTag,omitempty"`

	SubnetCIDR     string `json:"subnetCidr,omitempty"`
	GatewayIP      string `json:"gatewayIp,omitempty"`
	DHCPEnabled    bool   `json:"dhcpEnabled"`
	DHCPRangeStart string `json:"dhcpRangeStart,omitempty"`
	DHCPRangeEnd   string `json:"dhcpRangeEnd,omitempty"`
	NATEnabled     bool   `json:"natEnabled"`
	UplinkLink     string `json:"uplinkLink,omitempty"`

	LinkMode     string `json:"linkMode,omitempty"`
	HostAccess   bool   `json:"hostAccess"`
	HostAccessIP string `json:"hostAccessIp,omitempty"`

	// IPv6Enabled turns IPv6 on for the host shim; HostAccessIPv6 is its static
	// address, and empty means the shim takes one by SLAAC. HostLinkName is the
	// operator's name for the shim, empty for the generated default.
	IPv6Enabled    bool   `json:"ipv6Enabled"`
	HostAccessIPv6 string `json:"hostAccessIpv6,omitempty"`
	HostLinkName   string `json:"hostLinkName,omitempty"`

	IsDefault bool      `json:"isDefault"`
	State     string    `json:"state"`
	LastError string    `json:"lastError,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Lease is one DHCP lease row.
type Lease struct {
	ID        string     `json:"id"`
	NetworkID string     `json:"networkId"`
	MAC       string     `json:"mac"`
	IP        string     `json:"ip"`
	Hostname  string     `json:"hostname,omitempty"`
	GuestKind string     `json:"guestKind,omitempty"`
	GuestID   string     `json:"guestId,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Image is one catalog entry.
type Image struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Distro       string `json:"distro"`
	Release      string `json:"release"`
	Arch         string `json:"arch"`
	Variant      string `json:"variant,omitempty"`
	DisplayName  string `json:"displayName"`
	SourceURL    string `json:"sourceUrl"`
	ChecksumURL  string `json:"checksumUrl,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
	ChecksumAlgo string `json:"checksumAlgo"`
	SignatureURL string `json:"signatureUrl,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	// IsBuiltin entries cannot be deleted — only their cached copies can be
	// evicted. They may ship without a checksum, and then download on the same
	// acknowledgement any unverified image needs.
	IsBuiltin       bool   `json:"isBuiltin"`
	InitSystem      string `json:"initSystem,omitempty"`
	NetConfigFormat string `json:"netConfigFormat,omitempty"`

	// Origin separates an entry fetched from a mirror from one the operator
	// uploaded. An upload has no SourceURL at all, so the two are not variants
	// of one flow and the UI does not pretend they are.
	Origin string `json:"origin"`
	// LibraryFile is the upload's name inside the library directory — never a
	// path, because the directory is resolved from configuration at read time.
	LibraryFile string     `json:"libraryFile,omitempty"`
	UploadedBy  string     `json:"uploadedBy,omitempty"`
	UploadedAt  *time.Time `json:"uploadedAt,omitempty"`

	// Cache is this image's per-environment download state, populated when a
	// listing is scoped to one environment.
	Cache *ImageCache `json:"cache,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ImageCache is one image's state on one host.
type ImageCache struct {
	ID            string `json:"id"`
	ImageID       string `json:"imageId"`
	EnvironmentID string `json:"environmentId"`
	LocalPath     string `json:"localPath"`
	SizeBytes     int64  `json:"sizeBytes,omitempty"`
	Verified      bool   `json:"verified"`
	Signed        bool   `json:"signed"`
	State         string `json:"state"`
	ProgressPct   int    `json:"progressPct"`
	// Phase is what the host is doing now — downloading, verifying,
	// decompressing, installing. The percentage counts bytes off the wire and
	// sits at 100 through the last three, which on a compressed image is most
	// of the wait.
	Phase      string     `json:"phase,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// Image cache states.
const (
	CacheStatePending     = "pending"
	CacheStateDownloading = "downloading"
	CacheStateReady       = "ready"
	CacheStateFailed      = "failed"
)

// Object states shared by pools and networks.
const (
	StateUnknown = "unknown"
	StateReady   = "ready"
	StateError   = "error"
)

// Overview is the landing page's per-host summary, assembled in one call so
// the page is one request rather than four per host.
type Overview struct {
	Environments []OverviewEntry `json:"environments"`
}

// OverviewEntry is one host on the overview.
type OverviewEntry struct {
	EnvironmentID  string `json:"environmentId"`
	Name           string `json:"name"`
	ComputeEnabled bool   `json:"computeEnabled"`
	Available      bool   `json:"available"`
	// UnavailableReason distinguishes the three failure modes — not an agent
	// environment, agent offline, agent too old — because each has a different
	// fix and collapsing them would leave the operator without the one thing
	// they need to know.
	UnavailableReason string      `json:"unavailableReason,omitempty"`
	Capability        *Capability `json:"capability,omitempty"`
	Pools             []Pool      `json:"pools,omitempty"`
	Networks          []Network   `json:"networks,omitempty"`
	CachedImages      int         `json:"cachedImages"`
	// SetupComplete drives the wizard's entry point: a host with compute on,
	// a pool and a network is ready for guests.
	SetupComplete bool `json:"setupComplete"`
	// SharedLibrary mirrors the environment's flag so the card can show it
	// without a second request per host.
	SharedLibrary bool `json:"sharedLibrary"`
}

// PoolCreateRequest is the create dialog's body.
type PoolCreateRequest struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Backend       string `json:"backend"`
	Target        string `json:"target"`
	IsDefault     bool   `json:"isDefault"`
}

// PoolEnsureRequest asks for the environment's pool, creating a directory pool
// at Target when there is none.
type PoolEnsureRequest struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name,omitempty"`
	Target        string `json:"target,omitempty"`
}

// NetworkCreateRequest is the network create dialog's body.
type NetworkCreateRequest struct {
	EnvironmentID  string `json:"environmentId"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	Bridge         string `json:"bridge"`
	VLANTag        int    `json:"vlanTag,omitempty"`
	SubnetCIDR     string `json:"subnetCidr,omitempty"`
	GatewayIP      string `json:"gatewayIp,omitempty"`
	DHCPEnabled    bool   `json:"dhcpEnabled,omitempty"`
	DHCPRangeStart string `json:"dhcpRangeStart,omitempty"`
	DHCPRangeEnd   string `json:"dhcpRangeEnd,omitempty"`
	NATEnabled     bool   `json:"natEnabled,omitempty"`
	UplinkLink     string `json:"uplinkLink,omitempty"`
	LinkMode       string `json:"linkMode,omitempty"`
	HostAccess     bool   `json:"hostAccess,omitempty"`
	HostAccessIP   string `json:"hostAccessIp,omitempty"`
	IPv6Enabled    bool   `json:"ipv6Enabled,omitempty"`
	HostAccessIPv6 string `json:"hostAccessIpv6,omitempty"`
	HostLinkName   string `json:"hostLinkName,omitempty"`
	IsDefault      bool   `json:"isDefault,omitempty"`
}

// ImageCreateRequest adds an operator-supplied catalog entry.
type ImageCreateRequest struct {
	Kind         string `json:"kind"`
	Distro       string `json:"distro"`
	Release      string `json:"release"`
	Arch         string `json:"arch"`
	DisplayName  string `json:"displayName"`
	SourceURL    string `json:"sourceUrl"`
	ChecksumURL  string `json:"checksumUrl,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
	ChecksumAlgo string `json:"checksumAlgo,omitempty"`
	SignatureURL string `json:"signatureUrl,omitempty"`
	InitSystem   string `json:"initSystem,omitempty"`
}

// Image origins.
const (
	// OriginCatalog is fetched from SourceURL by the host itself.
	OriginCatalog = "catalog"
	// OriginUpload is adopted from the shared image library.
	OriginUpload = "upload"
)

// ImageUploadRequest is the metadata beside an uploaded file.
//
// Everything here is descriptive. The checksum is not accepted from the client
// under any name: McHarbor computes it from the bytes it receives, which is the
// only version of it that proves anything.
type ImageUploadRequest struct {
	Kind        string `json:"kind"`
	Distro      string `json:"distro"`
	Release     string `json:"release"`
	Arch        string `json:"arch"`
	DisplayName string `json:"displayName"`
	Variant     string `json:"variant,omitempty"`
	InitSystem  string `json:"initSystem,omitempty"`
	// FileName is the browser's filename, kept for its extension.
	FileName string `json:"fileName"`
}

// ImageFetchRequest starts a download on one host.
type ImageFetchRequest struct {
	EnvironmentID string `json:"environmentId"`
	// AcknowledgeUnverified is the operator's explicit acceptance that this
	// source publishes no checksum. It is honoured only for a non-builtin
	// entry, and the acknowledgement is written into the audit row so the
	// decision is attributable later.
	AcknowledgeUnverified bool `json:"acknowledgeUnverified,omitempty"`
}

// EnvironmentUpdateRequest turns compute on or off for a host.
type EnvironmentUpdateRequest struct {
	ComputeEnabled   *bool  `json:"computeEnabled,omitempty"`
	DefaultPoolID    string `json:"defaultPoolId,omitempty"`
	DefaultNetworkID string `json:"defaultNetworkId,omitempty"`
	SharedLibrary    *bool  `json:"sharedLibrary,omitempty"`
}

// ImageFetchAllResult reports what a fan-out download decided, per host.
//
// Skipped carries a reason per environment rather than a count, because "why
// did that host not get it" is the only question anyone asks of this.
type ImageFetchAllResult struct {
	ImageID string            `json:"imageId"`
	Started []string          `json:"started,omitempty"`
	Skipped map[string]string `json:"skipped,omitempty"`
	// SharedWith names the hosts a single shared download covers.
	SharedWith []string `json:"sharedWith,omitempty"`
}
