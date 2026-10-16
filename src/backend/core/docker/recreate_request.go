// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import "github.com/moby/moby/api/types/container"

// The recreate request shape lives in core so the containers module (which
// serves POST /containers/{id}/recreate) and the container_backups module
// (which rebuilds a container from a restored config) share one definition
// without importing each other. The containers module aliases these types.

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

// RecreateRequest is the JSON body for POST /containers/{id}/recreate.
type RecreateRequest struct {
	Image             string                       `json:"image,omitempty"`
	PullImage         bool                         `json:"pullImage,omitempty"`
	Env               []string                     `json:"env,omitempty"`
	Labels            map[string]string            `json:"labels,omitempty"`
	Cmd               []string                     `json:"cmd,omitempty"`
	Entrypoint        []string                     `json:"entrypoint,omitempty"`
	WorkingDir        *string                      `json:"workingDir,omitempty"`
	Hostname          *string                      `json:"hostname,omitempty"`
	Domainname        *string                      `json:"domainname,omitempty"`
	User              *string                      `json:"user,omitempty"`
	Tty               *bool                        `json:"tty,omitempty"`
	OpenStdin         *bool                        `json:"openStdin,omitempty"`
	StopSignal        *string                      `json:"stopSignal,omitempty"`
	ExposedPorts      map[string]struct{}          `json:"exposedPorts,omitempty"`
	PortBindings      map[string][]PortBindingSpec `json:"portBindings,omitempty"`
	NetworkMode       *string                      `json:"networkMode,omitempty"`
	Privileged        *bool                        `json:"privileged,omitempty"`
	ReadonlyRootfs    *bool                        `json:"readonlyRootfs,omitempty"`
	Dns               []string                     `json:"dns,omitempty"`
	DnsSearch         []string                     `json:"dnsSearch,omitempty"`
	DnsOptions        []string                     `json:"dnsOptions,omitempty"`
	ExtraHosts        []string                     `json:"extraHosts,omitempty"`
	CapAdd            []string                     `json:"capAdd,omitempty"`
	CapDrop           []string                     `json:"capDrop,omitempty"`
	SecurityOpt       []string                     `json:"securityOpt,omitempty"`
	ShmSize           *int64                       `json:"shmSize,omitempty"`
	PidMode           *string                      `json:"pidMode,omitempty"`
	Init              *bool                        `json:"init,omitempty"`
	AutoRemove        *bool                        `json:"autoRemove,omitempty"`
	OomKillDisable    *bool                        `json:"oomKillDisable,omitempty"`
	PidsLimit         *int64                       `json:"pidsLimit,omitempty"`
	Devices           *[]container.DeviceMapping   `json:"devices,omitempty"`
	DeviceRequests    *[]container.DeviceRequest   `json:"deviceRequests,omitempty"`
	Memory            *int64                       `json:"memory,omitempty"`
	MemorySwap        *int64                       `json:"memorySwap,omitempty"`
	MemoryReservation *int64                       `json:"memoryReservation,omitempty"`
	NanoCPUs          *int64                       `json:"nanoCPUs,omitempty"`
	CPUShares         *int64                       `json:"cpuShares,omitempty"`
	CPUPeriod         *int64                       `json:"cpuPeriod,omitempty"`
	CPUQuota          *int64                       `json:"cpuQuota,omitempty"`
	CpusetCpus        *string                      `json:"cpusetCpus,omitempty"`
	CpusetMems        *string                      `json:"cpusetMems,omitempty"`
	BlkioWeight       *uint16                      `json:"blkioWeight,omitempty"`
	RestartPolicy     *container.RestartPolicy     `json:"restartPolicy,omitempty"`
	LogDriver         *string                      `json:"logDriver,omitempty"`
	LogOptions        map[string]string            `json:"logOptions,omitempty"`
	Healthcheck       *HealthcheckSpec             `json:"healthcheck,omitempty"`
	UnlockProtected   bool                         `json:"unlockProtected,omitempty"`
	SecretFileMode    *bool                        `json:"secretFileMode,omitempty"`
}
