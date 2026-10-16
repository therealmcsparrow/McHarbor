// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	networkTypes "github.com/moby/moby/api/types/network"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

// CreateRequest is the JSON body for POST /containers.
type CreateRequest struct {
	Name          string                         `json:"name"`
	Image         string                         `json:"image"`
	Cmd           []string                       `json:"cmd,omitempty"`
	Env           []string                       `json:"env,omitempty"`
	Labels        map[string]string              `json:"labels,omitempty"`
	ExposedPorts  networkTypes.PortSet           `json:"exposedPorts,omitempty"`
	HostConfig    *container.HostConfig          `json:"hostConfig,omitempty"`
	NetworkConfig *networkTypes.NetworkingConfig `json:"networkingConfig,omitempty"`
	Volumes       map[string]struct{}            `json:"volumes,omitempty"`
	WorkingDir    string                         `json:"workingDir,omitempty"`
	Entrypoint    []string                       `json:"entrypoint,omitempty"`
	User          string                         `json:"user,omitempty"`
	Hostname      string                         `json:"hostname,omitempty"`
	Domainname    string                         `json:"domainname,omitempty"`
	Tty           bool                           `json:"tty,omitempty"`
	OpenStdin     bool                           `json:"openStdin,omitempty"`
	// SecretFileMode delivers every ${secret:NAME} this container references
	// as a file under /run/secrets, and sets <VAR>_FILE to its path, instead
	// of putting the value in the environment.
	//
	// Opt-in, and off by default: it only works with images that honour the
	// docker-library `_FILE` convention, and an image that does not will not
	// see the variable at all.
	SecretFileMode bool `json:"secretFileMode,omitempty"`
}

// UpdateRequest is the JSON body for POST /containers/{id}/update.
type UpdateRequest struct {
	Memory            *int64                   `json:"memory,omitempty"`
	MemorySwap        *int64                   `json:"memorySwap,omitempty"`
	MemoryReservation *int64                   `json:"memoryReservation,omitempty"`
	NanoCPUs          *int64                   `json:"nanoCPUs,omitempty"`
	CPUShares         *int64                   `json:"cpuShares,omitempty"`
	CPUQuota          *int64                   `json:"cpuQuota,omitempty"`
	CPUPeriod         *int64                   `json:"cpuPeriod,omitempty"`
	CpusetCpus        *string                  `json:"cpusetCpus,omitempty"`
	CpusetMems        *string                  `json:"cpusetMems,omitempty"`
	BlkioWeight       *uint16                  `json:"blkioWeight,omitempty"`
	RestartPolicy     *container.RestartPolicy `json:"restartPolicy,omitempty"`
	UnlockProtected   bool                     `json:"unlockProtected,omitempty"`
}

// RenameRequest is the JSON body for POST /containers/{id}/rename.
type RenameRequest struct {
	Name            string `json:"name"`
	UnlockProtected bool   `json:"unlockProtected,omitempty"`
}

// PortBindingSpec describes a host binding for a container port.
// Defined in core/docker so container_backups can build a recreate
// request without importing this module.
type PortBindingSpec = coredocker.PortBindingSpec

// HealthcheckSpec mirrors a Docker healthcheck configuration.
type HealthcheckSpec = coredocker.HealthcheckSpec

// NetworkConnectRequest is the JSON body for POST /containers/{id}/network/connect.
type NetworkConnectRequest struct {
	Network     string   `json:"network"`
	Aliases     []string `json:"aliases,omitempty"`
	IPv4Address string   `json:"ipv4Address,omitempty"`
	IPv6Address string   `json:"ipv6Address,omitempty"`
	MacAddress  string   `json:"macAddress,omitempty"`
	Reconnect   bool     `json:"reconnect,omitempty"`
}

// NetworkDisconnectRequest is the JSON body for POST /containers/{id}/network/disconnect.
type NetworkDisconnectRequest struct {
	Network string `json:"network"`
	Force   bool   `json:"force,omitempty"`
}

// RecreateRequest is the JSON body for POST /containers/{id}/recreate.
type RecreateRequest = coredocker.RecreateRequest

// LogsQuery holds parsed query params for GET /containers/{id}/logs.
type LogsQuery struct {
	Stdout bool
	Stderr bool
	Tail   string
	Since  string
}

// ShellResult represents a detected shell in a container.
type ShellResult struct {
	Shell     string `json:"shell"`
	Available bool   `json:"available"`
}

// ContainerSummary is a simplified container info for list responses.
// JSON tags use PascalCase to match Docker API convention and frontend types.
type ContainerSummary struct {
	ID              string                    `json:"Id"`
	Names           []string                  `json:"Names"`
	Image           string                    `json:"Image"`
	ImageID         string                    `json:"ImageID"`
	Command         string                    `json:"Command"`
	Created         int64                     `json:"Created"`
	State           string                    `json:"State"`
	Status          string                    `json:"Status"`
	Ports           []PortBinding             `json:"Ports"`
	Labels          map[string]string         `json:"Labels"`
	StackName       string                    `json:"StackName,omitempty"`
	StackService    string                    `json:"StackService,omitempty"`
	NetworkSettings *ContainerNetworkSettings `json:"NetworkSettings,omitempty"`
	Mounts          []ContainerMountSummary   `json:"Mounts"`
	Protected       bool                      `json:"Protected"`
	// SelfTarget is app, helper or protected when acting on the container needs
	// host_self.manage; HostAccess lists why a container on McHarbor's own environment
	// reaches its host, which gates a shell or file write into it the same way.
	SelfTarget string   `json:"SelfTarget,omitempty"`
	HostAccess []string `json:"HostAccess,omitempty"`
}

// ContainerNetworkSettings holds network info for the list response.
type ContainerNetworkSettings struct {
	Networks map[string]ContainerNetworkInfo `json:"Networks"`
}

// ContainerNetworkInfo holds per-network info.
type ContainerNetworkInfo struct {
	IPAddress  string `json:"IPAddress"`
	Gateway    string `json:"Gateway"`
	MacAddress string `json:"MacAddress"`
}

// ContainerMountSummary holds mount info for the list response.
type ContainerMountSummary struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

// BulkContainerMetric holds computed stats for one container.
type BulkContainerMetric struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpuPercent"`
	MemUsage   int64   `json:"memUsage"`
	MemLimit   int64   `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      int64   `json:"netRx"`
	NetTx      int64   `json:"netTx"`
	BlockRead  int64   `json:"blockRead"`
	BlockWrite int64   `json:"blockWrite"`
	PIDs       uint64  `json:"pids"`
}

// PortBinding is a simplified port binding for the list response.
type PortBinding struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// RemoveExtendedRequest is the JSON body for POST /containers/{id}/remove.
type RemoveExtendedRequest struct {
	Force         bool `json:"force"`
	RemoveVolumes bool `json:"removeVolumes"`
	RemoveImage   bool `json:"removeImage"`
	RemoveStack   bool `json:"removeStack"`
	RemoveNetwork bool `json:"removeNetwork"`
}

// RemovedNetwork reports a network that was removed together with the container.
type RemovedNetwork struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FailedNetworkRemoval reports a network that could not be removed after the container was removed.
type FailedNetworkRemoval struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// RemoveExtendedResult reports which resources were removed.
type RemoveExtendedResult struct {
	ContainerRemoved bool                   `json:"containerRemoved"`
	ImageRemoved     bool                   `json:"imageRemoved"`
	StackRemoved     bool                   `json:"stackRemoved"`
	NetworksRemoved  []RemovedNetwork       `json:"networksRemoved"`
	NetworksFailed   []FailedNetworkRemoval `json:"networksFailed"`
}

// MoveContainerPlanRequest is the JSON body for POST /containers/{id}/move/plan.
type MoveContainerPlanRequest struct {
	TargetEnvID string              `json:"targetEnvId"`
	TargetName  string              `json:"targetName,omitempty"`
	NetworkMode string              `json:"networkMode,omitempty"`
	Networks    []MoveNetworkConfig `json:"networks,omitempty"`
	Volumes     []MoveVolumeConfig  `json:"volumes,omitempty"`
}

// MoveContainerRequest is the JSON body for POST /containers/{id}/move.
type MoveContainerRequest struct {
	TargetEnvID           string              `json:"targetEnvId"`
	TargetName            string              `json:"targetName,omitempty"`
	NetworkMode           string              `json:"networkMode,omitempty"`
	TransferImage         bool                `json:"transferImage"`
	CreateMissingNetworks bool                `json:"createMissingNetworks"`
	CreateMissingVolumes  bool                `json:"createMissingVolumes"`
	CopyNamedVolumes      bool                `json:"copyNamedVolumes"`
	StartTarget           bool                `json:"startTarget"`
	StopSource            bool                `json:"stopSource"`
	RemoveSource          bool                `json:"removeSource"`
	Networks              []MoveNetworkConfig `json:"networks,omitempty"`
	Volumes               []MoveVolumeConfig  `json:"volumes,omitempty"`
}

// MoveContainerPlan describes the resources and configuration changes required to move a container.
type MoveContainerPlan struct {
	SourceEnvID     string            `json:"sourceEnvId"`
	TargetEnvID     string            `json:"targetEnvId"`
	ContainerID     string            `json:"containerId"`
	ContainerName   string            `json:"containerName"`
	TargetName      string            `json:"targetName"`
	Image           MoveImagePlan     `json:"image"`
	Stack           MoveStackPlan     `json:"stack"`
	Volumes         []MoveVolumePlan  `json:"volumes"`
	Networks        []MoveNetworkPlan `json:"networks"`
	NetworkMode     string            `json:"networkMode,omitempty"`
	Ports           []MovePortPlan    `json:"ports"`
	RequiredChanges []string          `json:"requiredChanges"`
	Warnings        []string          `json:"warnings"`
}

// MoveImagePlan reports image availability on the destination environment.
type MoveImagePlan struct {
	Reference    string `json:"reference"`
	ID           string `json:"id"`
	Size         int64  `json:"size,omitempty"`
	Exists       bool   `json:"exists"`
	WillTransfer bool   `json:"willTransfer"`
}

// MoveStackPlan reports Compose stack labels preserved during the move.
type MoveStackPlan struct {
	Name           string `json:"name,omitempty"`
	Service        string `json:"service,omitempty"`
	LabelsPreserve bool   `json:"labelsPreserve"`
	ManagedRecord  bool   `json:"managedRecord"`
}

// MoveVolumePlan reports a mount that affects the container move.
type MoveVolumePlan struct {
	Type              string `json:"type"`
	Name              string `json:"name,omitempty"`
	TargetName        string `json:"targetName,omitempty"`
	Source            string `json:"source,omitempty"`
	TargetSource      string `json:"targetSource,omitempty"`
	Destination       string `json:"destination"`
	TargetDestination string `json:"targetDestination"`
	Mode              string `json:"mode,omitempty"`
	Exists            bool   `json:"exists"`
	WillCreate        bool   `json:"willCreate"`
	WillCopy          bool   `json:"willCopy"`
	Manual            bool   `json:"manual"`
}

// MoveVolumeConfig describes target mount settings for a container move.
type MoveVolumeConfig struct {
	SourceName        string `json:"sourceName"`
	SourceDestination string `json:"sourceDestination,omitempty"`
	TargetName        string `json:"targetName,omitempty"`
	TargetSource      string `json:"targetSource,omitempty"`
	TargetDestination string `json:"targetDestination,omitempty"`
}

// MoveNetworkPlan reports a network attachment needed on the destination environment.
type MoveNetworkPlan struct {
	Name             string             `json:"name"`
	SourceName       string             `json:"sourceName"`
	TargetName       string             `json:"targetName"`
	ID               string             `json:"id,omitempty"`
	Driver           string             `json:"driver,omitempty"`
	Exists           bool               `json:"exists"`
	WillCreate       bool               `json:"willCreate"`
	Aliases          []string           `json:"aliases,omitempty"`
	TargetAliases    []string           `json:"targetAliases,omitempty"`
	IPAddress        string             `json:"ipAddress,omitempty"`
	TargetIPAddress  string             `json:"targetIpAddress,omitempty"`
	MacAddress       string             `json:"macAddress,omitempty"`
	TargetMacAddress string             `json:"targetMacAddress,omitempty"`
	Builtin          bool               `json:"builtin"`
	Internal         bool               `json:"internal"`
	Attachable       bool               `json:"attachable"`
	IPAM             *networkTypes.IPAM `json:"ipam,omitempty"`
	Options          map[string]string  `json:"options,omitempty"`
	Labels           map[string]string  `json:"labels,omitempty"`
}

// MoveNetworkConfig describes target network settings for a container move.
type MoveNetworkConfig struct {
	SourceName string             `json:"sourceName"`
	TargetName string             `json:"targetName"`
	Driver     string             `json:"driver,omitempty"`
	Internal   bool               `json:"internal,omitempty"`
	Attachable bool               `json:"attachable,omitempty"`
	IPAM       *networkTypes.IPAM `json:"ipam,omitempty"`
	Options    map[string]string  `json:"options,omitempty"`
	Labels     map[string]string  `json:"labels,omitempty"`
	Aliases    []string           `json:"aliases,omitempty"`
	IPAddress  string             `json:"ipAddress,omitempty"`
	MacAddress string             `json:"macAddress,omitempty"`
}

// MovePortPlan reports a host port binding that will be reused on the destination.
type MovePortPlan struct {
	ContainerPort string `json:"containerPort"`
	HostIP        string `json:"hostIp,omitempty"`
	HostPort      string `json:"hostPort,omitempty"`
}

// MoveContainerResult reports resources changed during a container move.
type MoveContainerResult struct {
	TargetContainerID string   `json:"targetContainerId"`
	TargetName        string   `json:"targetName"`
	ImageTransferred  bool     `json:"imageTransferred"`
	NetworksCreated   []string `json:"networksCreated"`
	VolumesCreated    []string `json:"volumesCreated"`
	VolumesCopied     []string `json:"volumesCopied"`
	SourceStopped     bool     `json:"sourceStopped"`
	SourceRemoved     bool     `json:"sourceRemoved"`
	Warnings          []string `json:"warnings"`
}

// MoveContainerEvent reports progress while moving a container.
type MoveContainerEvent struct {
	Step              int    `json:"step"`
	Total             int    `json:"total"`
	Message           string `json:"message"`
	Status            string `json:"status"`
	Phase             string `json:"phase,omitempty"`
	BytesTransferred  int64  `json:"bytesTransferred,omitempty"`
	BytesTotal        int64  `json:"bytesTotal,omitempty"`
	TargetContainerID string `json:"targetContainerId,omitempty"`
	TargetName        string `json:"targetName,omitempty"`
}

// ContainerService represents an OS-level service detected inside a container.
type ContainerService struct {
	Name   string `json:"name"`
	Status string `json:"status"`        // "running", "stopped", "failed", etc.
	Sub    string `json:"sub,omitempty"` // sub-state (systemd: "running", "exited", "dead")
}

// ContainerServicesResult holds detected init system and its services.
type ContainerServicesResult struct {
	InitSystem string             `json:"initSystem"` // "systemd", "sysvinit", "openrc", "supervisord", ""
	Services   []ContainerService `json:"services"`
}

// FileEntry represents a file or directory in a container filesystem.
type FileEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	IsDir      bool   `json:"isDir"`
	ModTime    string `json:"modTime"`
	LinkTarget string `json:"linkTarget,omitempty"`
}

// addrString renders an IP the way the Docker API used to hand it over: an unset
// address is the empty string, never netip's "invalid IP".
func addrString(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	return addr.String()
}
