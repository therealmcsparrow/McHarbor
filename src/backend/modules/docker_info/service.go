// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker_info

import (
	"context"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service handles Docker system info retrieval.
type Service struct {
	dockerPool *docker.ClientPool
}

// NewService creates a new docker info service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{dockerPool: pool}
}

// SystemInfo returns extended Docker daemon info for the given environment.
func (s *Service) SystemInfo(ctx context.Context, envID string) (*DockerSystemInfo, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	infoCtx, infoCancel := context.WithTimeout(ctx, 30*time.Second)
	defer infoCancel()

	info, err := cli.Info(infoCtx)
	if err != nil {
		return nil, fmt.Errorf("fetching Docker info: %w", err)
	}

	verCtx, verCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verCancel()

	version, err := cli.ServerVersion(verCtx)
	if err != nil {
		return nil, fmt.Errorf("fetching Docker version: %w", err)
	}

	// Extract runtime names
	runtimes := make([]string, 0, len(info.Runtimes))
	for name := range info.Runtimes {
		runtimes = append(runtimes, name)
	}

	// Extract driver status as string pairs
	driverStatus := make([][]string, 0, len(info.DriverStatus))
	for _, pair := range info.DriverStatus {
		driverStatus = append(driverStatus, []string{pair[0], pair[1]})
	}

	// Extract plugin names
	pluginsVolume := make([]string, 0, len(info.Plugins.Volume))
	pluginsVolume = append(pluginsVolume, info.Plugins.Volume...)

	pluginsNetwork := make([]string, 0, len(info.Plugins.Network))
	pluginsNetwork = append(pluginsNetwork, info.Plugins.Network...)

	pluginsLog := make([]string, 0, len(info.Plugins.Log))
	pluginsLog = append(pluginsLog, info.Plugins.Log...)

	// Swarm info
	swarmActive := info.Swarm.LocalNodeState == "active"
	swarmNodeID := info.Swarm.NodeID
	swarmManagers := info.Swarm.Managers
	swarmNodes := info.Swarm.Nodes

	labels := info.Labels
	if labels == nil {
		labels = []string{}
	}

	securityOptions := info.SecurityOptions
	if securityOptions == nil {
		securityOptions = []string{}
	}

	return &DockerSystemInfo{
		ID:            info.ID,
		ServerVersion: info.ServerVersion,
		APIVersion:    version.APIVersion,
		MinAPIVersion: version.MinAPIVersion,
		GitCommit:     version.GitCommit,
		GoVersion:     version.GoVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		KernelVersion: info.KernelVersion,
		Hostname:      info.Name,

		NCPU:     info.NCPU,
		MemTotal: info.MemTotal,

		StorageDriver: info.Driver,
		DockerRootDir: info.DockerRootDir,
		DriverStatus:  driverStatus,

		CgroupDriver:  info.CgroupDriver,
		CgroupVersion: info.CgroupVersion,
		DefaultRuntime: info.DefaultRuntime,
		Runtimes:      runtimes,

		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersPaused:  info.ContainersPaused,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,

		SecurityOptions: securityOptions,

		PluginsVolume:  pluginsVolume,
		PluginsNetwork: pluginsNetwork,
		PluginsLog:     pluginsLog,

		Labels: labels,

		SwarmActive:   swarmActive,
		SwarmNodeID:   swarmNodeID,
		SwarmManagers: swarmManagers,
		SwarmNodes:    swarmNodes,

		LoggingDriver: info.LoggingDriver,
		Isolation:     string(info.Isolation),
	}, nil
}
