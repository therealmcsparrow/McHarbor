// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

// Environment represents a container environment/host connection stored in the DB.
type Environment struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	OrchestratorType string  `json:"orchestratorType"`
	ConnectionType   string  `json:"connectionType"`
	SocketPath       *string `json:"socketPath,omitempty"`
	Host             *string `json:"host,omitempty"`
	Port             *int    `json:"port,omitempty"`
	TLSCa            *string `json:"tlsCa,omitempty"`
	TLSCert          *string `json:"tlsCert,omitempty"`
	TLSKey           *string `json:"tlsKey,omitempty"`
	SSHHost          *string `json:"sshHost,omitempty"`
	SSHPort          *int    `json:"sshPort,omitempty"`
	SSHUser          *string `json:"sshUser,omitempty"`
	SSHKey           *string `json:"sshKey,omitempty"`
	IsDefault        bool    `json:"isDefault"`
	IsActive         bool    `json:"isActive"`
	DockerVersion    *string `json:"dockerVersion,omitempty"`
	LastConnected    *string `json:"lastConnected,omitempty"`
	// Kubernetes fields
	Kubeconfig     *string `json:"kubeconfig,omitempty"`
	K8sNamespace   *string `json:"k8sNamespace,omitempty"`
	K8sServerURL   *string `json:"k8sServerUrl,omitempty"`
	K8sBearerToken *string `json:"k8sBearerToken,omitempty"`
	K8sCACert      *string `json:"k8sCaCert,omitempty"`
	K8sVersion     *string `json:"k8sVersion,omitempty"`
	// Agent fields
	AgentToken                          *string `json:"agentToken,omitempty"`
	AgentStatus                         *string `json:"agentStatus,omitempty"`
	AgentVersion                        *string `json:"agentVersion,omitempty"`
	AgentHostname                       *string `json:"agentHostname,omitempty"`
	AgentOS                             *string `json:"agentOs,omitempty"`
	AgentArch                           *string `json:"agentArch,omitempty"`
	AgentLastSeen                       *string `json:"agentLastSeen,omitempty"`
	ScheduledUpdateCheckEnabled         bool    `json:"scheduledUpdateCheckEnabled"`
	AutomaticImagePruningEnabled        bool    `json:"automaticImagePruningEnabled"`
	TrackContainerEventsEnabled         bool    `json:"trackContainerEventsEnabled"`
	CollectContainerMetricsEnabled      bool    `json:"collectContainerMetricsEnabled"`
	HighlightContainerChangesEnabled    bool    `json:"highlightContainerChangesEnabled"`
	DockerDiskUsageNotificationsEnabled bool    `json:"dockerDiskUsageNotificationsEnabled"`
	DockerDiskUsageThresholdPercent     int     `json:"dockerDiskUsageThresholdPercent"`
	Timezone                            string  `json:"timezone"`
	CreatedAt                           string  `json:"createdAt"`
	UpdatedAt                           string  `json:"updatedAt"`
}

// CreateRequest is the JSON body for POST /environments.
type CreateRequest struct {
	Name             string  `json:"name"`
	OrchestratorType string  `json:"orchestratorType"`
	ConnectionType   string  `json:"connectionType"`
	SocketPath       *string `json:"socketPath,omitempty"`
	Host             *string `json:"host,omitempty"`
	Port             *int    `json:"port,omitempty"`
	TLSCa            *string `json:"tlsCa,omitempty"`
	TLSCert          *string `json:"tlsCert,omitempty"`
	TLSKey           *string `json:"tlsKey,omitempty"`
	SSHHost          *string `json:"sshHost,omitempty"`
	SSHPort          *int    `json:"sshPort,omitempty"`
	SSHUser          *string `json:"sshUser,omitempty"`
	SSHKey           *string `json:"sshKey,omitempty"`
	IsDefault        bool    `json:"isDefault"`
	// Kubernetes fields
	Kubeconfig     *string `json:"kubeconfig,omitempty"`
	K8sNamespace   *string `json:"k8sNamespace,omitempty"`
	K8sServerURL   *string `json:"k8sServerUrl,omitempty"`
	K8sBearerToken *string `json:"k8sBearerToken,omitempty"`
	K8sCACert      *string `json:"k8sCaCert,omitempty"`
}

// UpdateRequest is the JSON body for PUT /environments/{id}.
type UpdateRequest struct {
	Name             *string `json:"name,omitempty"`
	OrchestratorType *string `json:"orchestratorType,omitempty"`
	ConnectionType   *string `json:"connectionType,omitempty"`
	SocketPath       *string `json:"socketPath,omitempty"`
	Host             *string `json:"host,omitempty"`
	Port             *int    `json:"port,omitempty"`
	TLSCa            *string `json:"tlsCa,omitempty"`
	TLSCert          *string `json:"tlsCert,omitempty"`
	TLSKey           *string `json:"tlsKey,omitempty"`
	SSHHost          *string `json:"sshHost,omitempty"`
	SSHPort          *int    `json:"sshPort,omitempty"`
	SSHUser          *string `json:"sshUser,omitempty"`
	SSHKey           *string `json:"sshKey,omitempty"`
	IsDefault        *bool   `json:"isDefault,omitempty"`
	IsActive         *bool   `json:"isActive,omitempty"`
	// Kubernetes fields
	Kubeconfig                          *string `json:"kubeconfig,omitempty"`
	K8sNamespace                        *string `json:"k8sNamespace,omitempty"`
	K8sServerURL                        *string `json:"k8sServerUrl,omitempty"`
	K8sBearerToken                      *string `json:"k8sBearerToken,omitempty"`
	K8sCACert                           *string `json:"k8sCaCert,omitempty"`
	ScheduledUpdateCheckEnabled         *bool   `json:"scheduledUpdateCheckEnabled,omitempty"`
	AutomaticImagePruningEnabled        *bool   `json:"automaticImagePruningEnabled,omitempty"`
	TrackContainerEventsEnabled         *bool   `json:"trackContainerEventsEnabled,omitempty"`
	CollectContainerMetricsEnabled      *bool   `json:"collectContainerMetricsEnabled,omitempty"`
	HighlightContainerChangesEnabled    *bool   `json:"highlightContainerChangesEnabled,omitempty"`
	DockerDiskUsageNotificationsEnabled *bool   `json:"dockerDiskUsageNotificationsEnabled,omitempty"`
	DockerDiskUsageThresholdPercent     *int    `json:"dockerDiskUsageThresholdPercent,omitempty"`
	Timezone                            *string `json:"timezone,omitempty"`
}

// DetectedSocket represents an auto-detected Docker or Podman socket.
type DetectedSocket struct {
	Path    string `json:"path"`
	Runtime string `json:"runtime"`
}

// TestResult represents the result of a connection test.
type TestResult struct {
	Success       bool    `json:"success"`
	DockerVersion *string `json:"dockerVersion,omitempty"`
	K8sVersion    *string `json:"k8sVersion,omitempty"`
	Error         string  `json:"error,omitempty"`
}
