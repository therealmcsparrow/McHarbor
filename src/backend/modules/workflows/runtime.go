// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"errors"
)

// Runtime adapters in this file break the workflows → other-modules
// dependency direction. The concrete implementations live in
// `internal/bootstrap/adapters.go` and are injected from `main.go`.

// ContainerBackupInput describes an ad-hoc container backup the
// workflow node needs the container-backups module to execute.
type ContainerBackupInput struct {
	Name              string
	StorageLocationID string
	IncludeConfig     bool
	IncludeLogs       bool
	IncludeFilesystem bool
	IncludeImage      bool
	SelectedMounts    []string
}

// ContainerBackupRunner executes container-backup operations on
// behalf of workflow nodes.
type ContainerBackupRunner interface {
	RunAdhoc(ctx context.Context, envID, containerID string, input ContainerBackupInput) (any, error)
	RunPlan(ctx context.Context, planID string) (any, error)
	Download(ctx context.Context, runID string) (*ContainerBackupDownload, error)
	// Restore replays a completed run into the container. Empty items
	// means everything the archive offers.
	Restore(ctx context.Context, runID string, input ContainerBackupRestoreInput) (any, error)
	// ListPlans / ListRuns are environment scoped; containerID narrows
	// the result when non-empty.
	ListPlans(ctx context.Context, envID, containerID string) (any, error)
	ListRuns(ctx context.Context, envID, containerID string) (any, error)
	Run(ctx context.Context, runID string) (any, error)
	CancelRun(ctx context.Context, runID string) (any, error)
	DeleteRun(ctx context.Context, runID string) (bool, error)
}

// ContainerBackupRestoreInput describes a restore the workflow node
// asks the container-backups module to perform.
type ContainerBackupRestoreInput struct {
	SecretKey    string
	RestoreItems []string
	// SelfHostAuthorized is the running workflow's saved host_self decision.
	SelfHostAuthorized bool
}

// ContainerBackupDownload is the subset of a backup-run download
// descriptor that workflow nodes need to surface in their payload.
type ContainerBackupDownload struct {
	RunID       string
	Path        string
	FileName    string
	ContentType string
	Size        int64
	ModTime     string
}

// LinkContainerRequest describes the link/unlink stack-container
// workflow nodes pass to the stacks module.
type LinkContainerRequest struct {
	ContainerID string
	StackName   string
	ServiceName string
}

// StackContainerLinker links or unlinks a container to/from a
// compose stack on behalf of a workflow node.
type StackContainerLinker interface {
	LinkContainer(ctx context.Context, envID string, req LinkContainerRequest) (any, error)
	UnlinkContainer(envID, containerID string) error
}

// StorageLocationSummary is the subset of storage location fields
// the workflow storage-location nodes need.
type StorageLocationSummary struct {
	ID           string
	Name         string
	LocationType string
	BasePath     string
	Enabled      bool
}

// StorageLocationReader reads storage locations on behalf of
// workflow nodes.
type StorageLocationReader interface {
	List(ctx context.Context) ([]StorageLocationSummary, error)
	ByID(ctx context.Context, id string) (*StorageLocationSummary, error)
}

// StorageObjectSummary is one object returned by a storage listing.
type StorageObjectSummary struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// StorageTestStep mirrors one phase of a storage connection probe.
type StorageTestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// StorageTestResult is the outcome of a storage connection test.
type StorageTestResult struct {
	StorageLocationID string            `json:"storageLocationId"`
	LocationName      string            `json:"locationName"`
	LocationType      string            `json:"locationType"`
	OverallStatus     string            `json:"overallStatus"`
	Duration          string            `json:"duration"`
	Steps             []StorageTestStep `json:"steps"`
}

// StorageUsageSummary is the used / total capacity a location reports.
type StorageUsageSummary struct {
	Supported  bool    `json:"supported"`
	UsedBytes  int64   `json:"usedBytes"`
	TotalBytes int64   `json:"totalBytes"`
	FreeBytes  int64   `json:"freeBytes"`
	Percent    float64 `json:"percent"`
}

// StorageObjectOperator performs object-level operations against a
// storage location through the adapter contract (local or plugin).
type StorageObjectOperator interface {
	ListObjects(ctx context.Context, locationID, prefix string) ([]StorageObjectSummary, error)
	UploadObject(ctx context.Context, locationID, key string, data []byte) error
	DownloadObject(ctx context.Context, locationID, key string) ([]byte, error)
	DeleteObject(ctx context.Context, locationID, key string) error
	Test(ctx context.Context, locationID string) (*StorageTestResult, error)
	Usage(ctx context.Context, locationID string) (*StorageUsageSummary, error)
}

// ErrStorageObjectsUnavailable indicates the storage-object runtime
// adapter has not been configured on the workflow service.
var ErrStorageObjectsUnavailable = errors.New("storage object runtime adapter not configured")

// ErrStorageLocationsUnavailable indicates the storage-location
// runtime adapter has not been configured on the workflow service.
// Callers should treat it as a non-fatal configuration error so
// node execution surfaces it cleanly instead of crashing.
var ErrStorageLocationsUnavailable = errors.New("storage location runtime adapter not configured")

// ErrContainerBackupsUnavailable indicates the container-backup
// runtime adapter has not been configured.
var ErrContainerBackupsUnavailable = errors.New("container backup runtime adapter not configured")

// ErrStackLinkerUnavailable indicates the stack-container linker
// runtime adapter has not been configured.
var ErrStackLinkerUnavailable = errors.New("stack container linker runtime adapter not configured")
