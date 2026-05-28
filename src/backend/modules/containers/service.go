// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service wraps Docker SDK container operations.
type Service struct {
	pool *docker.ClientPool
	db   *sql.DB
}

// NewService creates a new container service.
func NewService(pool *docker.ClientPool, db *sql.DB) *Service {
	return &Service{pool: pool, db: db}
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(envID string) (*client.Client, error) {
	c, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

type containerStackLink struct {
	StackName    string
	StackService string
}

func (s *Service) stackLinksByContainer(envID string) (map[string]containerStackLink, error) {
	rows, err := s.db.Query(`
		SELECT container_id, stack_name, COALESCE(service_name, '')
		FROM container_stack_links
		WHERE environment_id = ?
		LIMIT 1000
	`, envID)
	if err != nil {
		return nil, fmt.Errorf("querying container stack links: %w", err)
	}
	defer rows.Close()

	links := make(map[string]containerStackLink)
	for rows.Next() {
		var id string
		var link containerStackLink
		if err := rows.Scan(&id, &link.StackName, &link.StackService); err != nil {
			return nil, fmt.Errorf("scanning container stack link: %w", err)
		}
		links[id] = link
	}
	return links, rows.Err()
}

// List returns all containers, optionally including stopped ones.
func (s *Service) List(ctx context.Context, envID string, all bool) ([]ContainerSummary, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	links, err := s.stackLinksByContainer(envID)
	if err != nil {
		return nil, err
	}

	result := make([]ContainerSummary, 0, len(containers))
	for _, c := range containers {
		ports := make([]PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}

		// Map network settings
		var netSettings *ContainerNetworkSettings
		if c.NetworkSettings != nil && len(c.NetworkSettings.Networks) > 0 {
			networks := make(map[string]ContainerNetworkInfo, len(c.NetworkSettings.Networks))
			for name, ep := range c.NetworkSettings.Networks {
				networks[name] = ContainerNetworkInfo{
					IPAddress:  ep.IPAddress,
					Gateway:    ep.Gateway,
					MacAddress: ep.MacAddress,
				}
			}
			netSettings = &ContainerNetworkSettings{Networks: networks}
		}

		// Map mounts
		mounts := make([]ContainerMountSummary, 0, len(c.Mounts))
		for _, m := range c.Mounts {
			mounts = append(mounts, ContainerMountSummary{
				Type:        string(m.Type),
				Source:      m.Source,
				Destination: m.Destination,
				Mode:        m.Mode,
				RW:          m.RW,
			})
		}

		summary := ContainerSummary{
			ID:              c.ID,
			Names:           c.Names,
			Image:           c.Image,
			ImageID:         c.ImageID,
			Command:         c.Command,
			Created:         c.Created,
			State:           c.State,
			Status:          c.Status,
			Ports:           ports,
			Labels:          c.Labels,
			NetworkSettings: netSettings,
			Mounts:          mounts,
		}
		if link, ok := links[c.ID]; ok {
			summary.StackName = link.StackName
			summary.StackService = link.StackService
		}
		result = append(result, summary)
	}

	return result, nil
}

// Inspect returns detailed container information.
func (s *Service) Inspect(ctx context.Context, envID, id string) (types.ContainerJSON, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return types.ContainerJSON{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("inspecting container %s: %w", id, err)
	}

	return info, nil
}

// Create creates a new container.
func (s *Service) Create(ctx context.Context, envID string, req CreateRequest) (container.CreateResponse, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return container.CreateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	config := &container.Config{
		Image:        req.Image,
		Cmd:          req.Cmd,
		Env:          req.Env,
		Labels:       req.Labels,
		ExposedPorts: req.ExposedPorts,
		Volumes:      req.Volumes,
		WorkingDir:   req.WorkingDir,
		Entrypoint:   req.Entrypoint,
		User:         req.User,
		Hostname:     req.Hostname,
		Domainname:   req.Domainname,
		Tty:          req.Tty,
		OpenStdin:    req.OpenStdin,
	}

	hostConfig := req.HostConfig
	if hostConfig == nil {
		hostConfig = &container.HostConfig{}
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, req.NetworkConfig, nil, req.Name)
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("creating container: %w", err)
	}

	return resp, nil
}

// Remove removes a container and optionally cleans up its bind mounts.
func (s *Service) Remove(ctx context.Context, envID, id string, force, removeVolumes bool) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Inspect first to collect bind mount paths before the container is gone.
	var bindMounts []string
	info, inspErr := cli.ContainerInspect(ctx, id)
	if inspErr == nil && info.Mounts != nil {
		for _, m := range info.Mounts {
			if m.Type == "bind" {
				bindMounts = append(bindMounts, m.Source)
			}
		}
	}

	err = cli.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
	if err != nil {
		return fmt.Errorf("removing container %s: %w", id, err)
	}

	// Clean up bind-mount host directories via a temporary container.
	if len(bindMounts) > 0 {
		docker.RemoveBindMounts(ctx, cli, bindMounts)
	}

	return nil
}

// RemoveImage removes a Docker image by ID.
func (s *Service) RemoveImage(ctx context.Context, envID, imageID string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{PruneChildren: true})
	if err != nil {
		return fmt.Errorf("removing image %s: %w", imageID, err)
	}

	return nil
}

// Start starts a stopped container.
func (s *Service) Start(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", id, err)
	}

	return nil
}

// Stop stops a running container with an optional timeout.
func (s *Service) Stop(ctx context.Context, envID, id string, timeout int) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stopOpts := container.StopOptions{}
	if timeout > 0 {
		stopOpts.Timeout = &timeout
	}

	if err := cli.ContainerStop(ctx, id, stopOpts); err != nil {
		return fmt.Errorf("stopping container %s: %w", id, err)
	}

	return nil
}

// Restart restarts a container.
func (s *Service) Restart(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.ContainerRestart(ctx, id, container.StopOptions{}); err != nil {
		return fmt.Errorf("restarting container %s: %w", id, err)
	}

	return nil
}

// Pause pauses a running container.
func (s *Service) Pause(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.ContainerPause(ctx, id); err != nil {
		return fmt.Errorf("pausing container %s: %w", id, err)
	}

	return nil
}

// Unpause unpauses a paused container.
func (s *Service) Unpause(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.ContainerUnpause(ctx, id); err != nil {
		return fmt.Errorf("unpausing container %s: %w", id, err)
	}

	return nil
}

// Kill sends a signal to a container.
func (s *Service) Kill(ctx context.Context, envID, id, signal string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if signal == "" {
		signal = "SIGKILL"
	}

	if err := cli.ContainerKill(ctx, id, signal); err != nil {
		return fmt.Errorf("killing container %s: %w", id, err)
	}

	return nil
}

// Update updates runtime resources for a container.
func (s *Service) Update(ctx context.Context, envID, id string, req UpdateRequest) (container.ContainerUpdateOKBody, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return container.ContainerUpdateOKBody{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	updateConfig := container.UpdateConfig{
		Resources: container.Resources{
			Memory:            req.Memory,
			MemorySwap:        req.MemorySwap,
			MemoryReservation: req.MemoryReservation,
			NanoCPUs:          req.NanoCPUs,
			CPUShares:         req.CPUShares,
			CPUQuota:          req.CPUQuota,
			CPUPeriod:         req.CPUPeriod,
			CpusetCpus:        req.CpusetCpus,
			CpusetMems:        req.CpusetMems,
			BlkioWeight:       req.BlkioWeight,
		},
	}

	if req.RestartPolicy != nil {
		updateConfig.RestartPolicy = *req.RestartPolicy
	}

	resp, err := cli.ContainerUpdate(ctx, id, updateConfig)
	if err != nil {
		return container.ContainerUpdateOKBody{}, fmt.Errorf("updating container %s: %w", id, err)
	}

	return resp, nil
}

// Recreate stops, renames, creates a new container with the same config, starts it, and removes the old one.
func (s *Service) Recreate(ctx context.Context, envID, id string, req RecreateRequest) (container.CreateResponse, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return container.CreateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	// Inspect current container
	info, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("inspecting container for recreate: %w", err)
	}

	originalName := strings.TrimPrefix(info.Name, "/")
	if isSelfMcHarborContainer(info) {
		operation := "reinstall"
		if req.PullImage {
			operation = "update"
		}

		dockerHost := ""
		if envID != "" {
			if host, hostErr := s.pool.DockerHost(envID); hostErr == nil {
				dockerHost = host
			} else {
				slog.Warn("containers: failed to resolve docker host for self recreate helper", "error", hostErr, "env", envID)
			}
		}

		if _, err := docker.ScheduleDetachedSelfUpdateHelper(ctx, cli, info, os.Getenv("DATA_DIR"), dockerHost, operation); err != nil {
			return container.CreateResponse{}, fmt.Errorf("scheduling self recreate helper: %w", err)
		}

		return container.CreateResponse{ID: info.ID}, nil
	}

	// Stop the container if running
	if info.State.Running {
		timeout := 10
		if err := cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
			return container.CreateResponse{}, fmt.Errorf("stopping container for recreate: %w", err)
		}
	}

	// Rename old container
	oldName := originalName + "-old-" + fmt.Sprintf("%d", time.Now().Unix())
	if err := cli.ContainerRename(ctx, id, oldName); err != nil {
		return container.CreateResponse{}, fmt.Errorf("renaming container for recreate: %w", err)
	}

	// Optionally update image
	imgName := info.Config.Image
	if req.Image != "" {
		imgName = req.Image
	}

	// Pull image if requested
	if req.PullImage {
		pullResp, pullErr := cli.ImagePull(ctx, imgName, image.PullOptions{})
		if pullErr != nil {
			// Revert rename on failure
			cli.ContainerRename(ctx, info.ID, originalName)
			return container.CreateResponse{}, fmt.Errorf("pulling image for recreate: %w", pullErr)
		}
		if _, copyErr := io.Copy(io.Discard, pullResp); copyErr != nil {
			slog.Warn("containers: image pull stream drain failed during recreate", "error", copyErr, "image", imgName, "container", info.ID)
		}
		if closeErr := pullResp.Close(); closeErr != nil {
			slog.Warn("containers: image pull stream close failed during recreate", "error", closeErr, "image", imgName, "container", info.ID)
		}
	}

	// Create new container with same config
	cfg := info.Config
	cfg.Image = imgName

	// Apply Config overrides from request
	if req.Env != nil {
		cfg.Env = req.Env
	}
	if req.Labels != nil {
		cfg.Labels = req.Labels
	}
	if req.Cmd != nil {
		cfg.Cmd = req.Cmd
	}
	if req.Entrypoint != nil {
		cfg.Entrypoint = req.Entrypoint
	}
	if req.WorkingDir != nil {
		cfg.WorkingDir = *req.WorkingDir
	}
	if req.Hostname != nil {
		cfg.Hostname = *req.Hostname
	}
	if req.User != nil {
		cfg.User = *req.User
	}
	if req.Domainname != nil {
		cfg.Domainname = *req.Domainname
	}
	if req.Tty != nil {
		cfg.Tty = *req.Tty
	}
	if req.OpenStdin != nil {
		cfg.OpenStdin = *req.OpenStdin
	}
	if req.StopSignal != nil {
		cfg.StopSignal = *req.StopSignal
	}
	if req.Healthcheck != nil {
		cfg.Healthcheck = &container.HealthConfig{
			Test:        req.Healthcheck.Test,
			Interval:    time.Duration(req.Healthcheck.Interval),
			Timeout:     time.Duration(req.Healthcheck.Timeout),
			Retries:     req.Healthcheck.Retries,
			StartPeriod: time.Duration(req.Healthcheck.StartPeriod),
		}
	}
	if req.ExposedPorts != nil {
		cfg.ExposedPorts = make(nat.PortSet)
		for k, v := range req.ExposedPorts {
			cfg.ExposedPorts[nat.Port(k)] = v
		}
	}

	// Apply HostConfig overrides from request
	hc := info.HostConfig
	if req.PortBindings != nil {
		portMap := make(nat.PortMap)
		for port, bindings := range req.PortBindings {
			pb := make([]nat.PortBinding, len(bindings))
			for i, b := range bindings {
				pb[i] = nat.PortBinding{HostIP: b.HostIP, HostPort: b.HostPort}
			}
			portMap[nat.Port(port)] = pb
		}
		hc.PortBindings = portMap
	}
	if req.NetworkMode != nil {
		hc.NetworkMode = container.NetworkMode(*req.NetworkMode)
	}
	if req.Privileged != nil {
		hc.Privileged = *req.Privileged
	}
	if req.ReadonlyRootfs != nil {
		hc.ReadonlyRootfs = *req.ReadonlyRootfs
	}
	if req.Dns != nil {
		hc.DNS = req.Dns
	}
	if req.DnsSearch != nil {
		hc.DNSSearch = req.DnsSearch
	}
	if req.CapAdd != nil {
		hc.CapAdd = req.CapAdd
	}
	if req.CapDrop != nil {
		hc.CapDrop = req.CapDrop
	}
	if req.DnsOptions != nil {
		hc.DNSOptions = req.DnsOptions
	}
	if req.ExtraHosts != nil {
		hc.ExtraHosts = req.ExtraHosts
	}
	if req.SecurityOpt != nil {
		hc.SecurityOpt = req.SecurityOpt
	}
	if req.ShmSize != nil {
		hc.ShmSize = *req.ShmSize
	}
	if req.PidMode != nil {
		hc.PidMode = container.PidMode(*req.PidMode)
	}
	if req.Init != nil {
		hc.Init = req.Init
	}
	if req.AutoRemove != nil {
		hc.AutoRemove = *req.AutoRemove
	}
	if req.OomKillDisable != nil {
		hc.OomKillDisable = req.OomKillDisable
	}
	if req.PidsLimit != nil {
		hc.PidsLimit = req.PidsLimit
	}
	if req.DeviceRequests != nil {
		hc.DeviceRequests = *req.DeviceRequests
	}
	if req.LogDriver != nil {
		hc.LogConfig.Type = *req.LogDriver
	}
	if req.LogOptions != nil {
		hc.LogConfig.Config = req.LogOptions
	}
	if req.Memory != nil {
		hc.Memory = *req.Memory
	}
	if req.NanoCPUs != nil {
		hc.NanoCPUs = *req.NanoCPUs
	}
	if req.RestartPolicy != nil {
		hc.RestartPolicy = *req.RestartPolicy
	}

	// Rebuild networking config from the inspected container
	var netConfig *networkTypes.NetworkingConfig
	if info.NetworkSettings != nil && len(info.NetworkSettings.Networks) > 0 {
		endpointsConfig := make(map[string]*networkTypes.EndpointSettings)
		for name, ep := range info.NetworkSettings.Networks {
			endpointsConfig[name] = &networkTypes.EndpointSettings{
				IPAMConfig:          ep.IPAMConfig,
				Links:               ep.Links,
				Aliases:             ep.Aliases,
				NetworkID:           ep.NetworkID,
				EndpointID:          ep.EndpointID,
				Gateway:             ep.Gateway,
				IPAddress:           ep.IPAddress,
				IPPrefixLen:         ep.IPPrefixLen,
				IPv6Gateway:         ep.IPv6Gateway,
				GlobalIPv6Address:   ep.GlobalIPv6Address,
				GlobalIPv6PrefixLen: ep.GlobalIPv6PrefixLen,
				MacAddress:          ep.MacAddress,
				DriverOpts:          ep.DriverOpts,
			}
		}
		netConfig = &networkTypes.NetworkingConfig{
			EndpointsConfig: endpointsConfig,
		}
	}

	newResp, err := cli.ContainerCreate(ctx, cfg, hc, netConfig, nil, originalName)
	if err != nil {
		// Revert: rename old container back
		cli.ContainerRename(ctx, info.ID, originalName)
		return container.CreateResponse{}, fmt.Errorf("creating replacement container: %w", err)
	}

	// Start new container
	if err := cli.ContainerStart(ctx, newResp.ID, container.StartOptions{}); err != nil {
		// Clean up: remove new container, rename old back (best-effort)
		if rmErr := cli.ContainerRemove(ctx, newResp.ID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("containers: failed to clean up new container after start failure", "error", rmErr, "container", newResp.ID)
		}
		if rnErr := cli.ContainerRename(ctx, info.ID, originalName); rnErr != nil {
			slog.Warn("containers: failed to rename old container back after start failure", "error", rnErr, "container", info.ID)
		}
		return container.CreateResponse{}, fmt.Errorf("starting replacement container: %w", err)
	}

	// Remove old container
	if err := cli.ContainerRemove(ctx, info.ID, container.RemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		slog.Warn("containers: failed to remove old container after recreate", "error", err, "container", info.ID)
	}

	return newResp, nil
}

func isSelfMcHarborContainer(info types.ContainerJSON) bool {
	name := strings.ToLower(strings.TrimPrefix(info.Name, "/"))
	imageName := ""
	labels := map[string]string{}
	if info.Config != nil {
		imageName = strings.ToLower(strings.TrimSpace(info.Config.Image))
		labels = info.Config.Labels
	}

	if name == "mcharbor" {
		return true
	}
	if strings.Contains(imageName, "therealmcsparrow/mcharbor") && !strings.Contains(imageName, "mcharbor-agent") {
		return true
	}
	if strings.EqualFold(labels["org.opencontainers.image.title"], "McHarbor") {
		return true
	}

	return false
}

// NetworkConnect connects a container to a Docker network.
func (s *Service) NetworkConnect(ctx context.Context, envID, id, network string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.NetworkConnect(ctx, network, id, nil)
}

// NetworkDisconnect disconnects a container from a Docker network.
func (s *Service) NetworkDisconnect(ctx context.Context, envID, id, network string, force bool) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.NetworkDisconnect(ctx, network, id, force)
}

// Logs returns container logs.
func (s *Service) Logs(ctx context.Context, envID, id string, query LogsQuery) (string, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stdout := query.Stdout
	stderr := query.Stderr
	if !stdout && !stderr {
		stdout = true
		stderr = true
	}

	tail := query.Tail
	if tail == "" {
		tail = "100"
	}

	opts := container.LogsOptions{
		ShowStdout: stdout,
		ShowStderr: stderr,
		Tail:       tail,
		Timestamps: true,
	}

	if query.Since != "" {
		opts.Since = query.Since
	}

	reader, err := cli.ContainerLogs(ctx, id, opts)
	if err != nil {
		return "", fmt.Errorf("fetching logs for %s: %w", id, err)
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}

	// Demux if container is not TTY
	info, inspectErr := cli.ContainerInspect(ctx, id)
	if inspectErr == nil && !info.Config.Tty {
		demuxed := docker.DemuxDockerStream(buf.Bytes())
		return demuxed.Stdout + demuxed.Stderr, nil
	}

	return buf.String(), nil
}

// Stats returns container resource usage statistics.
func (s *Service) Stats(ctx context.Context, envID, id string, stream bool) (io.ReadCloser, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	// For non-streaming (one-shot) stats, apply a deadline so the call
	// doesn't hang indefinitely. Streaming callers manage their own
	// context lifecycle.
	if !stream {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	resp, err := cli.ContainerStats(ctx, id, stream)
	if err != nil {
		return nil, fmt.Errorf("fetching stats for %s: %w", id, err)
	}

	return resp.Body, nil
}

// Top returns running processes in a container.
func (s *Service) Top(ctx context.Context, envID, id string) (container.ContainerTopOKBody, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return container.ContainerTopOKBody{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	top, err := cli.ContainerTop(ctx, id, nil)
	if err != nil {
		return container.ContainerTopOKBody{}, fmt.Errorf("getting top for %s: %w", id, err)
	}

	return top, nil
}

// DetectShells probes a container for available shells.
func (s *Service) DetectShells(ctx context.Context, envID, id string) ([]ShellResult, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	shells := []string{"/bin/bash", "/bin/sh", "/bin/zsh"}
	results := make([]ShellResult, 0, len(shells))

	for _, shell := range shells {
		available := s.probeShell(ctx, cli, id, shell)
		results = append(results, ShellResult{
			Shell:     shell,
			Available: available,
		})
	}

	return results, nil
}

// probeShell tests if a shell is available in the container by running a quick exec.
func (s *Service) probeShell(ctx context.Context, cli *client.Client, containerID, shell string) bool {
	execConfig := container.ExecOptions{
		Cmd:          []string{shell, "-c", "echo ok"},
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return false
	}

	hijack, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return false
	}
	defer hijack.Close()

	// Read output with a timeout
	execCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output bytes.Buffer
	go func() {
		io.Copy(&output, hijack.Reader)
		close(done)
	}()

	select {
	case <-done:
	case <-execCtx.Done():
		return false
	}

	// Check exec exit code
	inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return false
	}

	return inspect.ExitCode == 0
}

// ListFiles lists files and directories at the given path inside a container.
// Uses exec ls -la for speed, falls back to CopyFromContainer if exec fails.
// Agent environments use the agent exec protocol (ContainerExecAttach can't
// work through agent transport, and CopyFromContainer is too slow for large dirs).
func (s *Service) ListFiles(ctx context.Context, envID, id, dirPath string) ([]FileEntry, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	if s.pool.IsAgentEnv(envID) {
		// Agent: use detached exec + CopyFromContainer for the output file.
		// This works through standard HTTP (RoundTrip) without requiring
		// the agent exec protocol.
		return s.listFilesViaDetachedExec(ctx, envID, id, dirPath)
	}

	// Local/direct: try exec-based listing first (fast)
	entries, err := s.listFilesViaExec(ctx, cli, id, dirPath)
	if err == nil {
		return entries, nil
	}

	// Fall back to CopyFromContainer (works for containers without sh/ls)
	return s.listFilesViaCopy(ctx, cli, id, dirPath)
}

// listFilesViaDetachedExec runs ls -la as a detached exec, writes output to a
// temp file, then retrieves it via CopyFromContainer. All steps use standard
// HTTP through RoundTrip, so this works with any agent version.
func (s *Service) listFilesViaDetachedExec(ctx context.Context, envID, id, dirPath string) ([]FileEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	tmpFile := fmt.Sprintf("/tmp/.mcharbor-ls-%d", time.Now().UnixNano())

	// Create exec that writes ls output to a temp file
	execResp, err := cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd: []string{"/bin/sh", "-c", fmt.Sprintf("ls -la %s > %s 2>&1", shellQuote(dirPath), tmpFile)},
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	// Start exec detached — standard POST through RoundTrip
	if err := cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{Detach: true}); err != nil {
		return nil, fmt.Errorf("exec start: %w", err)
	}

	// Poll until exec finishes
	for {
		inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			return nil, fmt.Errorf("exec inspect: %w", err)
		}
		if !inspect.Running {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Read the output file via CopyFromContainer (single small file)
	reader, _, err := cli.CopyFromContainer(ctx, id, tmpFile)
	if err != nil {
		return nil, fmt.Errorf("copy output file: %w", err)
	}
	defer reader.Close()

	// Extract the file content from the tar
	tr := tar.NewReader(reader)
	_, err = tr.Next()
	if err != nil {
		return nil, fmt.Errorf("reading tar header: %w", err)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		return nil, fmt.Errorf("reading output: %w", err)
	}

	// Clean up temp file without tying the best-effort removal to request cancellation.
	cleanupCtxBase := context.WithoutCancel(ctx)
	go func() {
		cleanCtx, cleanCancel := context.WithTimeout(cleanupCtxBase, 5*time.Second)
		defer cleanCancel()
		cleanExec, cerr := cli.ContainerExecCreate(cleanCtx, id, container.ExecOptions{
			Cmd: []string{"/bin/sh", "-c", "rm -f " + tmpFile},
		})
		if cerr == nil {
			cli.ContainerExecStart(cleanCtx, cleanExec.ID, container.ExecStartOptions{Detach: true})
		}
	}()

	return parseLsOutput(string(data), dirPath), nil
}

// ansiRegex matches ANSI escape sequences in terminal output.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// stripAnsi removes ANSI escape codes from TTY output.
func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// listFilesViaExec runs ls -la inside the container and parses the output.
func (s *Service) listFilesViaExec(ctx context.Context, cli *client.Client, id, dirPath string) ([]FileEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	execResp, err := cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", "ls -la " + shellQuote(dirPath)},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, err
	}

	hijack, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, err
	}
	defer hijack.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan struct{})
	go func() {
		stdcopy.StdCopy(&stdout, &stderr, hijack.Reader)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Check exit code
	inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, err
	}
	if inspect.ExitCode != 0 {
		return nil, fmt.Errorf("ls exited with code %d: %s", inspect.ExitCode, stderr.String())
	}

	return parseLsOutput(stdout.String(), dirPath), nil
}

// parseLsOutput parses `ls -la` output into FileEntry slice.
// Format: mode links owner group size month day time/year name [-> target]
func parseLsOutput(output, dirPath string) []FileEntry {
	lines := strings.Split(output, "\n")
	entries := make([]FileEntry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		mode := fields[0]
		size, _ := strconv.ParseInt(fields[4], 10, 64) // safe: defaults to 0 on parse failure

		// Name starts at field 8 (after: mode, links, owner, group, size, month, day, time/year)
		nameStr := strings.Join(fields[8:], " ")

		// Skip . and ..
		if nameStr == "." || nameStr == ".." {
			continue
		}

		name := nameStr
		linkTarget := ""

		// Check for symlink: name -> target
		if idx := strings.Index(nameStr, " -> "); idx != -1 {
			name = nameStr[:idx]
			linkTarget = nameStr[idx+4:]
		}

		isDir := len(mode) > 0 && mode[0] == 'd'

		fullPath := dirPath
		if !strings.HasSuffix(fullPath, "/") {
			fullPath += "/"
		}
		fullPath += name

		entries = append(entries, FileEntry{
			Name:       name,
			Path:       fullPath,
			Size:       size,
			Mode:       mode,
			IsDir:      isDir,
			LinkTarget: linkTarget,
		})
	}

	return entries
}

// shellQuote wraps a path in single quotes for safe shell usage.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// listFilesViaCopy uses CopyFromContainer as fallback for containers without sh.
func (s *Service) listFilesViaCopy(ctx context.Context, cli *client.Client, id, dirPath string) ([]FileEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reader, _, err := cli.CopyFromContainer(ctx, id, dirPath)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such file or directory") ||
			strings.Contains(errMsg, "not a directory") ||
			strings.Contains(errMsg, "Could not find the file") {
			return []FileEntry{}, nil
		}
		return nil, fmt.Errorf("listing files in %s for %s: %w", dirPath, id, err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	entries := make([]FileEntry, 0, 64)
	const maxEntries = 1000

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			// If tar reading fails (e.g. timeout on huge dirs), return what we have
			break
		}

		name := header.Name

		// Skip the root directory entry itself
		if name == "" || name == "./" || name == "." {
			continue
		}

		// Only list immediate children (depth 1)
		trimmed := strings.TrimSuffix(strings.TrimPrefix(name, "./"), "/")
		if strings.Contains(trimmed, "/") {
			continue
		}

		fullPath := dirPath
		if !strings.HasSuffix(fullPath, "/") {
			fullPath += "/"
		}
		fullPath += trimmed

		entry := FileEntry{
			Name:    trimmed,
			Path:    fullPath,
			Size:    header.Size,
			Mode:    header.FileInfo().Mode().String(),
			IsDir:   header.Typeflag == tar.TypeDir,
			ModTime: header.ModTime.UTC().Format(time.RFC3339),
		}

		if header.Typeflag == tar.TypeSymlink {
			entry.LinkTarget = header.Linkname
		}

		entries = append(entries, entry)
		if len(entries) >= maxEntries {
			break
		}
	}

	return entries, nil
}

// DetectServices detects OS-level services (systemd, SysV, OpenRC, supervisord) inside a container.
func (s *Service) DetectServices(ctx context.Context, envID, id string) (ContainerServicesResult, error) {
	empty := ContainerServicesResult{Services: []ContainerService{}}

	cmd := `/bin/sh -c 'if command -v systemctl >/dev/null 2>&1; then echo "___INIT:systemd"; systemctl list-units --type=service --no-pager --no-legend --plain 2>/dev/null; elif command -v service >/dev/null 2>&1 && [ -d /etc/init.d ]; then echo "___INIT:sysvinit"; service --status-all 2>/dev/null; elif command -v rc-status >/dev/null 2>&1; then echo "___INIT:openrc"; rc-status -a 2>/dev/null; elif command -v supervisorctl >/dev/null 2>&1; then echo "___INIT:supervisord"; supervisorctl status 2>/dev/null; else echo "___INIT:none"; fi'`

	var output string
	var err error

	if s.pool.IsAgentEnv(envID) {
		output, err = s.execViaDetached(ctx, envID, id, cmd)
	} else {
		cli, cErr := s.getClient(envID)
		if cErr != nil {
			return empty, cErr
		}
		output, err = s.execViaAttach(ctx, cli, id, cmd)
	}
	if err != nil {
		return empty, err
	}

	return parseServicesOutput(output), nil
}

// execViaAttach runs a command inside a container using attached exec (local environments).
func (s *Service) execViaAttach(ctx context.Context, cli *client.Client, id, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	execResp, err := cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	hijack, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer hijack.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan struct{})
	go func() {
		stdcopy.StdCopy(&stdout, &stderr, hijack.Reader)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	return stdout.String(), nil
}

// execViaDetached runs a command via detached exec + CopyFromContainer (for agent environments).
func (s *Service) execViaDetached(ctx context.Context, envID, id, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}

	tmpFile := fmt.Sprintf("/tmp/.mcharbor-svc-%d", time.Now().UnixNano())

	execResp, err := cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd: []string{"/bin/sh", "-c", fmt.Sprintf("%s > %s 2>&1", cmd, tmpFile)},
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	if err := cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{Detach: true}); err != nil {
		return "", fmt.Errorf("exec start: %w", err)
	}

	for {
		inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			return "", fmt.Errorf("exec inspect: %w", err)
		}
		if !inspect.Running {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	reader, _, err := cli.CopyFromContainer(ctx, id, tmpFile)
	if err != nil {
		return "", fmt.Errorf("copy output file: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	if _, err = tr.Next(); err != nil {
		return "", fmt.Errorf("reading tar header: %w", err)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		return "", fmt.Errorf("reading output: %w", err)
	}

	cleanupCtxBase := context.WithoutCancel(ctx)
	go func() {
		cleanCtx, cleanCancel := context.WithTimeout(cleanupCtxBase, 5*time.Second)
		defer cleanCancel()
		cleanExec, cerr := cli.ContainerExecCreate(cleanCtx, id, container.ExecOptions{
			Cmd: []string{"/bin/sh", "-c", "rm -f " + tmpFile},
		})
		if cerr == nil {
			cli.ContainerExecStart(cleanCtx, cleanExec.ID, container.ExecStartOptions{Detach: true})
		}
	}()

	return string(data), nil
}

// parseServicesOutput parses the combined detection script output.
func parseServicesOutput(output string) ContainerServicesResult {
	empty := ContainerServicesResult{Services: []ContainerService{}}
	output = strings.TrimSpace(output)
	if output == "" {
		return empty
	}

	lines := strings.SplitN(output, "\n", 2)
	header := strings.TrimSpace(lines[0])

	if !strings.HasPrefix(header, "___INIT:") {
		return empty
	}

	initSystem := strings.TrimPrefix(header, "___INIT:")
	if initSystem == "none" || initSystem == "" {
		return empty
	}

	body := ""
	if len(lines) > 1 {
		body = lines[1]
	}

	var services []ContainerService
	switch initSystem {
	case "systemd":
		services = parseSystemdServices(body)
	case "sysvinit":
		services = parseSysVServices(body)
	case "openrc":
		services = parseOpenRCServices(body)
	case "supervisord":
		services = parseSupervisordServices(body)
	default:
		return empty
	}

	return ContainerServicesResult{
		InitSystem: initSystem,
		Services:   services,
	}
}

// parseSystemdServices parses `systemctl list-units --type=service --no-pager --no-legend --plain`.
// Format: UNIT LOAD ACTIVE SUB DESCRIPTION...
func parseSystemdServices(output string) []ContainerService {
	var services []ContainerService
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		active := fields[2] // "active", "inactive", "failed"
		sub := fields[3]    // "running", "exited", "dead", etc.

		status := active
		if active == "active" {
			status = sub // more specific: "running" vs "exited"
		}

		services = append(services, ContainerService{
			Name:   name,
			Status: status,
			Sub:    sub,
		})
	}
	return services
}

// parseSysVServices parses `service --status-all`.
// Format: [ + ] service_name  or  [ - ] service_name  or  [ ? ] service_name
func parseSysVServices(output string) []ContainerService {
	var services []ContainerService
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Expected: [ + ] sshd  or  [ - ] cron
		if len(line) < 7 || line[0] != '[' {
			continue
		}

		marker := line[2] // '+', '-', or '?'
		// Name starts after " ] "
		idx := strings.Index(line, "] ")
		if idx < 0 || idx+2 >= len(line) {
			continue
		}
		name := strings.TrimSpace(line[idx+2:])
		if name == "" {
			continue
		}

		var status string
		switch marker {
		case '+':
			status = "running"
		case '-':
			status = "stopped"
		default:
			status = "unknown"
		}

		services = append(services, ContainerService{
			Name:   name,
			Status: status,
		})
	}
	return services
}

// parseOpenRCServices parses `rc-status -a`.
// Format varies — runlevel headers followed by "service_name [ status ]"
func parseOpenRCServices(output string) []ContainerService {
	var services []ContainerService
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip runlevel headers (e.g., "Runlevel: default")
		if strings.HasPrefix(line, "Runlevel:") || strings.HasPrefix(line, "Dynamic") || strings.HasPrefix(line, "Crashed") {
			continue
		}

		// Format: "service_name                    [ started ]" or "[ stopped ]"
		bracketIdx := strings.LastIndex(line, "[")
		if bracketIdx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:bracketIdx])
		if name == "" {
			continue
		}

		statusPart := line[bracketIdx:]
		statusPart = strings.Trim(statusPart, "[] ")

		status := statusPart
		if status == "started" {
			status = "running"
		}

		services = append(services, ContainerService{
			Name:   name,
			Status: status,
		})
	}
	return services
}

// parseSupervisordServices parses `supervisorctl status`.
// Format: name STATE  pid uptime  or  name STATE  description
func parseSupervisordServices(output string) []ContainerService {
	var services []ContainerService
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		state := strings.ToLower(fields[1])

		status := state
		if state == "running" {
			status = "running"
		} else if state == "stopped" || state == "exited" {
			status = "stopped"
		} else if state == "fatal" || state == "backoff" {
			status = "failed"
		}

		services = append(services, ContainerService{
			Name:   name,
			Status: status,
		})
	}
	return services
}

// BulkStats returns a snapshot of CPU/memory/network/block stats for all running containers.
func (s *Service) BulkStats(ctx context.Context, envID string) ([]BulkContainerMetric, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// List running containers
	running, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("listing running containers: %w", err)
	}

	if len(running) == 0 {
		return []BulkContainerMetric{}, nil
	}

	type statsResult struct {
		metric BulkContainerMetric
		err    error
	}

	ch := make(chan statsResult, len(running))
	for _, c := range running {
		go func(id, name string) {
			resp, sErr := cli.ContainerStats(ctx, id, false)
			if sErr != nil {
				ch <- statsResult{err: sErr}
				return
			}
			defer resp.Body.Close()

			data, rErr := io.ReadAll(resp.Body)
			if rErr != nil {
				ch <- statsResult{err: rErr}
				return
			}

			metric := parseStatsJSON(data, id, name)
			ch <- statsResult{metric: metric}
		}(c.ID, strings.TrimPrefix(c.Names[0], "/"))
	}

	metrics := make([]BulkContainerMetric, 0, len(running))
	for range running {
		res := <-ch
		if res.err == nil {
			metrics = append(metrics, res.metric)
		}
	}

	return metrics, nil
}

// parseStatsJSON extracts metrics from a Docker stats JSON blob.
func parseStatsJSON(data []byte, id, name string) BulkContainerMetric {
	// Use encoding/json for parsing
	var raw struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     uint64 `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
			Stats struct {
				Cache uint64 `json:"cache"`
			} `json:"stats"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
		BlkioStats struct {
			IOServiceBytesRecursive []struct {
				Op    string `json:"op"`
				Value uint64 `json:"value"`
			} `json:"io_service_bytes_recursive"`
		} `json:"blkio_stats"`
		PidsStats struct {
			Current uint64 `json:"current"`
		} `json:"pids_stats"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return BulkContainerMetric{ID: id, Name: name}
	}

	// CPU percent
	var cpuPercent float64
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(raw.CPUStats.OnlineCPUs) * 100.0
	}

	// Memory
	memUsage := int64(raw.MemoryStats.Usage - raw.MemoryStats.Stats.Cache)
	if memUsage < 0 {
		memUsage = int64(raw.MemoryStats.Usage)
	}
	memLimit := int64(raw.MemoryStats.Limit)
	var memPercent float64
	if memLimit > 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Network
	var netRx, netTx int64
	for _, n := range raw.Networks {
		netRx += int64(n.RxBytes)
		netTx += int64(n.TxBytes)
	}

	// Block I/O
	var blockRead, blockWrite int64
	for _, entry := range raw.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			blockRead += int64(entry.Value)
		case "write":
			blockWrite += int64(entry.Value)
		}
	}

	return BulkContainerMetric{
		ID:         id,
		Name:       name,
		CPUPercent: cpuPercent,
		MemUsage:   memUsage,
		MemLimit:   memLimit,
		MemPercent: memPercent,
		NetRx:      netRx,
		NetTx:      netTx,
		BlockRead:  blockRead,
		BlockWrite: blockWrite,
		PIDs:       raw.PidsStats.Current,
	}
}

// PruneContainers removes stopped containers.
func (s *Service) Prune(ctx context.Context, envID string) (container.PruneReport, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return container.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	report, err := cli.ContainersPrune(ctx, filters.Args{})
	if err != nil {
		return container.PruneReport{}, fmt.Errorf("pruning containers: %w", err)
	}

	return report, nil
}

// execCommand runs a command inside a container, handling both agent and local environments.
// Returns stdout output and any error.
func (s *Service) execCommand(ctx context.Context, envID, id string, cmd []string) (string, error) {
	shellCmd := strings.Join(cmd, " ")
	if s.pool.IsAgentEnv(envID) {
		return s.execViaDetached(ctx, envID, id, shellCmd)
	}
	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}
	return s.execViaAttach(ctx, cli, id, shellCmd)
}

// copyContentToContainer writes content as a single file into a container using CopyToContainer.
func (s *Service) copyContentToContainer(ctx context.Context, cli *client.Client, id, destDir, fileName string, content []byte, mode os.FileMode) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: fileName,
		Mode: int64(mode),
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("writing tar body: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar: %w", err)
	}

	return cli.CopyToContainer(ctx, id, destDir, &buf, container.CopyToContainerOptions{})
}

// FileContent reads a file from a container and returns its content.
func (s *Service) FileContent(ctx context.Context, envID, id, filePath string) ([]byte, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reader, _, err := cli.CopyFromContainer(ctx, id, filePath)
	if err != nil {
		return nil, fmt.Errorf("copying file from container: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("reading tar header: %w", err)
	}

	const maxFileSize = 10 * 1024 * 1024 // 10MB
	lr := io.LimitReader(tr, maxFileSize+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("reading file content: %w", err)
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("file exceeds maximum size of 10MB")
	}

	return data, nil
}

// SaveFileContent writes content to a file inside a container.
func (s *Service) SaveFileContent(ctx context.Context, envID, id, filePath string, content []byte) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	dir := path.Dir(filePath)
	fileName := path.Base(filePath)

	return s.copyContentToContainer(ctx, cli, id, dir, fileName, content, 0644)
}

// UploadFile uploads a file from a reader into a container directory.
func (s *Service) UploadFile(ctx context.Context, envID, id, destDir, fileName string, reader io.Reader, size int64) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: size,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := io.Copy(tw, reader); err != nil {
		return fmt.Errorf("writing tar body: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar: %w", err)
	}

	return cli.CopyToContainer(ctx, id, destDir, &buf, container.CopyToContainerOptions{})
}

// CreateDirectory creates a directory (including parents) inside a container.
func (s *Service) CreateDirectory(ctx context.Context, envID, id, dirPath string) error {
	_, err := s.execCommand(ctx, envID, id, []string{"mkdir", "-p", shellQuote(dirPath)})
	return err
}

// RenameFile renames or moves a file/directory inside a container.
func (s *Service) RenameFile(ctx context.Context, envID, id, oldPath, newPath string) error {
	_, err := s.execCommand(ctx, envID, id, []string{"mv", shellQuote(oldPath), shellQuote(newPath)})
	return err
}

// ChangePermissions changes the file mode of a path inside a container.
func (s *Service) ChangePermissions(ctx context.Context, envID, id, filePath, mode string) error {
	_, err := s.execCommand(ctx, envID, id, []string{"chmod", mode, shellQuote(filePath)})
	return err
}

// DeleteFile removes a file or directory inside a container.
func (s *Service) DeleteFile(ctx context.Context, envID, id, filePath string, recursive bool) error {
	cmd := []string{"rm", "-f", shellQuote(filePath)}
	if recursive {
		cmd = []string{"rm", "-rf", shellQuote(filePath)}
	}
	_, err := s.execCommand(ctx, envID, id, cmd)
	return err
}

// ImageUpdateResult holds the update-check result for a single container.
type ImageUpdateResult struct {
	ContainerID     string `json:"containerId"`
	ContainerName   string `json:"containerName"`
	Image           string `json:"image"`
	CurrentDigest   string `json:"currentDigest"`
	RemoteDigest    string `json:"remoteDigest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Error           string `json:"error,omitempty"`
}

// CheckImageUpdates compares local image digests with remote registry digests
// for each running container. Uses Docker's DistributionInspect under the hood.
func (s *Service) CheckImageUpdates(ctx context.Context, envID string, containerIDs []string) ([]ImageUpdateResult, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	// If no specific containers requested, get all running containers
	var targets []ContainerSummary
	if len(containerIDs) == 0 {
		all, err := s.List(ctx, envID, false) // only running
		if err != nil {
			return nil, fmt.Errorf("listing containers: %w", err)
		}
		targets = all
	} else {
		// Fetch only the requested containers
		all, err := s.List(ctx, envID, true)
		if err != nil {
			return nil, fmt.Errorf("listing containers: %w", err)
		}
		idSet := make(map[string]bool, len(containerIDs))
		for _, id := range containerIDs {
			idSet[id] = true
		}
		for _, c := range all {
			if idSet[c.ID] {
				targets = append(targets, c)
			}
		}
	}

	results := make([]ImageUpdateResult, len(targets))
	var wg sync.WaitGroup
	for i, c := range targets {
		wg.Add(1)
		go func(idx int, ctr ContainerSummary) {
			defer wg.Done()

			name := ctr.ID[:12]
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}

			result := ImageUpdateResult{
				ContainerID:   ctr.ID,
				ContainerName: name,
				Image:         ctr.Image,
			}

			// Get local image digest
			inspCtx, inspCancel := context.WithTimeout(ctx, 30*time.Second)
			defer inspCancel()

			imgInspect, _, err := cli.ImageInspectWithRaw(inspCtx, ctr.ImageID)
			if err != nil {
				result.Error = "inspect failed"
				results[idx] = result
				return
			}

			// Get the first RepoDigest as the current local digest
			if len(imgInspect.RepoDigests) > 0 {
				for _, rd := range imgInspect.RepoDigests {
					parts := strings.SplitN(rd, "@", 2)
					if len(parts) == 2 {
						result.CurrentDigest = parts[1]
						break
					}
				}
			}

			// Query the remote registry for the image reference
			distCtx, distCancel := context.WithTimeout(ctx, 30*time.Second)
			defer distCancel()

			// Use the image reference (e.g., "nginx:latest") for the registry query
			ref := ctr.Image
			distInspect, err := cli.DistributionInspect(distCtx, ref, "")
			if err != nil {
				result.Error = "registry check failed"
				results[idx] = result
				return
			}

			result.RemoteDigest = string(distInspect.Descriptor.Digest)

			// Compare digests
			if result.CurrentDigest != "" && result.RemoteDigest != "" {
				result.UpdateAvailable = result.CurrentDigest != result.RemoteDigest
			}

			results[idx] = result
		}(i, c)
	}

	wg.Wait()
	return results, nil
}
