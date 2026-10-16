// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerinfo

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/docker"

	"github.com/moby/moby/client"
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
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	infoCtx, infoCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer infoCancel()

	infoRes, err := cli.Info(infoCtx, client.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		return nil, fmt.Errorf("fetching Docker info: %w", err)
	}

	verCtx, verCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer verCancel()

	version, err := cli.ServerVersion(verCtx, client.ServerVersionOptions{})
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
		GitCommit:     engineDetail(version, "GitCommit"),
		GoVersion:     engineDetail(version, "GoVersion"),
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		KernelVersion: info.KernelVersion,
		Hostname:      info.Name,

		NCPU:     info.NCPU,
		MemTotal: info.MemTotal,

		StorageDriver: info.Driver,
		DockerRootDir: info.DockerRootDir,
		DriverStatus:  driverStatus,

		CgroupDriver:   info.CgroupDriver,
		CgroupVersion:  info.CgroupVersion,
		DefaultRuntime: info.DefaultRuntime,
		Runtimes:       runtimes,

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

// engineDetail reads key from the "Engine" component of a /version response;
// the moby client no longer surfaces the daemon's top-level GitCommit and
// GoVersion, and every engine since 18.09 repeats them in the Engine details.
func engineDetail(version client.ServerVersionResult, key string) string {
	for _, c := range version.Components {
		if c.Name == "Engine" {
			return c.Details[key]
		}
	}
	return ""
}
