// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

// BackupPlan stores manual or scheduled backup configuration for one container.
type BackupPlan struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	// ContainerIDStale is true when the server refreshed the
	// container_id on this read because the stored id no longer
	// matched a live container (e.g. Docker reassigned the id on
	// recreate). The persisted row has already been updated by the
	// time the flag is set, so the UI just shows a small badge.
	ContainerIDStale   bool                       `json:"containerIdStale,omitempty"`
	StorageLocationID  string                     `json:"storageLocationId,omitempty"`
	StorageLocationIDs []string                   `json:"storageLocationIds"`
	IncludeConfig      bool                       `json:"includeConfig"`
	IncludeLogs        bool                       `json:"includeLogs"`
	IncludeFilesystem  bool                       `json:"includeFilesystem"`
	IncludeImage       bool                       `json:"includeImage"`
	SelectedMounts     []string                   `json:"selectedMounts"`
	Selections         map[string]BackupSelection `json:"selections,omitempty"`
	LogTailLines       int                        `json:"logTailLines"`
	Cron               string                     `json:"cron,omitempty"`
	Enabled            bool                       `json:"enabled"`
	RetentionCount     int                        `json:"retentionCount"`
	RetentionDays      int                        `json:"retentionDays"`
	LastRunAt          string                     `json:"lastRunAt,omitempty"`
	NextRunAt          string                     `json:"nextRunAt,omitempty"`
	ConsistencyMode    string                     `json:"consistencyMode,omitempty"`
	SnapshotConfigID   string                     `json:"snapshotConfigId,omitempty"`
	// SnapshotConfigName is joined from container_backup_snapshot_configs,
	// empty unless SnapshotConfigID names a config that still exists.
	SnapshotConfigName string `json:"snapshotConfigName,omitempty"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

// RunOriginUpload marks a backup run whose archive an operator uploaded rather than one a backup plan produced.
const RunOriginUpload = "upload"

// BackupRun records a single backup execution.
type BackupRun struct {
	ID     string `json:"id"`
	PlanID string `json:"planId,omitempty"`
	// PlanName is joined from container_backup_plans in ListRuns/Run only,
	// empty for an ad-hoc run or one whose plan was deleted.
	PlanName  string `json:"planName,omitempty"`
	Operation string `json:"operation"`
	// Origin is "upload" for an archive supplied by an operator, empty when McHarbor made it.
	Origin        string `json:"origin,omitempty"`
	SourceRunID   string `json:"sourceRunId,omitempty"`
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName,omitempty"`
	// See BackupPlan.ContainerIDStale.
	ContainerIDStale  bool                   `json:"containerIdStale,omitempty"`
	Status            string                 `json:"status"`
	ArchivePath       string                 `json:"archivePath,omitempty"`
	ArchiveSize       int64                  `json:"archiveSize"`
	ArchiveEncryption string                 `json:"archiveEncryption,omitempty"`
	ArchiveKeyID      string                 `json:"archiveKeyId,omitempty"`
	RequiresSecretKey bool                   `json:"requiresSecretKey,omitempty"`
	Error             string                 `json:"error,omitempty"`
	ProgressStage     string                 `json:"progressStage,omitempty"`
	ProgressMessage   string                 `json:"progressMessage,omitempty"`
	ProgressUpdatedAt string                 `json:"progressUpdatedAt,omitempty"`
	Destinations      []BackupRunDestination `json:"destinations"`
	// IncludedItems is what this run actually captured (backup) or
	// wrote back (restore), in the "config" / "image" / "filesystem" /
	// "logs" / "mount:<destination>" vocabulary Service.Options mints.
	// AvailableItems is what was on offer at the time — the container's
	// inventory for a backup, the source archive's contents for a
	// restore. Scope is the derived "full" / "partial" label, empty when
	// AvailableItems was never recorded (rows predating migration 140,
	// or a container inspect that failed) so the UI can decline to claim
	// either.
	IncludedItems  []string `json:"includedItems"`
	AvailableItems []string `json:"availableItems"`
	Scope          string   `json:"scope,omitempty"`
	StartedAt      string   `json:"startedAt"`
	CompletedAt    string   `json:"completedAt,omitempty"`
	DurationMS     int64    `json:"durationMs"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

// BackupRunDestination records where a backup archive was stored.
type BackupRunDestination struct {
	ID                  string `json:"id"`
	RunID               string `json:"runId"`
	StorageLocationID   string `json:"storageLocationId,omitempty"`
	StorageLocationName string `json:"storageLocationName"`
	LocationType        string `json:"locationType"`
	Status              string `json:"status"`
	Path                string `json:"path"`
	Error               string `json:"error,omitempty"`
	UploadedAt          string `json:"uploadedAt,omitempty"`
	BytesUploaded       int64  `json:"bytesUploaded"`
	BytesTotal          int64  `json:"bytesTotal"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// BackupMigrationResult summarizes completed backup archive migrations.
type BackupMigrationResult struct {
	StorageLocationID string `json:"storageLocationId"`
	Total             int    `json:"total"`
	Migrated          int    `json:"migrated"`
	Skipped           int    `json:"skipped"`
	Failed            int    `json:"failed"`
}

// BackupDownload describes a validated backup archive ready to stream.
type BackupDownload struct {
	RunID       string
	Path        string
	FileName    string
	ContentType string
	Size        int64
	ModTime     string
}

// RestoreBackupInput describes a restore request for an encrypted backup run.
type RestoreBackupInput struct {
	SecretKey    string   `json:"secretKey,omitempty"`
	RestoreItems []string `json:"restoreItems,omitempty"`
	// WorkflowSelfAuthorized is the saved host_self decision of the workflow running the
	// restore; nil means a user-driven restore. It is never decoded from a request body.
	WorkflowSelfAuthorized *bool `json:"-"`
	// deferFilesystem makes a restore leave the filesystem layer for a second
	// pass into the recreated container; set by restoreConfigThenFilesystem only.
	deferFilesystem bool
}

// RestoreBackupResult describes which archive entries were restored.
// Restorable is everything the archive offered, whether or not the caller
// selected it — the denominator that makes "full" or "partial" meaningful.
// It is nil when the pipeline could not enumerate the archive (the agent
// transport reports only what it wrote back).
type RestoreBackupResult struct {
	RunID      string   `json:"runId"`
	Restored   []string `json:"restored"`
	Restorable []string `json:"restorable,omitempty"`
	// recreatedID is the replacement container a config restore created, for
	// the filesystem pass that has to follow it there.
	recreatedID string
}

// RestoreBackupOptionsInput describes a restore options request for an encrypted backup run.
type RestoreBackupOptionsInput struct {
	SecretKey string `json:"secretKey,omitempty"`
}

// RestoreBackupOptions describes restorable archive entries.
type RestoreBackupOptions struct {
	RunID   string         `json:"runId"`
	Items   []BackupOption `json:"items"`
	Message string         `json:"message,omitempty"`
}

// RestoreFilesystemEntriesInput is the request body for the
// filesystem tree-listing endpoint used by the restore dialog's
// per-file/per-folder picker.
type RestoreFilesystemEntriesInput struct {
	SecretKey string `json:"secretKey,omitempty"`
	InnerPath string `json:"innerPath,omitempty"`
}

// RestoreMountEntriesInput is the request body for the per-mount
// tree-listing endpoint. MountTarget is the source-mount path as
// recorded in the backup plan (e.g. "/data").
type RestoreMountEntriesInput struct {
	SecretKey   string `json:"secretKey,omitempty"`
	MountTarget string `json:"mountTarget"`
	InnerPath   string `json:"innerPath,omitempty"`
}

// RestoreConfigEntriesInput is the request body for the synthetic
// config tree-listing endpoint. ParentKey is empty for the root and
// otherwise a wire key like `config:Config.Env`.
type RestoreConfigEntriesInput struct {
	SecretKey string `json:"secretKey,omitempty"`
	ParentKey string `json:"parentKey,omitempty"`
}

// RestoreEntriesResult is the response shape for all three tree
// endpoints. Truncated is set when the server-side cap was reached
// and the listing is incomplete; MaxChildrenCap echoes the cap.
type RestoreEntriesResult struct {
	Items          []BackupOption `json:"items"`
	Truncated      bool           `json:"truncated"`
	MaxChildrenCap int            `json:"maxChildrenCap"`
}

// RestoreUploadedBackupInput describes an uploaded archive restore request.
type RestoreUploadedBackupInput struct {
	EnvironmentID string
	ContainerID   string
	SecretKey     string
	Reader        interface {
		Read([]byte) (int, error)
	}
}

// BackupOption describes one selectable backup item that exists for a container.
type BackupOption struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
	Required    bool   `json:"required"`
	// Partial reports whether this option supports "choose" mode — a
	// subset selected by path. Config, filesystem, and mounts do; image
	// and logs are all-or-nothing, so the UI renders them two-state.
	Partial bool `json:"partial"`
	// Tree-only fields (populated by restoreArchiveOptions when the
	// row represents a folder/file inside a mount tar or the synthetic
	// config root). Safe to omit on plain top-level rows because the
	// frontend types mark them optional.
	EntryKind   string `json:"entryKind,omitempty"`
	Path        string `json:"path,omitempty"`
	ParentMount string `json:"parentMount,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// BackupOptions is the option set returned for a specific container.
type BackupOptions struct {
	ContainerID   string         `json:"containerId"`
	ContainerName string         `json:"containerName"`
	Options       []BackupOption `json:"options"`
	// ConfigSections is the catalog the config option's "choose" mode
	// picks from. It rides on this response rather than a route of its
	// own so the picker needs no new endpoint and no new permission.
	ConfigSections []ConfigSection `json:"configSections"`
}

// CreateBackupPlanInput is used for creating a backup plan.
type CreateBackupPlanInput struct {
	Name               string                     `json:"name"`
	ContainerID        string                     `json:"containerId"`
	StorageLocationID  string                     `json:"storageLocationId"`
	StorageLocationIDs []string                   `json:"storageLocationIds"`
	IncludeConfig      bool                       `json:"includeConfig"`
	IncludeLogs        bool                       `json:"includeLogs"`
	IncludeFilesystem  bool                       `json:"includeFilesystem"`
	IncludeImage       bool                       `json:"includeImage"`
	SelectedMounts     []string                   `json:"selectedMounts"`
	Selections         map[string]BackupSelection `json:"selections,omitempty"`
	LogTailLines       int                        `json:"logTailLines"`
	Cron               string                     `json:"cron"`
	Enabled            bool                       `json:"enabled"`
	RetentionCount     int                        `json:"retentionCount"`
	RetentionDays      int                        `json:"retentionDays"`
	// ConsistencyMode decides how the archive is made consistent:
	// "pause" stops the container for the duration, "snapshot" freezes
	// the filesystem instead and leaves it running, "live" accepts a
	// possibly torn copy. Empty means "pause", which is what every
	// plan written before snapshots existed did.
	ConsistencyMode  string `json:"consistencyMode"`
	SnapshotConfigID string `json:"snapshotConfigId"`
}

// UpdateBackupPlanInput is used for updating a backup plan.
type UpdateBackupPlanInput struct {
	Name               *string                     `json:"name"`
	StorageLocationID  *string                     `json:"storageLocationId"`
	StorageLocationIDs *[]string                   `json:"storageLocationIds"`
	IncludeConfig      *bool                       `json:"includeConfig"`
	IncludeLogs        *bool                       `json:"includeLogs"`
	IncludeFilesystem  *bool                       `json:"includeFilesystem"`
	IncludeImage       *bool                       `json:"includeImage"`
	SelectedMounts     *[]string                   `json:"selectedMounts"`
	Selections         *map[string]BackupSelection `json:"selections"`
	LogTailLines       *int                        `json:"logTailLines"`
	Cron               *string                     `json:"cron"`
	Enabled            *bool                       `json:"enabled"`
	RetentionCount     *int                        `json:"retentionCount"`
	RetentionDays      *int                        `json:"retentionDays"`
	ConsistencyMode    *string                     `json:"consistencyMode"`
	SnapshotConfigID   *string                     `json:"snapshotConfigId"`
}

// RunBackupInput describes an ad-hoc manual backup request.
type RunBackupInput struct {
	Name               string                     `json:"name"`
	StorageLocationID  string                     `json:"storageLocationId"`
	StorageLocationIDs []string                   `json:"storageLocationIds"`
	IncludeConfig      bool                       `json:"includeConfig"`
	IncludeLogs        bool                       `json:"includeLogs"`
	IncludeFilesystem  bool                       `json:"includeFilesystem"`
	IncludeImage       bool                       `json:"includeImage"`
	SelectedMounts     []string                   `json:"selectedMounts"`
	Selections         map[string]BackupSelection `json:"selections,omitempty"`
}
