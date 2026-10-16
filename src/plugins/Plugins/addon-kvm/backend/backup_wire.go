// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// agentBackupSpec is one backup run as the agent receives it.
//
// Declared here rather than imported: the host's shared compute spec is linked
// into every plugin binary, so a typed field there would take every installed
// plugin offline for the sake of a struct only this plugin sends.
type agentBackupSpec struct {
	RunID      string             `json:"runId"`
	ChainID    string             `json:"chainId,omitempty"`
	Kind       string             `json:"kind"`
	Quiesce    bool               `json:"quiesce,omitempty"`
	Disks      []agentBackupDisk  `json:"disks,omitempty"`
	Upload     *agentBackupUpload `json:"upload,omitempty"`
	ScratchDir string             `json:"scratchDir,omitempty"`
}

// agentBackupDisk is one disk to copy.
type agentBackupDisk struct {
	DiskID     string `json:"diskId"`
	VolumePath string `json:"volumePath"`
	NodeName   string `json:"nodeName,omitempty"`
	BitmapName string `json:"bitmapName,omitempty"`
}

// agentBackupUpload points the agent back at McHarbor rather than at the real
// destination, so no storage credential ever reaches a remote host.
type agentBackupUpload struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// agentBackupDiskResult is what the agent actually did to one disk.
//
// FellBackToFull is the field this whole feature's correctness rests on: an
// incremental taken against a lost bitmap restores to a corrupt disk, so a
// downgrade is reported and McHarbor records a full run.
type agentBackupDiskResult struct {
	DiskID         string `json:"diskId"`
	Kind           string `json:"kind"`
	BytesWritten   int64  `json:"bytesWritten"`
	BitmapName     string `json:"bitmapName,omitempty"`
	BitmapPresent  bool   `json:"bitmapPresent"`
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	Consistency    string `json:"consistency,omitempty"`
	Error          string `json:"error,omitempty"`
}

// agentRestoreSpec rebuilds a machine's disks from a chain.
type agentRestoreSpec struct {
	RunID      string                  `json:"runId"`
	Disks      []agentRestoreDisk      `json:"disks,omitempty"`
	Artifacts  []agentRestoreArtifact  `json:"artifacts,omitempty"`
	ScratchDir string                  `json:"scratchDir,omitempty"`
}

// agentRestoreDisk is one disk to rebuild.
type agentRestoreDisk struct {
	DiskID     string `json:"diskId"`
	VolumePath string `json:"volumePath"`
}

// agentRestoreArtifact is one downloadable link of a chain.
//
// Sequence orders it. Applying a chain out of order silently produces a disk
// that looks intact and is not, so the order is explicit rather than implied
// by the slice.
type agentRestoreArtifact struct {
	DiskID   string `json:"diskId"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Token    string `json:"token"`
	Sequence  int   `json:"sequence"`
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}
