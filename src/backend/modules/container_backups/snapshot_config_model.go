// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

// SnapshotConfig is a per-environment consistency-snapshot setup: which driver freezes a container's volumes and the last probe result.
type SnapshotConfig struct {
	ID            string            `json:"id"`
	EnvironmentID string            `json:"environmentId"`
	Name          string            `json:"name"`
	Driver        string            `json:"driver"`
	PluginSlug    string            `json:"pluginSlug"`
	Enabled       bool              `json:"enabled"`
	PoolName      string            `json:"poolName,omitempty"`
	DatasetPrefix string            `json:"datasetPrefix,omitempty"`
	SSHTarget     string            `json:"sshTarget,omitempty"`
	UseLXD        bool              `json:"useLxd"`
	Config        map[string]string `json:"config,omitempty"`
	// SecretsSet names the secret fields that hold a stored value. Their values are
	// never returned, so this is how a form knows to say "leave blank to keep".
	SecretsSet   []string `json:"secretsSet,omitempty"`
	LastProbedAt string   `json:"lastProbedAt,omitempty"`
	LastProbe    string   `json:"lastProbe,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// CreateSnapshotConfigInput is the request body for creating a snapshot configuration.
type CreateSnapshotConfigInput struct {
	EnvironmentID string            `json:"environmentId"`
	Name          string            `json:"name"`
	Driver        string            `json:"driver"`
	PluginSlug    string            `json:"pluginSlug"`
	Enabled       *bool             `json:"enabled"`
	PoolName      string            `json:"poolName"`
	DatasetPrefix string            `json:"datasetPrefix"`
	SSHTarget     string            `json:"sshTarget"`
	UseLXD        *bool             `json:"useLxd"`
	Config        map[string]string `json:"config"`
}

// UpdateSnapshotConfigInput is the partial-update body for a snapshot configuration; nil fields keep their stored value.
type UpdateSnapshotConfigInput struct {
	Name          *string            `json:"name"`
	Enabled       *bool              `json:"enabled"`
	PoolName      *string            `json:"poolName"`
	DatasetPrefix *string            `json:"datasetPrefix"`
	SSHTarget     *string            `json:"sshTarget"`
	UseLXD        *bool              `json:"useLxd"`
	Config        *map[string]string `json:"config"`
}

// ProbeSnapshotResult reports whether the snapshot tool is usable on a host and which targets it can snapshot.
type ProbeSnapshotResult struct {
	Available bool                        `json:"available"`
	ToolPath  string                      `json:"toolPath,omitempty"`
	Version   string                      `json:"version,omitempty"`
	Error     string                      `json:"error,omitempty"`
	Targets   []ProbeSnapshotTargetResult `json:"targets"`
	Driver    string                      `json:"driver"`
}

// ProbeSnapshotTargetResult describes one snapshot-capable mount found by a probe.
type ProbeSnapshotTargetResult struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint,omitempty"`
	UsedBytes  int64  `json:"usedBytes"`
	Driver     string `json:"driver"`
}

// RecentSnapshot is a backup run that used snapshot consistency, shown in the recent snapshots card.
type RecentSnapshot struct {
	RunID             string `json:"runId"`
	PlanID            string `json:"planId"`
	EnvironmentID     string `json:"environmentId"`
	ContainerID       string `json:"containerId"`
	ContainerName     string `json:"containerName"`
	SnapshotName      string `json:"snapshotName"`
	SnapshotDriver    string `json:"snapshotDriver"`
	SnapshotMountPath string `json:"snapshotMountPath,omitempty"`
	SnapshotCreatedAt string `json:"snapshotCreatedAt,omitempty"`
	SnapshotRemovedAt string `json:"snapshotRemovedAt,omitempty"`
	SnapshotError     string `json:"snapshotError,omitempty"`
	StartedAt         string `json:"startedAt"`
	Status            string `json:"status"`
}
