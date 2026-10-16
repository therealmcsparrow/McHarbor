// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "time"

// Backup run operations and outcomes.
const (
	BackupOpBackup  = "backup"
	BackupOpRestore = "restore"

	BackupStatusRunning   = "running"
	BackupStatusSuccess   = "success"
	BackupStatusFailure   = "failure"
	BackupStatusCancelled = "cancelled"

	BackupKindFull        = "full"
	BackupKindIncremental = "incremental"
)

// How a rootfs was read. The agent refuses a live rootfs it can neither snapshot
// nor freeze; the one exception is a dir pool's unfrozen tree copy, recorded as
// BackupConsistencyLiveCopy because it is not point-in-time.
const (
	BackupConsistencySnapshot   = "snapshot"
	BackupConsistencyFrozen     = "frozen"
	BackupConsistencyFilesystem = "filesystem"
	BackupConsistencyLiveCopy   = "live-copy"
)

// Destination upload states, per run per destination per part.
const (
	DestinationUploading = "uploading"
	DestinationSuccess   = "success"
	DestinationFailure   = "failure"
)

// The two halves of one artifact. The manifest is the container's definition;
// the rootfs is the tar. Both are needed to rebuild a container McHarbor no
// longer holds a row for.
const (
	BackupPartManifest = "manifest"
	BackupPartRootfs   = "rootfs"
)

// BackupPlan is one container's backup definition, schedule and retention.
type BackupPlan struct {
	ID          string `json:"id"`
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	Cron        string `json:"cron,omitempty"`
	Enabled     bool   `json:"enabled"`

	RetentionCount int `json:"retentionCount"`
	RetentionDays  int `json:"retentionDays"`
	// FullEvery forces a full copy every N runs. However well the chain logic
	// works, a chain that never restarts is one lost listing away from a very
	// long restore — so this is a floor, not a tuning knob.
	FullEvery int `json:"fullEvery"`

	DestinationIDs []string `json:"destinationIds"`

	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// BackupRun is one execution — a backup or a restore.
//
// Both live in one table, keyed by Operation, exactly as the machine module
// does it. History, progress and polling are then one implementation rather
// than two that drift apart.
type BackupRun struct {
	ID            string `json:"id"`
	PlanID        string `json:"planId,omitempty"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName,omitempty"`
	EnvironmentID string `json:"environmentId"`
	Operation     string `json:"operation"`
	SourceRunID   string `json:"sourceRunId,omitempty"`
	Status        string `json:"status"`

	// Kind is what was actually taken, never what was requested. The agent
	// decides, because only it can see whether the listing survived.
	Kind    string `json:"kind"`
	ChainID string `json:"chainId,omitempty"`
	// FellBackToFull and FallbackReason explain why tonight's "incremental"
	// moved the whole rootfs. Without them the operator sees only a slow run.
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	Consistency    string `json:"consistency"`

	BytesTotal int64 `json:"bytesTotal"`
	// TargetContainerID is where a restore put the rootfs. Set only on a
	// restore, and different from ContainerID when the operator restored into
	// a new container rather than over the original.
	TargetContainerID string `json:"targetContainerId,omitempty"`
	// IDShift is what the agent did with file owners on a restore: "none",
	// "shifted" or "skipped". Empty on a backup, and on a restore the agent never
	// answered or that predates the field.
	IDShift string `json:"idShift,omitempty"`

	Error string `json:"error,omitempty"`
	// ErrorHint names a known fix for Error, such as "backup_key_missing", so
	// the interface can point at the page that resolves it.
	ErrorHint         string `json:"errorHint,omitempty"`
	ProgressStage     string `json:"progressStage,omitempty"`
	ProgressMessage   string `json:"progressMessage,omitempty"`
	ProgressUpdatedAt string `json:"progressUpdatedAt,omitempty"`

	Destinations []BackupRunDestination `json:"destinations"`

	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DurationMS  int64      `json:"durationMs"`
}

// BackupRunDestination is one destination's outcome for one part of one run.
type BackupRunDestination struct {
	ID            string     `json:"id"`
	RunID         string     `json:"runId"`
	LocationID    string     `json:"locationId"`
	LocationName  string     `json:"locationName,omitempty"`
	Part          string     `json:"part"`
	Status        string     `json:"status"`
	RemotePath    string     `json:"remotePath,omitempty"`
	KeyID         string     `json:"keyId,omitempty"`
	BytesUploaded int64      `json:"bytesUploaded"`
	Error         string     `json:"error,omitempty"`
	UploadedAt    *time.Time `json:"uploadedAt,omitempty"`
}

// BackupChain is one container's incremental lineage.
//
// Invalidated is the field the restore path trusts. A chain whose listing was
// lost, or whose rootfs was reverted or restored underneath it, cannot produce
// a correct incremental — and an incremental taken against it restores a tree
// that looks intact and is missing everything changed since.
type BackupChain struct {
	ID          string     `json:"id"`
	ContainerID string     `json:"containerId"`
	PlanID      string     `json:"planId,omitempty"`
	ListingName string     `json:"listingName"`
	LastRunID   string     `json:"lastRunId,omitempty"`
	LastFullAt  *time.Time `json:"lastFullAt,omitempty"`
	LastRunAt   *time.Time `json:"lastRunAt,omitempty"`
	Invalidated bool       `json:"invalidated"`
	InvalidWhy  string     `json:"invalidWhy,omitempty"`
}

// CreateBackupPlanInput is the create form's body.
type CreateBackupPlanInput struct {
	ContainerID    string   `json:"containerId"`
	Name           string   `json:"name"`
	Cron           string   `json:"cron,omitempty"`
	Enabled        bool     `json:"enabled"`
	RetentionCount int      `json:"retentionCount"`
	RetentionDays  int      `json:"retentionDays"`
	FullEvery      int      `json:"fullEvery"`
	DestinationIDs []string `json:"destinationIds"`
}

// UpdateBackupPlanInput patches a plan. Every field is a pointer so an absent
// one means "leave it" rather than "set it to the zero value".
type UpdateBackupPlanInput struct {
	Name           *string   `json:"name,omitempty"`
	Cron           *string   `json:"cron,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
	RetentionCount *int      `json:"retentionCount,omitempty"`
	RetentionDays  *int      `json:"retentionDays,omitempty"`
	FullEvery      *int      `json:"fullEvery,omitempty"`
	DestinationIDs *[]string `json:"destinationIds,omitempty"`
}

// RunBackupInput starts an ad-hoc run.
type RunBackupInput struct {
	// Kind may be left empty to let the chain decide, or forced to full when an
	// operator wants a fresh baseline without waiting for FullEvery.
	Kind string `json:"kind,omitempty"`
}

// RestoreBackupInput restores one run into a container's rootfs.
type RestoreBackupInput struct {
	// TargetContainerID restores into a different container. Empty means the
	// one the backup came from, which is the common case; naming another is how
	// a backup of a destroyed container is brought back.
	TargetContainerID string `json:"targetContainerId,omitempty"`
	// ConfirmName is the container's name typed out. A restore replaces a whole
	// filesystem and cannot be undone, so a tick box is not enough.
	ConfirmName string `json:"confirmName"`
}
