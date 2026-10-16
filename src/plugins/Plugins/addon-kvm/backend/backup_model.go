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

// Destination upload states, per run per destination.
const (
	DestinationUploading = "uploading"
	DestinationSuccess   = "success"
	DestinationFailure   = "failure"
)

// BackupPlan is one machine's backup definition, schedule and retention.
//
// One row rather than three tables, mirroring container_backup_plans: an
// operator thinks of "the nightly backup of this machine" as one thing, and
// splitting it would mean three round trips to answer one question.
type BackupPlan struct {
	ID      string `json:"id"`
	VMID    string `json:"vmId"`
	Name    string `json:"name"`
	Cron    string `json:"cron,omitempty"`
	Enabled bool   `json:"enabled"`

	RetentionCount int `json:"retentionCount"`
	RetentionDays  int `json:"retentionDays"`
	// FullEvery forces a full copy every N runs. However well the chain logic
	// works, a chain that never restarts is one lost bitmap away from a very
	// long restore — so this is a floor, not a tuning knob.
	FullEvery int `json:"fullEvery"`
	// Quiesce freezes the guest's filesystems for the copy where the guest
	// agent answers. Off means crash-consistent, which the run records.
	Quiesce bool `json:"quiesce"`

	DestinationIDs []string `json:"destinationIds"`

	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// BackupRun is one execution — a backup or a restore.
//
// Both live in one table, keyed by Operation, exactly as the container module
// does it. History, progress, cancellation and polling are then one
// implementation rather than two that drift apart.
type BackupRun struct {
	ID            string `json:"id"`
	PlanID        string `json:"planId,omitempty"`
	VMID          string `json:"vmId"`
	EnvironmentID string `json:"environmentId"`
	Operation     string `json:"operation"`
	SourceRunID   string `json:"sourceRunId,omitempty"`
	Status        string `json:"status"`

	// Kind is what was actually taken, never what was requested. The agent
	// decides, because only it can see whether the dirty bitmap survived.
	Kind    string `json:"kind"`
	ChainID string `json:"chainId,omitempty"`
	// FellBackToFull and FallbackReason explain why tonight's "incremental"
	// moved the whole disk. Without them the operator sees only a slow run.
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	Consistency    string `json:"consistency"`

	BytesTotal    int64    `json:"bytesTotal"`
	IncludedDisks []string `json:"includedDisks,omitempty"`

	Error             string `json:"error,omitempty"`
	ProgressStage     string `json:"progressStage,omitempty"`
	ProgressMessage   string `json:"progressMessage,omitempty"`
	ProgressUpdatedAt string `json:"progressUpdatedAt,omitempty"`

	Destinations []BackupRunDestination `json:"destinations"`

	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DurationMS  int64      `json:"durationMs"`
}

// BackupRunDestination is one destination's outcome for one run.
//
// Per destination and per disk, because a run that reached two of three
// destinations is a partial success and the operator has to see which one to
// retry rather than a single verdict for the whole run.
type BackupRunDestination struct {
	ID            string     `json:"id"`
	RunID         string     `json:"runId"`
	LocationID    string     `json:"locationId"`
	LocationName  string     `json:"locationName,omitempty"`
	DiskID        string     `json:"diskId,omitempty"`
	Status        string     `json:"status"`
	RemotePath    string     `json:"remotePath,omitempty"`
	KeyID         string     `json:"keyId,omitempty"`
	BytesUploaded int64      `json:"bytesUploaded"`
	Error         string     `json:"error,omitempty"`
	UploadedAt    *time.Time `json:"uploadedAt,omitempty"`
}

// BackupChain is one disk's incremental lineage.
//
// Invalidated is the field the restore path trusts. A chain whose bitmap was
// lost, or whose disk was resized or reverted underneath it, cannot produce a
// correct incremental — and an incremental taken against it restores to a
// disk that looks intact and is not.
type BackupChain struct {
	ID          string     `json:"id"`
	VMID        string     `json:"vmId"`
	DiskID      string     `json:"diskId"`
	PlanID      string     `json:"planId,omitempty"`
	BitmapName  string     `json:"bitmapName"`
	Granularity int64      `json:"granularity,omitempty"`
	LastRunID   string     `json:"lastRunId,omitempty"`
	LastFullAt  *time.Time `json:"lastFullAt,omitempty"`
	LastRunAt   *time.Time `json:"lastRunAt,omitempty"`
	Invalidated bool       `json:"invalidated"`
	InvalidWhy  string     `json:"invalidWhy,omitempty"`
}

// CreateBackupPlanInput is the create form's body.
type CreateBackupPlanInput struct {
	VMID           string   `json:"vmId"`
	Name           string   `json:"name"`
	Cron           string   `json:"cron,omitempty"`
	Enabled        bool     `json:"enabled"`
	RetentionCount int      `json:"retentionCount"`
	RetentionDays  int      `json:"retentionDays"`
	FullEvery      int      `json:"fullEvery"`
	Quiesce        bool     `json:"quiesce"`
	DestinationIDs []string `json:"destinationIds"`
}

// UpdateBackupPlanInput patches a plan. Every field is a pointer so an absent
// one means "leave it" rather than "set it to the zero value" — the same
// PATCH semantics the container plans use.
type UpdateBackupPlanInput struct {
	Name           *string   `json:"name,omitempty"`
	Cron           *string   `json:"cron,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
	RetentionCount *int      `json:"retentionCount,omitempty"`
	RetentionDays  *int      `json:"retentionDays,omitempty"`
	FullEvery      *int      `json:"fullEvery,omitempty"`
	Quiesce        *bool     `json:"quiesce,omitempty"`
	DestinationIDs *[]string `json:"destinationIds,omitempty"`
}

// RunBackupInput starts an ad-hoc run.
type RunBackupInput struct {
	// Kind may be left empty to let the chain decide, or forced to full when
	// an operator wants a fresh baseline without waiting for FullEvery.
	Kind string `json:"kind,omitempty"`
}

// RestoreBackupInput restores one run into its machine's disks.
type RestoreBackupInput struct {
	// DiskIDs restores a subset. Empty means every disk the run covered.
	DiskIDs []string `json:"diskIds,omitempty"`
	// Confirm is the operator saying they understand the machine's current
	// disks are replaced. A restore with no explicit consent is refused.
	Confirm bool `json:"confirm"`
}
