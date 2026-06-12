// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

// BackupPlan stores manual or scheduled backup configuration for one container.
type BackupPlan struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	EnvironmentID      string   `json:"environmentId"`
	ContainerID        string   `json:"containerId"`
	ContainerName      string   `json:"containerName"`
	StorageLocationID  string   `json:"storageLocationId,omitempty"`
	StorageLocationIDs []string `json:"storageLocationIds"`
	IncludeConfig      bool     `json:"includeConfig"`
	IncludeLogs        bool     `json:"includeLogs"`
	IncludeFilesystem  bool     `json:"includeFilesystem"`
	IncludeImage       bool     `json:"includeImage"`
	SelectedMounts     []string `json:"selectedMounts"`
	Cron               string   `json:"cron,omitempty"`
	Enabled            bool     `json:"enabled"`
	RetentionCount     int      `json:"retentionCount"`
	RetentionDays      int      `json:"retentionDays"`
	LastRunAt          string   `json:"lastRunAt,omitempty"`
	NextRunAt          string   `json:"nextRunAt,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

// BackupRun records a single backup execution.
type BackupRun struct {
	ID                string                 `json:"id"`
	PlanID            string                 `json:"planId,omitempty"`
	Operation         string                 `json:"operation"`
	SourceRunID       string                 `json:"sourceRunId,omitempty"`
	EnvironmentID     string                 `json:"environmentId"`
	ContainerID       string                 `json:"containerId"`
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
	StartedAt         string                 `json:"startedAt"`
	CompletedAt       string                 `json:"completedAt,omitempty"`
	DurationMS        int64                  `json:"durationMs"`
	CreatedAt         string                 `json:"createdAt"`
	UpdatedAt         string                 `json:"updatedAt"`
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
}

// RestoreBackupResult describes which archive entries were restored.
type RestoreBackupResult struct {
	RunID    string   `json:"runId"`
	Restored []string `json:"restored"`
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
}

// BackupOptions is the option set returned for a specific container.
type BackupOptions struct {
	ContainerID   string         `json:"containerId"`
	ContainerName string         `json:"containerName"`
	Options       []BackupOption `json:"options"`
}

// CreateBackupPlanInput is used for creating a backup plan.
type CreateBackupPlanInput struct {
	Name               string   `json:"name"`
	ContainerID        string   `json:"containerId"`
	StorageLocationID  string   `json:"storageLocationId"`
	StorageLocationIDs []string `json:"storageLocationIds"`
	IncludeConfig      bool     `json:"includeConfig"`
	IncludeLogs        bool     `json:"includeLogs"`
	IncludeFilesystem  bool     `json:"includeFilesystem"`
	IncludeImage       bool     `json:"includeImage"`
	SelectedMounts     []string `json:"selectedMounts"`
	Cron               string   `json:"cron"`
	Enabled            bool     `json:"enabled"`
	RetentionCount     int      `json:"retentionCount"`
	RetentionDays      int      `json:"retentionDays"`
}

// UpdateBackupPlanInput is used for updating a backup plan.
type UpdateBackupPlanInput struct {
	Name               *string   `json:"name"`
	StorageLocationID  *string   `json:"storageLocationId"`
	StorageLocationIDs *[]string `json:"storageLocationIds"`
	IncludeConfig      *bool     `json:"includeConfig"`
	IncludeLogs        *bool     `json:"includeLogs"`
	IncludeFilesystem  *bool     `json:"includeFilesystem"`
	IncludeImage       *bool     `json:"includeImage"`
	SelectedMounts     *[]string `json:"selectedMounts"`
	Cron               *string   `json:"cron"`
	Enabled            *bool     `json:"enabled"`
	RetentionCount     *int      `json:"retentionCount"`
	RetentionDays      *int      `json:"retentionDays"`
}

// RunBackupInput describes an ad-hoc manual backup request.
type RunBackupInput struct {
	Name               string   `json:"name"`
	StorageLocationID  string   `json:"storageLocationId"`
	StorageLocationIDs []string `json:"storageLocationIds"`
	IncludeConfig      bool     `json:"includeConfig"`
	IncludeLogs        bool     `json:"includeLogs"`
	IncludeFilesystem  bool     `json:"includeFilesystem"`
	IncludeImage       bool     `json:"includeImage"`
	SelectedMounts     []string `json:"selectedMounts"`
}
