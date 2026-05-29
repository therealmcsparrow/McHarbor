// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"github.com/docker/docker/api/types/container"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// CreateRequest is the JSON body for POST /containers.
type CreateRequest struct {
	Name          string                         `json:"name"`
	Image         string                         `json:"image"`
	Cmd           []string                       `json:"cmd,omitempty"`
	Env           []string                       `json:"env,omitempty"`
	Labels        map[string]string              `json:"labels,omitempty"`
	ExposedPorts  nat.PortSet                    `json:"exposedPorts,omitempty"`
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
}

// UpdateRequest is the JSON body for POST /containers/{id}/update.
type UpdateRequest struct {
	Memory            int64                    `json:"memory,omitempty"`
	MemorySwap        int64                    `json:"memorySwap,omitempty"`
	MemoryReservation int64                    `json:"memoryReservation,omitempty"`
	NanoCPUs          int64                    `json:"nanoCPUs,omitempty"`
	CPUShares         int64                    `json:"cpuShares,omitempty"`
	CPUQuota          int64                    `json:"cpuQuota,omitempty"`
	CPUPeriod         int64                    `json:"cpuPeriod,omitempty"`
	CpusetCpus        string                   `json:"cpusetCpus,omitempty"`
	CpusetMems        string                   `json:"cpusetMems,omitempty"`
	BlkioWeight       uint16                   `json:"blkioWeight,omitempty"`
	RestartPolicy     *container.RestartPolicy `json:"restartPolicy,omitempty"`
}

// PortBindingSpec describes a host binding for a container port.
type PortBindingSpec struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// HealthcheckSpec mirrors a Docker healthcheck configuration.
type HealthcheckSpec struct {
	Test        []string `json:"test"`
	Interval    int64    `json:"interval,omitempty"` // nanoseconds
	Timeout     int64    `json:"timeout,omitempty"`  // nanoseconds
	Retries     int      `json:"retries,omitempty"`
	StartPeriod int64    `json:"startPeriod,omitempty"` // nanoseconds
}

// NetworkConnectRequest is the JSON body for POST /containers/{id}/network/connect.
type NetworkConnectRequest struct {
	Network string `json:"network"`
}

// NetworkDisconnectRequest is the JSON body for POST /containers/{id}/network/disconnect.
type NetworkDisconnectRequest struct {
	Network string `json:"network"`
	Force   bool   `json:"force,omitempty"`
}

// RecreateRequest is the JSON body for POST /containers/{id}/recreate.
type RecreateRequest struct {
	Image          string                       `json:"image,omitempty"`
	PullImage      bool                         `json:"pullImage,omitempty"`
	Env            []string                     `json:"env,omitempty"`
	Labels         map[string]string            `json:"labels,omitempty"`
	Cmd            []string                     `json:"cmd,omitempty"`
	Entrypoint     []string                     `json:"entrypoint,omitempty"`
	WorkingDir     *string                      `json:"workingDir,omitempty"`
	Hostname       *string                      `json:"hostname,omitempty"`
	Domainname     *string                      `json:"domainname,omitempty"`
	User           *string                      `json:"user,omitempty"`
	Tty            *bool                        `json:"tty,omitempty"`
	OpenStdin      *bool                        `json:"openStdin,omitempty"`
	StopSignal     *string                      `json:"stopSignal,omitempty"`
	ExposedPorts   map[string]struct{}          `json:"exposedPorts,omitempty"`
	PortBindings   map[string][]PortBindingSpec `json:"portBindings,omitempty"`
	NetworkMode    *string                      `json:"networkMode,omitempty"`
	Privileged     *bool                        `json:"privileged,omitempty"`
	ReadonlyRootfs *bool                        `json:"readonlyRootfs,omitempty"`
	Dns            []string                     `json:"dns,omitempty"`
	DnsSearch      []string                     `json:"dnsSearch,omitempty"`
	DnsOptions     []string                     `json:"dnsOptions,omitempty"`
	ExtraHosts     []string                     `json:"extraHosts,omitempty"`
	CapAdd         []string                     `json:"capAdd,omitempty"`
	CapDrop        []string                     `json:"capDrop,omitempty"`
	SecurityOpt    []string                     `json:"securityOpt,omitempty"`
	ShmSize        *int64                       `json:"shmSize,omitempty"`
	PidMode        *string                      `json:"pidMode,omitempty"`
	Init           *bool                        `json:"init,omitempty"`
	AutoRemove     *bool                        `json:"autoRemove,omitempty"`
	OomKillDisable *bool                        `json:"oomKillDisable,omitempty"`
	PidsLimit      *int64                       `json:"pidsLimit,omitempty"`
	DeviceRequests *[]container.DeviceRequest   `json:"deviceRequests,omitempty"`
	Memory         *int64                       `json:"memory,omitempty"`
	NanoCPUs       *int64                       `json:"nanoCPUs,omitempty"`
	RestartPolicy  *container.RestartPolicy     `json:"restartPolicy,omitempty"`
	LogDriver      *string                      `json:"logDriver,omitempty"`
	LogOptions     map[string]string            `json:"logOptions,omitempty"`
	Healthcheck    *HealthcheckSpec             `json:"healthcheck,omitempty"`
}

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
}

// RemoveExtendedResult reports which resources were removed.
type RemoveExtendedResult struct {
	ContainerRemoved bool `json:"containerRemoved"`
	ImageRemoved     bool `json:"imageRemoved"`
	StackRemoved     bool `json:"stackRemoved"`
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
