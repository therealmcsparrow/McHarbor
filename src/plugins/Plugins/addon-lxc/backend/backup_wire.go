// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "encoding/json"

// agentBackupSpec is one container backup as the agent receives it.
//
// Declared here rather than imported: the host's shared compute spec is linked
// into every plugin binary, so a typed field there would take every installed
// plugin offline for the sake of a struct only this plugin sends. It mirrors the
// protocol's CTBackupSpec field for field, and the JSON tags are the contract.
type agentBackupSpec struct {
	RunID   string `json:"runId"`
	ChainID string `json:"chainId,omitempty"`
	Kind    string `json:"kind"`

	RootfsPath  string `json:"rootfsPath"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`

	Manifest    json.RawMessage `json:"manifest,omitempty"`
	ListingName string          `json:"listingName,omitempty"`

	Upload     *agentBackupUpload `json:"upload,omitempty"`
	ScratchDir string             `json:"scratchDir,omitempty"`
}

// agentBackupUpload points the agent back at McHarbor rather than at the real
// destination, so no storage credential ever reaches a remote host.
type agentBackupUpload struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// agentBackupResult is what the agent actually did.
//
// FellBackToFull is the field this feature's correctness rests on: an
// incremental taken against a lost listing restores a rootfs missing every file
// changed since the listing went, so a downgrade is reported and this plugin
// records a full run.
type agentBackupResult struct {
	Kind           string `json:"kind"`
	BytesWritten   int64  `json:"bytesWritten"`
	ListingName    string `json:"listingName,omitempty"`
	ListingPresent bool   `json:"listingPresent"`
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	Consistency    string `json:"consistency,omitempty"`
	// IDShift is the restore's acknowledgement: "none" or "shifted" when the agent
	// handled ownership, empty from an agent that predates re-owning.
	IDShift string `json:"idShift,omitempty"`
	Error   string `json:"error,omitempty"`
}

// agentRestoreSpec rebuilds a container's rootfs from a chain.
type agentRestoreSpec struct {
	RunID       string `json:"runId"`
	TargetPath  string `json:"targetPath"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`

	Artifacts  []agentRestoreArtifact `json:"artifacts,omitempty"`
	ScratchDir string                 `json:"scratchDir,omitempty"`

	// SourceIDMap is the range the backup was taken in, nil when it was never
	// recorded; the agent then reads it off the tree or refuses. TargetIDMap is
	// the range of the container restored into, and the tree is re-owned into it.
	SourceIDMap *backupIDMap `json:"sourceIdMap,omitempty"`
	TargetIDMap *backupIDMap `json:"targetIdMap,omitempty"`
}

// agentRestoreArtifact is one downloadable link of a chain.
//
// Sequence orders it. Applying an incremental before the full it builds on, or
// skipping a link, produces a rootfs that looks intact and is not, so the order
// is explicit rather than implied by the slice.
type agentRestoreArtifact struct {
	Part      string `json:"part"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	Sequence  int    `json:"sequence"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// backupManifest is the container's definition, written into the artifact.
//
// Its purpose is a restore into a container McHarbor no longer has a row for:
// the rootfs alone does not say what init it runs, how it was id-mapped or what
// limits it had, and guessing any of those produces a container that boots
// wrong rather than one that does not boot.
type backupManifest struct {
	Version       int     `json:"version"`
	ContainerID   string  `json:"containerId"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	EnvironmentID string  `json:"environmentId"`
	OSFamily      string  `json:"osFamily,omitempty"`
	OSVersion     string  `json:"osVersion,omitempty"`
	InitSystem    string  `json:"initSystem"`
	Privileged    bool    `json:"privileged"`
	Nesting       bool    `json:"nesting"`
	PoolBackend   string  `json:"poolBackend"`
	RootfsPath    string  `json:"rootfsPath"`
	Limits        Limits  `json:"limits"`
	NICs          []NIC   `json:"nics,omitempty"`
	Mounts        []Mount `json:"mounts,omitempty"`
	TakenAt       string  `json:"takenAt"`
	// IDMap is the id range the rootfs was archived in. The archive holds host
	// ids, so a restore into another range has to know it; absent from manifests
	// written before 2026.09.12, which is why the field is optional.
	IDMap *backupIDMap `json:"idMap,omitempty"`
}
