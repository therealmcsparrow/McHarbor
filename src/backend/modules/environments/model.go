// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

// TopologyNode is a single node in an environment's topology graph. Nodes
// can represent containers, networks, or volumes.
type TopologyNode struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"` // "container" | "network" | "volume"
	Label      string `json:"label"`
	State      string `json:"state,omitempty"`
	StackName  string `json:"stackName,omitempty"`
	Subnet     string `json:"subnet,omitempty"`
	Mountpoint string `json:"mountpoint,omitempty"`
}

// TopologyEdge is a connection between two nodes. "container-network" and
// "container-volume" are the supported kinds.
type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// TopologyResponse is the full topology graph for a single environment.
type TopologyResponse struct {
	EnvID string         `json:"envId"`
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// Environment represents a container environment/host connection stored in the DB.
type Environment struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ConnectionType string  `json:"connectionType"`
	SocketPath     *string `json:"socketPath,omitempty"`
	Host           *string `json:"host,omitempty"`
	Port           *int    `json:"port,omitempty"`
	TLSCa          *string `json:"tlsCa,omitempty"`
	TLSCert        *string `json:"tlsCert,omitempty"`
	TLSKey         *string `json:"tlsKey,omitempty"`
	SSHHost        *string `json:"sshHost,omitempty"`
	SSHPort        *int    `json:"sshPort,omitempty"`
	SSHUser        *string `json:"sshUser,omitempty"`
	SSHKey         *string `json:"sshKey,omitempty"`
	IsDefault      bool    `json:"isDefault"`
	IsActive       bool    `json:"isActive"`
	DockerVersion  *string `json:"dockerVersion,omitempty"`
	LastConnected  *string `json:"lastConnected,omitempty"`
	// Agent fields
	AgentToken    *string `json:"agentToken,omitempty"`
	AgentStatus   *string `json:"agentStatus,omitempty"`
	AgentVersion  *string `json:"agentVersion,omitempty"`
	AgentHostname *string `json:"agentHostname,omitempty"`
	// AgentHostIP and AgentBuild come from the live connection, like the fields
	// above them: neither is stored, and a remembered copy would keep describing
	// an agent that has since moved or gone.
	AgentHostIP   *string `json:"agentHostIp,omitempty"`
	AgentBuild    *string `json:"agentBuild,omitempty"`
	AgentOS       *string `json:"agentOs,omitempty"`
	AgentArch     *string `json:"agentArch,omitempty"`
	AgentLastSeen *string `json:"agentLastSeen,omitempty"`
	// AgentInstallKind is "native" or "container", and nil for an agent older
	// than 3.5.0. Only a native agent can host guests, which is why it is on
	// the environment rather than buried in the capability probe.
	AgentInstallKind *string `json:"agentInstallKind,omitempty"`
	// AgentTLSMode is how the live agent secured its control channel, and is
	// nil for anything that is not a connected agent environment.
	AgentTLSMode *string `json:"agentTlsMode,omitempty"`
	// PendingConnectionType is the type this environment is being migrated to.
	// The row still reads as its current type until the new connection proves
	// itself, so nothing breaks while an agent is being installed.
	PendingConnectionType               *string `json:"pendingConnectionType,omitempty"`
	PendingMigrationStartedAt           *string `json:"pendingMigrationStartedAt,omitempty"`
	MigrationRevertTo                   *string `json:"migrationRevertTo,omitempty"`
	ScheduledUpdateCheckEnabled         bool    `json:"scheduledUpdateCheckEnabled"`
	AutomaticImagePruningEnabled        bool    `json:"automaticImagePruningEnabled"`
	TrackContainerEventsEnabled         bool    `json:"trackContainerEventsEnabled"`
	CollectContainerMetricsEnabled      bool    `json:"collectContainerMetricsEnabled"`
	HighlightContainerChangesEnabled    bool    `json:"highlightContainerChangesEnabled"`
	DockerDiskUsageNotificationsEnabled bool    `json:"dockerDiskUsageNotificationsEnabled"`
	DockerDiskUsageThresholdPercent     int     `json:"dockerDiskUsageThresholdPercent"`
	DockerDiskUsageUseGlobalDefault     bool    `json:"dockerDiskUsageUseGlobalDefault"`
	Timezone                            string  `json:"timezone"`
	LogRetentionDays                    int     `json:"logRetentionDays"`
	ContainerPruneEnabled               bool    `json:"containerPruneEnabled"`
	ContainerPruneStoppedDays           int     `json:"containerPruneStoppedDays"`
	ImagePruneDanglingOnly              bool    `json:"imagePruneDanglingOnly"`
	AutoUpdateEnabled                   bool    `json:"autoUpdateEnabled"`
	AutoUpdateWindowStart               string  `json:"autoUpdateWindowStart"`
	AutoUpdateWindowEnd                 string  `json:"autoUpdateWindowEnd"`
	AutoUpdateDays                      string  `json:"autoUpdateDays"`
	MetricRetentionHours                int     `json:"metricRetentionHours"`
	CreatedAt                           string  `json:"createdAt"`
	UpdatedAt                           string  `json:"updatedAt"`
	// StackCount is the number of managed stacks in this environment,
	// counted in List only — ByID leaves it nil.
	StackCount *int `json:"stackCount,omitempty"`
}

// CreateRequest is the JSON body for POST /environments.
type CreateRequest struct {
	Name           string  `json:"name"`
	ConnectionType string  `json:"connectionType"`
	SocketPath     *string `json:"socketPath,omitempty"`
	Host           *string `json:"host,omitempty"`
	Port           *int    `json:"port,omitempty"`
	TLSCa          *string `json:"tlsCa,omitempty"`
	TLSCert        *string `json:"tlsCert,omitempty"`
	TLSKey         *string `json:"tlsKey,omitempty"`
	SSHHost        *string `json:"sshHost,omitempty"`
	SSHPort        *int    `json:"sshPort,omitempty"`
	SSHUser        *string `json:"sshUser,omitempty"`
	SSHKey         *string `json:"sshKey,omitempty"`
	IsDefault      bool    `json:"isDefault"`
	// Agent fields — AgentToken is the existing token issued by another
	// McHarbor host when an agent is already running and this install
	// needs to accept its incoming WebSocket connection. When set,
	// the host skips the "show token once" dialog (the user already
	// has the token) and only validates + hashes + stores the value.
	AgentToken *string `json:"agentToken,omitempty"`
}

// UpdateRequest is the JSON body for PUT /environments/{id}.
//
// ConnectionType is accepted but no longer applied: changing it needs a token
// minted or revoked, agent metadata cleared and a live connection dropped, none
// of which a field edit does. POST /environments/{id}/migrate is the one path
// that does all of it. See migration.go.
type UpdateRequest struct {
	Name                                *string `json:"name,omitempty"`
	ConnectionType                      *string `json:"connectionType,omitempty"`
	SocketPath                          *string `json:"socketPath,omitempty"`
	Host                                *string `json:"host,omitempty"`
	Port                                *int    `json:"port,omitempty"`
	TLSCa                               *string `json:"tlsCa,omitempty"`
	TLSCert                             *string `json:"tlsCert,omitempty"`
	TLSKey                              *string `json:"tlsKey,omitempty"`
	SSHHost                             *string `json:"sshHost,omitempty"`
	SSHPort                             *int    `json:"sshPort,omitempty"`
	SSHUser                             *string `json:"sshUser,omitempty"`
	SSHKey                              *string `json:"sshKey,omitempty"`
	IsDefault                           *bool   `json:"isDefault,omitempty"`
	IsActive                            *bool   `json:"isActive,omitempty"`
	ScheduledUpdateCheckEnabled         *bool   `json:"scheduledUpdateCheckEnabled,omitempty"`
	AutomaticImagePruningEnabled        *bool   `json:"automaticImagePruningEnabled,omitempty"`
	TrackContainerEventsEnabled         *bool   `json:"trackContainerEventsEnabled,omitempty"`
	CollectContainerMetricsEnabled      *bool   `json:"collectContainerMetricsEnabled,omitempty"`
	HighlightContainerChangesEnabled    *bool   `json:"highlightContainerChangesEnabled,omitempty"`
	DockerDiskUsageNotificationsEnabled *bool   `json:"dockerDiskUsageNotificationsEnabled,omitempty"`
	DockerDiskUsageThresholdPercent     *int    `json:"dockerDiskUsageThresholdPercent,omitempty"`
	DockerDiskUsageUseGlobalDefault     *bool   `json:"dockerDiskUsageUseGlobalDefault,omitempty"`
	Timezone                            *string `json:"timezone,omitempty"`
	LogRetentionDays                    *int    `json:"logRetentionDays,omitempty"`
	ContainerPruneEnabled               *bool   `json:"containerPruneEnabled,omitempty"`
	ContainerPruneStoppedDays           *int    `json:"containerPruneStoppedDays,omitempty"`
	ImagePruneDanglingOnly              *bool   `json:"imagePruneDanglingOnly,omitempty"`
	AutoUpdateEnabled                   *bool   `json:"autoUpdateEnabled,omitempty"`
	AutoUpdateWindowStart               *string `json:"autoUpdateWindowStart,omitempty"`
	AutoUpdateWindowEnd                 *string `json:"autoUpdateWindowEnd,omitempty"`
	AutoUpdateDays                      *string `json:"autoUpdateDays,omitempty"`
	MetricRetentionHours                *int    `json:"metricRetentionHours,omitempty"`
}

// DetectedSocket represents an auto-detected Docker socket.
type DetectedSocket struct {
	Path    string `json:"path"`
	Runtime string `json:"runtime"`
}

// TestResult represents the result of a connection test.
type TestResult struct {
	Success       bool    `json:"success"`
	DockerVersion *string `json:"dockerVersion,omitempty"`
	Error         string  `json:"error,omitempty"`
}

// EnvTestStep is a single named phase of an environment connection
// test. Status mirrors the storage-location test vocabulary so the
// frontend can render pass / fail / warn / skip with the same badge
// styling. Latency is a Go-formatted duration ("12.3ms", "1.4s").
type EnvTestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// EnvTestResult is the detailed per-step connection test result
// returned to the frontend dialog. The existing TestResult type is
// kept for backwards compatibility with any other consumer; the
// /environments/{id}/test endpoint returns this richer shape.
type EnvTestResult struct {
	EnvID         string        `json:"envId"`
	EnvName       string        `json:"envName"`
	Orchestrator  string        `json:"orchestrator"`
	Overall       string        `json:"overall"`
	Steps         []EnvTestStep `json:"steps"`
	DockerVersion *string       `json:"dockerVersion,omitempty"`
	Duration      string        `json:"duration"`
	StartedAt     string        `json:"startedAt"`
	CompletedAt   string        `json:"completedAt"`
}
