// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

// AppInstallation represents a single installed instance of a catalog app.
type AppInstallation struct {
	ID              string `json:"id"`
	StackID         string `json:"stackId"`
	StackName       string `json:"stackName"`
	EnvironmentID   string `json:"environmentId,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	InstalledAt     string `json:"installedAt"`
}

// AppTemplate represents a single application in the catalog.
type AppTemplate struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    string        `json:"category"`
	Image       string        `json:"image"`
	Logo        string        `json:"logo"`
	Website     string        `json:"website"`
	DocsURL     string        `json:"docsUrl"`
	Ports       []PortMapping `json:"ports"`
	Volumes     []VolumeMount `json:"volumes"`
	EnvVars     []EnvVarDef   `json:"envVars"`
	// Optional raw compose override YAML (merged/used instead of generated compose).
	ComposeOverride string `json:"composeOverride,omitempty"`
	MinMemory       string `json:"minMemory,omitempty"`
	Source          string `json:"source"`
	Version         string `json:"version"`
	// Populated at query time from the installation records.
	Installed     bool              `json:"installed"`
	StackID       string            `json:"stackId,omitempty"`
	Installations []AppInstallation `json:"installations"`
}

// PortMapping defines a container port mapping.
type PortMapping struct {
	Host      int    `json:"host"`
	Container int    `json:"container"`
	Protocol  string `json:"protocol"` // tcp, udp
}

// VolumeMount defines a bind mount or named volume.
type VolumeMount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// EnvVarDef defines an environment variable with metadata.
type EnvVarDef struct {
	Key         string `json:"key"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Secret      bool   `json:"secret,omitempty"`
}

// CatalogFile is the top-level structure of catalog.json.
type CatalogFile struct {
	Version string        `json:"version"`
	Apps    []AppTemplate `json:"apps"`
}

// InstallRequest is the JSON body for POST /app-store/install.
type InstallRequest struct {
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	EnvironmentID string            `json:"environmentId,omitempty"`
	Ports         []PortMapping     `json:"ports,omitempty"`
	Volumes       []VolumeMount     `json:"volumes,omitempty"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
}

// InstallResult is returned after a successful install.
type InstallResult struct {
	AppSlug   string `json:"appSlug"`
	StackID   string `json:"stackId"`
	StackName string `json:"stackName"`
	Status    string `json:"status"`
}

// InstalledApp represents an installed app with its stack status.
type InstalledApp struct {
	ID              string `json:"id"`
	CatalogSlug     string `json:"catalogSlug"`
	StackID         string `json:"stackId"`
	StackName       string `json:"stackName"`
	EnvironmentID   string `json:"environmentId,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	InstalledAt     string `json:"installedAt"`
	// Populated from stacks table.
	StackStatus string `json:"stackStatus"`
}

// SyncStatus represents the remote catalog sync state.
type SyncStatus struct {
	LastSyncedAt string `json:"lastSyncedAt"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	AppsAdded    int    `json:"appsAdded"`
	AppsUpdated  int    `json:"appsUpdated"`
}

// CategoryCount holds a category name and count of apps in it.
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// InstallEvent is an SSE event sent during streaming install.
type InstallEvent struct {
	Step      int    `json:"step"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
	Status    string `json:"status"`          // "progress", "done", "error"
	Phase     string `json:"phase,omitempty"` // "install", "scan", "scan-result", "scan-error"
	StackID   string `json:"stackId,omitempty"`
	StackName string `json:"stackName,omitempty"`
}
