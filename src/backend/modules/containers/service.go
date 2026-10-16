// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	cerrdefs "github.com/containerd/errdefs"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	networkTypes "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// longDockerOpTimeout bounds the Docker operations that legitimately outrun
// docker.CallTimeout: a create that pulls, a move that copies a layer set.
const longDockerOpTimeout = 60 * time.Second

const recreateOperationLabel = "com.mcharbor.recreate-operation"

// SecretResolver substitutes ${secret:NAME} references in a container's
// environment immediately before the Docker call. Held behind an interface so
// this module does not depend on the vault to compile.
type SecretResolver interface {
	ResolveSlice(ctx context.Context, env []string, scope secrets.ResolveScope) ([]string, error)
}

// Service wraps Docker SDK container operations.
type Service struct {
	pool     *docker.ClientPool
	db       *sql.DB
	dataDir  string
	resolver SecretResolver
	// stats is the shared live stats cache. Nil (in tests) means BulkStats
	// sweeps the daemon itself.
	stats *statshub.Hub
	// pinned are the root-of-trust values a recreate must not let an editor move.
	pinned []string
}

// SetStatsHub makes BulkStats read the shared cache instead of sweeping every
// container per request — per open tab, every five seconds.
func (s *Service) SetStatsHub(hub *statshub.Hub) { s.stats = hub }

// SetSecretResolver injects the vault resolver. Called once from main.go.
func (s *Service) SetSecretResolver(r SecretResolver) { s.resolver = r }

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}

var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

const agentRecreateHelperImage = "docker:27-cli"

// NewService creates a new container service.
func NewService(pool *docker.ClientPool, db *sql.DB, dataDir string, enc *encryption.Service) *Service {
	svc := &Service{pool: pool, db: db, dataDir: dataDir}
	if db != nil && enc != nil {
		svc.resolver = secrets.NewService(db, enc, nil)
	}
	return svc
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(ctx context.Context, envID string) (*client.Client, error) {
	c, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

func normalizeContainerRenameName(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "/")
}

func isValidContainerRenameName(name string) bool {
	return containerNamePattern.MatchString(name)
}

type containerStackLink struct {
	StackName    string
	StackService string
}

func (s *Service) stackLinksByContainer(ctx context.Context, envID string) (map[string]containerStackLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT container_id, stack_name, COALESCE(service_name, '')
		FROM container_stack_links
		WHERE environment_id = ?
		LIMIT 1000
	`, envID)
	if err != nil {
		return nil, fmt.Errorf("querying container stack links: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

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
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	containersRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: all})
	containers := containersRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	links, err := s.stackLinksByContainer(ctx, envID)
	if err != nil {
		return nil, err
	}

	result := make([]ContainerSummary, 0, len(containers))
	// The row flags are advisory (the handlers re-check), so an undetectable daemon just
	// leaves HostAccess empty instead of failing the list.
	selfHost, selfHostErr := s.pool.SelfHost(ctx, envID)
	if selfHostErr != nil {
		slog.Debug("containers: self-host detection failed for list", "env", envID, "error", selfHostErr)
	}
	for _, c := range containers {
		ports := make([]PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				IP:          addrString(p.IP),
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
					IPAddress:  addrString(ep.IPAddress),
					Gateway:    addrString(ep.Gateway),
					MacAddress: ep.MacAddress.String(),
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
			State:           string(c.State),
			Status:          c.Status,
			Ports:           ports,
			Labels:          c.Labels,
			NetworkSettings: netSettings,
			Mounts:          stripSummarySecretMounts(mounts),
			Protected:       docker.IsProtectedContainer(c.Names, c.Image, c.Labels),
		}
		summary.SelfTarget = string(docker.ClassifySummary(c.ID, c.Names, c.Image, c.Labels))
		if summary.SelfTarget != "" {
			summary.Protected = true
		} else if selfHost.Hosted {
			summary.HostAccess = summaryHostAccess(c.Mounts, selfHost)
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
func (s *Service) Inspect(ctx context.Context, envID, id string) (container.InspectResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.InspectResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	infoRes, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, fmt.Errorf("inspecting container %s: %w", id, err)
	}
	info := infoRes.Container

	return info, nil
}

// Create creates a new container.
func (s *Service) Create(ctx context.Context, envID string, req CreateRequest) (container.CreateResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.CreateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	// The reference is what the caller stored; the plaintext exists only from
	// here to the Docker call — or, in file mode, not even there.
	secretRefs := captureSecretRefs(req.Env)
	resolvedEnv, secretMount, err := s.prepareSecrets(ctx, cli, envID, req.Name, req.Image, req.Env, req.SecretFileMode)
	if err != nil {
		return container.CreateResponse{}, err
	}
	// Kept on the container because Config.Env only ever holds the resolved
	// value: without this the reference is unrecoverable, and file mode could
	// never be switched on or off again on an existing container.
	if encoded := encodeSecretRefs(secretRefs); encoded != "" {
		if req.Labels == nil {
			req.Labels = map[string]string{}
		}
		req.Labels[secretRefsLabel] = encoded
	}
	if secretMount != nil {
		if req.HostConfig == nil {
			req.HostConfig = &container.HostConfig{}
		}
		req.HostConfig.Mounts = append(req.HostConfig.Mounts, *secretMount)
		// Recorded on the container so Recreate keeps the choice: the request
		// that made it is long gone by then, and the only durable record of
		// how this container was built is its own config.
		if req.Labels == nil {
			req.Labels = map[string]string{}
		}
		req.Labels[secretFileModeLabel] = "true"
	}

	config := &container.Config{
		Image:        req.Image,
		Cmd:          req.Cmd,
		Env:          resolvedEnv,
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
	if err := s.checkHostAccess(ctx, envID, hostConfig); err != nil {
		return container.CreateResponse{}, err
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: config, HostConfig: hostConfig, NetworkingConfig: req.NetworkConfig, Name: req.Name})
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("creating container: %w", err)
	}

	// The references were written against the name a moment ago, because the
	// id did not exist yet. Move them now, so a later rename does not orphan
	// them and the impact list can address this container directly.
	s.bindSecretReferences(ctx, req.Name, resp.ID, "")

	return container.CreateResponse{ID: resp.ID, Warnings: resp.Warnings}, nil
}

// Remove removes a container and optionally cleans up its bind mounts.
func (s *Service) Remove(ctx context.Context, envID, id string, force, removeVolumes bool) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, longDockerOpTimeout)
	defer cancel()

	// Inspect first to collect bind mount paths before the container is gone.
	var bindMounts []string
	infoRes, inspErr := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	info := infoRes.Container
	if inspErr == nil && docker.ClassifyContainer(info) != docker.SelfTargetNone {
		return docker.ErrProtectedResource
	}
	if inspErr == nil && info.Mounts != nil {
		for _, m := range info.Mounts {
			if m.Type == "bind" {
				bindMounts = append(bindMounts, m.Source)
			}
		}
	}

	_, err = cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{
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

	// A removed container is not stale, it is gone. Leaving its references
	// behind would have the impact list asking to restart something that no
	// longer exists, which is how a useful list becomes one people dismiss.
	s.clearSecretReferences(ctx, id, strings.TrimPrefix(info.Name, "/"))

	return nil
}

// clearSecretReferences drops a removed container's references under both the
// identifiers it may have been recorded with: the id, and the name that rows
// written before the rebind still carry.
func (s *Service) clearSecretReferences(ctx context.Context, id, name string) {
	binder, ok := s.resolver.(secretReferenceBinder)
	if !ok {
		return
	}
	for _, identifier := range []string{id, name} {
		if identifier == "" {
			continue
		}
		if err := binder.ClearReferences(ctx, "container", identifier); err != nil {
			slog.Warn("containers: clearing secret references failed",
				"container", identifier, "error", err)
		}
	}
}

// RemoveImage removes a Docker image by ID.
func (s *Service) RemoveImage(ctx context.Context, envID, imageID string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	_, err = cli.ImageRemove(ctx, imageID, client.ImageRemoveOptions{PruneChildren: true})
	if err != nil {
		return fmt.Errorf("removing image %s: %w", imageID, err)
	}

	return nil
}

// Start starts a stopped container.
func (s *Service) Start(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", id, err)
	}

	return nil
}

// Stop stops a running container with an optional timeout.
func (s *Service) Stop(ctx context.Context, envID, id string, timeout int) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	stopOpts := client.ContainerStopOptions{}
	if timeout > 0 {
		stopOpts.Timeout = &timeout
	}

	if _, err := cli.ContainerStop(ctx, id, stopOpts); err != nil {
		return fmt.Errorf("stopping container %s: %w", id, err)
	}

	return nil
}

// Restart restarts a container.
func (s *Service) Restart(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	if _, err := cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{}); err != nil {
		return fmt.Errorf("restarting container %s: %w", id, err)
	}

	return nil
}

// Pause pauses a running container.
func (s *Service) Pause(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	if _, err := cli.ContainerPause(ctx, id, client.ContainerPauseOptions{}); err != nil {
		return fmt.Errorf("pausing container %s: %w", id, err)
	}

	return nil
}

// Unpause unpauses a paused container.
func (s *Service) Unpause(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	if _, err := cli.ContainerUnpause(ctx, id, client.ContainerUnpauseOptions{}); err != nil {
		return fmt.Errorf("unpausing container %s: %w", id, err)
	}

	return nil
}

// Kill sends a signal to a container.
func (s *Service) Kill(ctx context.Context, envID, id, signal string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	if signal == "" {
		signal = "SIGKILL"
	}

	if _, err := cli.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: signal}); err != nil {
		return fmt.Errorf("killing container %s: %w", id, err)
	}

	return nil
}

// Update updates runtime resources for a container.
func (s *Service) Update(ctx context.Context, envID, id string, req UpdateRequest) (container.UpdateResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.UpdateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := ensureContainerMutableForRuntimeEdit(ctx, cli, id, req.UnlockProtected); err != nil {
		return container.UpdateResponse{}, err
	}

	updateConfig := container.UpdateConfig{
		Resources: container.Resources{},
	}
	if req.Memory != nil {
		updateConfig.Memory = *req.Memory
	}
	if req.MemorySwap != nil {
		updateConfig.MemorySwap = *req.MemorySwap
	}
	if req.MemoryReservation != nil {
		updateConfig.MemoryReservation = *req.MemoryReservation
	}
	if req.NanoCPUs != nil {
		updateConfig.NanoCPUs = *req.NanoCPUs
	}
	if req.CPUShares != nil {
		updateConfig.CPUShares = *req.CPUShares
	}
	if req.CPUQuota != nil {
		updateConfig.CPUQuota = *req.CPUQuota
	}
	if req.CPUPeriod != nil {
		updateConfig.CPUPeriod = *req.CPUPeriod
	}
	if req.CpusetCpus != nil {
		updateConfig.CpusetCpus = *req.CpusetCpus
	}
	if req.CpusetMems != nil {
		updateConfig.CpusetMems = *req.CpusetMems
	}
	if req.BlkioWeight != nil {
		updateConfig.BlkioWeight = *req.BlkioWeight
	}

	if req.RestartPolicy != nil {
		updateConfig.RestartPolicy = *req.RestartPolicy
	}

	resp, err := cli.ContainerUpdate(ctx, id, client.ContainerUpdateOptions{
		Resources:     &updateConfig.Resources,
		RestartPolicy: &updateConfig.RestartPolicy,
	})
	if err != nil {
		return container.UpdateResponse{}, fmt.Errorf("updating container %s: %w", id, err)
	}

	return container.UpdateResponse{Warnings: resp.Warnings}, nil
}

func ensureContainerMutableForRuntimeEdit(ctx context.Context, cli *client.Client, id string, unlockProtected bool) error {
	if !unlockProtected {
		return docker.EnsureContainerMutable(ctx, cli, id)
	}

	infoRes, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	switch docker.ClassifyContainer(infoRes.Container) {
	case docker.SelfTargetNone, docker.SelfTargetApp:
		return nil
	default:
		return docker.ErrProtectedResource
	}
}

// Rename changes the Docker container name.
func (s *Service) Rename(ctx context.Context, envID, id, name string, unlockProtected bool) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := ensureContainerMutableForRuntimeEdit(ctx, cli, id, unlockProtected); err != nil {
		return err
	}

	if _, err := cli.ContainerRename(ctx, id, client.ContainerRenameOptions{NewName: name}); err != nil {
		return fmt.Errorf("renaming container %s: %w", id, err)
	}

	return nil
}

// Recreate stops, renames, creates a new container with the same config, starts it, and removes the old one.
func (s *Service) Recreate(ctx context.Context, envID, id string, req RecreateRequest) (container.CreateResponse, error) {
	return s.recreate(ctx, envID, id, req, nil)
}

// RecreateWithMounts is Recreate for the configuration editor, which may also replace the
// container's mounts. The list is validated before anything is stopped.
func (s *Service) RecreateWithMounts(ctx context.Context, envID, id string, req RecreateEditRequest) (container.CreateResponse, error) {
	if req.Mounts != nil {
		if err := validateMountEdits(*req.Mounts); err != nil {
			return container.CreateResponse{}, err
		}
	}
	return s.recreate(ctx, envID, id, req.RecreateRequest, req.Mounts)
}

func (s *Service) recreate(ctx context.Context, envID, id string, req RecreateRequest, mounts *[]MountEdit) (container.CreateResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.CreateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	// Inspect current container
	infoRes, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("inspecting container for recreate: %w", err)
	}
	info := infoRes.Container
	imageName := ""
	labels := map[string]string{}
	if info.Config != nil {
		imageName = info.Config.Image
		labels = info.Config.Labels
		if err := s.guardMovedWithheld(req, info); err != nil {
			return container.CreateResponse{}, err
		}
		// The editor was given a redacted list; every value still carrying the marker is
		// one the operator did not change, and must be written back as it was.
		req.Env = envredact.Restore(req.Env, info.Config.Env)
	}
	// Every recreate of a McHarbor container rebuilds its keys and data volume onto
	// whatever the request names, so it needs host_self.manage whatever UnlockProtected says.
	if kind := docker.ClassifyContainer(info); kind != docker.SelfTargetNone {
		if err := requireHostSelf(ctx, i18n.ErrHostSelfContainerRequired, string(kind), nil); err != nil {
			return container.CreateResponse{}, err
		}
	}

	// Resolve ${secret:NAME} once, here, before anything branches. Every path
	// out of Recreate — the self-recreate helper, the agent path, the ordinary
	// replacement — reads req.Env, so resolving in one place is what covers all
	// three without duplication.
	//
	// File mode defaults to however the container being replaced was built: a
	// recreate preserves that, and silently dropping back to environment
	// injection would put credentials back somewhere the operator had
	// deliberately taken them out of. An explicit flag in the request is the
	// operator changing their mind, which is the one thing that overrides it.
	secretOwner := strings.TrimPrefix(info.Name, "/")
	fileMode := labels[secretFileModeLabel] == "true"
	if req.SecretFileMode != nil {
		fileMode = *req.SecretFileMode
	}
	// Config.Env holds resolved values, so the references have to come back
	// from the label before anything can be re-planned from them.
	secretRefs := decodeSecretRefs(labels[secretRefsLabel])
	if refs := captureSecretRefs(req.Env); len(refs) > 0 {
		secretRefs = refs
	}
	var secretMount *mount.Mount
	secretEnv := req.Env
	if secretEnv == nil && len(secretRefs) > 0 && info.Config != nil {
		secretEnv = info.Config.Env
	}
	if secretEnv != nil {
		secretEnv = restoreSecretRefs(secretEnv, secretRefs)
		resolvedEnv, m, resolveErr := s.prepareSecrets(ctx, cli, envID, secretOwner, imageName, secretEnv, fileMode)
		if resolveErr != nil {
			return container.CreateResponse{}, resolveErr
		}
		req.Env = resolvedEnv
		secretMount = m
	}
	if kind := docker.ClassifyContainer(info); kind != docker.SelfTargetNone {
		// McHarbor's own mounts hold its keys and data volume; the editor does not rewrite them.
		if mounts != nil {
			return container.CreateResponse{}, docker.ErrProtectedResource
		}
		// The McHarbor container recreates itself through a detached helper, so
		// it is editable — but only deliberately. The unlock flag is the
		// operator confirming the unlock dialog; without it the protection
		// stands, exactly as ensureContainerMutableForRuntimeEdit treats it.
		if kind == docker.SelfTargetApp {
			if !req.UnlockProtected {
				return container.CreateResponse{}, docker.ErrProtectedResource
			}
			return s.recreateSelfContainer(ctx, envID, cli, info, req)
		}
		return container.CreateResponse{}, docker.ErrProtectedResource
	}

	originalName := strings.TrimPrefix(info.Name, "/")
	if info.HostConfig != nil && info.HostConfig.AutoRemove {
		return container.CreateResponse{}, fmt.Errorf("recreating auto-remove containers is not supported")
	}
	if s.pool.IsAgentEnvContext(ctx, envID) && docker.IsAgentContainer([]string{info.Name}, imageName, labels) {
		// The connected agent's mounts are its link to the host; an edit could cut it off.
		if mounts != nil {
			return container.CreateResponse{}, docker.ErrProtectedResource
		}
		return s.recreateConnectedAgentContainer(ctx, cli, info, originalName, req)
	}

	// The preview is the full replacement spec, edited mounts included, so the host-access
	// guard sees a newly bound host path exactly as it would on create.
	if _, previewHC, _, specErr := replacementContainerSpec(info, req, imageName, mounts); specErr == nil {
		if err := s.checkHostAccess(ctx, envID, previewHC); err != nil {
			return container.CreateResponse{}, err
		}
	}

	wasRunning := info.State.Running
	wasPaused := info.State.Paused
	if wasPaused {
		if _, err := cli.ContainerUnpause(ctx, id, client.ContainerUnpauseOptions{}); err != nil {
			if restoreErr := restoreContainerAfterRecreate(ctx, cli, info, originalName, "", "", ""); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restoring container after ambiguous unpause: %w", restoreErr))
			}
			return container.CreateResponse{}, fmt.Errorf("unpausing container for recreate: %w", err)
		}
	}
	if wasRunning {
		timeout := 10
		if _, err := cli.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
			if restoreErr := restoreContainerAfterRecreate(ctx, cli, info, originalName, "", "", ""); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restoring container after ambiguous stop: %w", restoreErr))
			}
			return container.CreateResponse{}, fmt.Errorf("stopping container for recreate: %w", err)
		}
	}

	operationID := xid.New().String()
	shortID := info.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	replacementName := "mcharbor-recreate-" + shortID + "-" + operationID
	oldName := originalName + "-old-" + fmt.Sprintf("%d", time.Now().Unix())
	if _, err := cli.ContainerRename(ctx, id, client.ContainerRenameOptions{NewName: oldName}); err != nil {
		if restoreErr := restoreContainerAfterRecreate(ctx, cli, info, originalName, "", "", ""); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("rolling back recreate: %w", restoreErr))
		}
		return container.CreateResponse{}, fmt.Errorf("renaming container for recreate: %w", err)
	}
	restoreOriginal := func(replacementID string) error {
		return restoreContainerAfterRecreate(ctx, cli, info, originalName, replacementID, replacementName, operationID)
	}
	failRecreate := func(operation string, cause error, replacementID string) (container.CreateResponse, error) {
		if restoreErr := restoreOriginal(replacementID); restoreErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rolling back recreate: %w", restoreErr))
		}
		return container.CreateResponse{}, fmt.Errorf("%s: %w", operation, cause)
	}

	// Optionally update image
	imgName := info.Config.Image
	if req.Image != "" {
		imgName = req.Image
	}

	// Pull image if requested
	if req.PullImage {
		pullResp, pullErr := cli.ImagePull(ctx, imgName, client.ImagePullOptions{})
		if pullErr != nil {
			return failRecreate("pulling image for recreate", pullErr, "")
		}
		if _, copyErr := io.Copy(io.Discard, pullResp); copyErr != nil {
			slog.Warn("containers: image pull stream drain failed during recreate", "error", copyErr, "image", imgName, "container", info.ID)
		}
		if closeErr := pullResp.Close(); closeErr != nil {
			slog.Warn("containers: image pull stream close failed during recreate", "error", closeErr, "image", imgName, "container", info.ID)
		}
	}

	cfg, hc, netConfig, err := replacementContainerSpec(info, req, imgName, mounts)
	if err != nil {
		return failRecreate("preparing replacement container", err, "")
	}
	labels = make(map[string]string, len(cfg.Labels)+1)
	for key, value := range cfg.Labels {
		labels[key] = value
	}
	labels[recreateOperationLabel] = operationID
	// The replacement is built from the old container's spec, so the previous
	// mount and the previous labels arrive with it. Both have to be rewritten
	// rather than merged, or turning file mode off would leave the volume
	// mounted and the label claiming it is still on.
	hc.Mounts = withoutSecretMount(hc.Mounts)
	delete(labels, secretFileModeLabel)
	delete(labels, secretRefsLabel)
	if secretMount != nil {
		hc.Mounts = append(hc.Mounts, *secretMount)
		labels[secretFileModeLabel] = "true"
	}
	if encoded := encodeSecretRefs(secretRefs); encoded != "" {
		labels[secretRefsLabel] = encoded
	}
	cfg.Labels = labels

	newResp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: cfg, HostConfig: hc, NetworkingConfig: netConfig, Name: replacementName})
	if err != nil {
		return failRecreate("creating replacement container", err, "")
	}
	if _, err := cli.ContainerRename(ctx, newResp.ID, client.ContainerRenameOptions{NewName: originalName}); err != nil {
		return failRecreate("naming replacement container", err, newResp.ID)
	}

	if _, err := cli.ContainerStart(ctx, newResp.ID, client.ContainerStartOptions{}); err != nil {
		return failRecreate("starting replacement container", err, newResp.ID)
	}
	if wasPaused {
		if _, err := cli.ContainerPause(ctx, newResp.ID, client.ContainerPauseOptions{}); err != nil {
			return failRecreate("pausing replacement container", err, newResp.ID)
		}
	}

	// Remove old container
	if _, err := cli.ContainerRemove(ctx, info.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
		slog.Warn("containers: failed to remove old container after recreate", "error", err, "container", info.ID)
	}

	// Only once the old container is gone is the volume detached and actually
	// removable. Leaving it would leave the credentials readable on the host
	// after the operator asked for file mode to be turned off.
	if !fileMode {
		if err := docker.RemoveSecretVolume(ctx, cli, secretOwner); err != nil {
			slog.Warn("containers: failed to remove secret volume after recreate", "error", err, "container", secretOwner)
		}
	}

	s.bindSecretReferences(ctx, originalName, newResp.ID, info.ID)

	return container.CreateResponse{ID: newResp.ID, Warnings: newResp.Warnings}, nil
}

// withoutSecretMount strips the /run/secrets mount from a spec inherited from
// the container being replaced, identified by its target rather than its
// source so a volume named under an older scheme is still recognised.
func withoutSecretMount(mounts []mount.Mount) []mount.Mount {
	out := mounts[:0]
	for _, m := range mounts {
		if m.Target == secrets.SecretMountPath {
			continue
		}
		out = append(out, m)
	}
	return out
}

func restoreContainerAfterRecreate(ctx context.Context, cli *client.Client, original container.InspectResponse, originalName, replacementID, replacementName, operationID string) error {
	var restoreErrs []error
	if replacementID == "" && replacementName != "" {
		inspectCtx, inspectCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		candidateRes, err := cli.ContainerInspect(inspectCtx, replacementName, client.ContainerInspectOptions{})
		candidate := candidateRes.Container
		inspectCancel()
		if err == nil && candidate.ID != original.ID {
			if candidate.Config != nil && candidate.Config.Labels[recreateOperationLabel] == operationID {
				replacementID = candidate.ID
			} else {
				restoreErrs = append(restoreErrs, fmt.Errorf("temporary replacement ownership could not be verified"))
			}
		} else if err != nil && !cerrdefs.IsNotFound(err) {
			restoreErrs = append(restoreErrs, fmt.Errorf("inspecting ambiguous replacement: %w", err))
		}
	}
	if replacementID != "" {
		removeCtx, removeCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		_, err := cli.ContainerRemove(removeCtx, replacementID, client.ContainerRemoveOptions{Force: true})
		removeCancel()
		if err != nil && !cerrdefs.IsNotFound(err) {
			restoreErrs = append(restoreErrs, fmt.Errorf("removing failed replacement: %w", err))
		}
	}

	inspectCtx, inspectCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
	currentRes, inspectErr := cli.ContainerInspect(inspectCtx, original.ID, client.ContainerInspectOptions{})
	current := currentRes.Container
	inspectCancel()
	if inspectErr != nil {
		restoreErrs = append(restoreErrs, fmt.Errorf("inspecting original container during rollback: %w", inspectErr))
	}
	if inspectErr != nil || strings.TrimPrefix(current.Name, "/") != originalName {
		renameCtx, renameCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		_, err := cli.ContainerRename(renameCtx, original.ID, client.ContainerRenameOptions{NewName: originalName})
		renameCancel()
		if err != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("restoring original name: %w", err))
		}
	}
	if original.State.Running && (inspectErr != nil || current.State == nil || !current.State.Running) {
		startCtx, startCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		_, err := cli.ContainerStart(startCtx, original.ID, client.ContainerStartOptions{})
		startCancel()
		if err != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("restoring original running state: %w", err))
		}
	}
	if original.State.Paused && (inspectErr != nil || current.State == nil || !current.State.Paused) {
		pauseCtx, pauseCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
		_, err := cli.ContainerPause(pauseCtx, original.ID, client.ContainerPauseOptions{})
		pauseCancel()
		if err != nil {
			restoreErrs = append(restoreErrs, fmt.Errorf("restoring original paused state: %w", err))
		}
	}
	return errors.Join(restoreErrs...)
}

func (s *Service) recreateSelfContainer(ctx context.Context, envID string, cli *client.Client, info container.InspectResponse, req RecreateRequest) (container.CreateResponse, error) {
	if info.Config == nil {
		return container.CreateResponse{}, fmt.Errorf("inspected container has no config")
	}

	targetImage := info.Config.Image
	if req.Image != "" {
		targetImage = req.Image
	}

	dockerHost := ""
	if envID != "" {
		if host, err := s.pool.DockerHostContext(ctx, envID); err == nil && host != "" {
			dockerHost = host
		}
	}

	// The helper rebuilds the container from a clone of the current config, so
	// anything the editor changed has to be handed over explicitly. Env is
	// carried; every other field of req is still dropped by
	// cloneSelfContainerConfig.
	replacementEnv, removeEnv := selfRecreateEnvChanges(info, req)

	if _, err := docker.ScheduleDetachedSelfUpdateHelperForImageWithEnvChanges(ctx, cli, info, s.dataDir, dockerHost, selfRecreateOperation(req), targetImage, replacementEnv, removeEnv); err != nil {
		return container.CreateResponse{}, fmt.Errorf("scheduling self container recreate: %w", err)
	}

	return container.CreateResponse{ID: info.ID}, nil
}

// selfRecreateEnvChanges diffs the requested environment against the running
// container's. req.Env is authoritative when present — the config editor sends
// the whole list, and only when the operator changed it — so a key that is
// missing from it was deleted in the editor and has to be removed rather than
// left behind by the helper's merge.
//
// A nil req.Env means the request did not touch the environment at all, which
// is not the same as "clear it": both results are nil and the helper keeps what
// the clone already carries.
func selfRecreateEnvChanges(info container.InspectResponse, req RecreateRequest) (map[string]string, []string) {
	if req.Env == nil {
		return nil, nil
	}

	desired := make(map[string]string, len(req.Env))
	order := make([]string, 0, len(req.Env))
	for _, entry := range req.Env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if _, seen := desired[key]; !seen {
			order = append(order, key)
		}
		desired[key] = value
	}

	current := map[string]string{}
	if info.Config != nil {
		for _, entry := range info.Config.Env {
			key, value, ok := strings.Cut(entry, "=")
			if !ok || key == "" {
				continue
			}
			current[key] = value
		}
	}

	replacement := map[string]string{}
	for _, key := range order {
		value := desired[key]
		if existing, ok := current[key]; ok && existing == value {
			continue
		}
		replacement[key] = value
	}

	var removals []string
	for _, entry := range currentEnvOrder(info) {
		if _, keep := desired[entry]; !keep {
			removals = append(removals, entry)
		}
	}

	if len(replacement) == 0 {
		replacement = nil
	}
	return replacement, removals
}

// currentEnvOrder lists the keys of the running container's environment in the
// order Docker reports them, so removals stay deterministic.
func currentEnvOrder(info container.InspectResponse) []string {
	if info.Config == nil {
		return nil
	}
	keys := make([]string, 0, len(info.Config.Env))
	seen := make(map[string]struct{}, len(info.Config.Env))
	for _, entry := range info.Config.Env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func selfRecreateOperation(req RecreateRequest) string {
	if req.PullImage {
		return "update"
	}
	return "reinstall"
}

func (s *Service) recreateConnectedAgentContainer(ctx context.Context, cli *client.Client, info container.InspectResponse, originalName string, req RecreateRequest) (container.CreateResponse, error) {
	if info.Config == nil {
		return container.CreateResponse{}, fmt.Errorf("inspected container has no config")
	}

	imgName := info.Config.Image
	if req.Image != "" {
		imgName = req.Image
	}

	if req.PullImage {
		pullResp, pullErr := cli.ImagePull(ctx, imgName, client.ImagePullOptions{})
		if pullErr != nil {
			return container.CreateResponse{}, fmt.Errorf("pulling image for agent recreate: %w", pullErr)
		}
		if _, copyErr := io.Copy(io.Discard, pullResp); copyErr != nil {
			slog.Warn("containers: agent image pull stream drain failed during recreate", "error", copyErr, "image", imgName, "container", info.ID)
		}
		if closeErr := pullResp.Close(); closeErr != nil {
			slog.Warn("containers: agent image pull stream close failed during recreate", "error", closeErr, "image", imgName, "container", info.ID)
		}
	}

	oldName := originalName + "-old-" + fmt.Sprintf("%d", time.Now().Unix())
	if err := ensureAgentRecreateHelperImage(ctx, cli); err != nil {
		return container.CreateResponse{}, err
	}
	if _, err := cli.ContainerRename(ctx, info.ID, client.ContainerRenameOptions{NewName: oldName}); err != nil {
		return container.CreateResponse{}, fmt.Errorf("renaming running agent container for recreate: %w", err)
	}

	cfg, hc, netConfig, err := replacementContainerSpec(info, req, imgName, nil)
	if err != nil {
		restoreRunningAgentContainer(ctx, cli, info.ID, originalName)
		return container.CreateResponse{}, err
	}

	newResp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: cfg, HostConfig: hc, NetworkingConfig: netConfig, Name: originalName})
	if err != nil {
		restoreRunningAgentContainer(ctx, cli, info.ID, originalName)
		return container.CreateResponse{}, fmt.Errorf("creating replacement agent container: %w", err)
	}

	if err := scheduleAgentRecreateHelper(ctx, cli, info, newResp.ID, originalName); err != nil {
		removeReplacementAgentContainer(ctx, cli, newResp.ID)
		restoreRunningAgentContainer(ctx, cli, info.ID, originalName)
		return container.CreateResponse{}, err
	}

	return container.CreateResponse{ID: newResp.ID, Warnings: newResp.Warnings}, nil
}

func ensureAgentRecreateHelperImage(ctx context.Context, cli *client.Client) error {
	if _, err := cli.ImageInspect(ctx, agentRecreateHelperImage); err == nil {
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting agent recreate helper image: %w", err)
	}

	pullResp, err := cli.ImagePull(ctx, agentRecreateHelperImage, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling agent recreate helper image: %w", err)
	}
	if _, copyErr := io.Copy(io.Discard, pullResp); copyErr != nil {
		slog.Warn("containers: agent recreate helper image pull stream drain failed", "error", copyErr, "image", agentRecreateHelperImage)
	}
	if closeErr := pullResp.Close(); closeErr != nil {
		slog.Warn("containers: agent recreate helper image pull stream close failed", "error", closeErr, "image", agentRecreateHelperImage)
	}
	return nil
}

func scheduleAgentRecreateHelper(ctx context.Context, cli *client.Client, info container.InspectResponse, newID, originalName string) error {
	socketMount, err := agentDockerSocketMount(info)
	if err != nil {
		return err
	}

	helperName := "mcharbor-agent-recreate-helper-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	helperResp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: agentRecreateHelperImage,
		Cmd:   []string{"sh", "-lc", agentRecreateHelperScript()},
		Env: []string{
			"OLD_ID=" + info.ID,
			"NEW_ID=" + newID,
			"ORIGINAL_NAME=" + originalName,
		},
		Labels: map[string]string{
			"com.mcharbor.helper": "agent-recreate",
		},
	}, HostConfig: &container.HostConfig{
		AutoRemove: true,
		Mounts:     []mount.Mount{socketMount},
	}, Name: helperName})
	if err != nil {
		return fmt.Errorf("creating agent recreate helper container: %w", err)
	}

	if _, err := cli.ContainerStart(ctx, helperResp.ID, client.ContainerStartOptions{}); err != nil {
		if _, rmErr := cli.ContainerRemove(ctx, helperResp.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("containers: failed to clean up agent recreate helper after start failure", "error", rmErr, "container", helperResp.ID)
		}
		return fmt.Errorf("starting agent recreate helper container: %w", err)
	}

	return nil
}

func agentDockerSocketMount(info container.InspectResponse) (mount.Mount, error) {
	for _, mp := range info.Mounts {
		if path.Clean(mp.Destination) != "/var/run/docker.sock" {
			continue
		}

		source := mp.Source
		if mp.Type == mount.TypeVolume {
			source = mp.Name
		}
		if source == "" {
			break
		}

		return mount.Mount{
			Type:     mount.Type(mp.Type),
			Source:   source,
			Target:   mp.Destination,
			ReadOnly: !mp.RW,
		}, nil
	}

	return mount.Mount{}, fmt.Errorf("agent recreate requires a /var/run/docker.sock mount")
}

func agentRecreateHelperScript() string {
	return `set -eu
log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
log "stopping old agent container ${OLD_ID}"
docker stop -t 10 "$OLD_ID" || true
log "starting replacement agent container ${NEW_ID}"
if docker start "$NEW_ID"; then
  stable=0
  for i in 1 2 3 4 5; do
    sleep 1
    running="$(docker inspect -f '{{.State.Running}}' "$NEW_ID" 2>/dev/null || true)"
    if [ "$running" = "true" ]; then
      stable=$((stable + 1))
      continue
    fi
    stable=0
    break
  done
  if [ "$stable" -ge 3 ]; then
    log "replacement agent is running; removing old agent container"
    docker rm -f "$OLD_ID" || true
    exit 0
  fi
fi
log "replacement agent failed to stay running; rolling back old agent container"
docker rm -f "$NEW_ID" || true
docker rename "$OLD_ID" "$ORIGINAL_NAME" 2>/dev/null || true
docker start "$OLD_ID"
exit 1`
}

func removeReplacementAgentContainer(parent context.Context, cli *client.Client, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), docker.CallTimeout)
	defer cancel()
	if _, err := cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil {
		slog.Warn("containers: failed to clean up new agent container after helper scheduling failure", "error", err, "container", id)
	}
}

func restoreRunningAgentContainer(parent context.Context, cli *client.Client, id, originalName string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), docker.CallTimeout)
	defer cancel()
	if _, err := cli.ContainerRename(ctx, id, client.ContainerRenameOptions{NewName: originalName}); err != nil {
		slog.Warn("containers: failed to rename old agent container back", "error", err, "container", id)
	}
}

func replacementContainerSpec(info container.InspectResponse, req RecreateRequest, imgName string, mounts *[]MountEdit) (*container.Config, *container.HostConfig, *networkTypes.NetworkingConfig, error) {
	if info.Config == nil {
		return nil, nil, nil, fmt.Errorf("inspected container has no config")
	}

	cfg := *info.Config
	cfg.Image = imgName

	// Apply Config overrides from request. req.Env has already been resolved
	// by Recreate — this is a free function with no service to resolve with,
	// and resolution belongs next to the Docker call anyway.
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
		cfg.ExposedPorts = make(networkTypes.PortSet)
		for k, v := range req.ExposedPorts {
			port, err := networkTypes.ParsePort(k)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("exposed port %q: %w", k, err)
			}
			cfg.ExposedPorts[port] = v
		}
	}

	hc := &container.HostConfig{}
	if info.HostConfig != nil {
		hostConfig := *info.HostConfig
		hc = &hostConfig
	}

	// Apply HostConfig overrides from request
	if req.PortBindings != nil {
		portMap := make(networkTypes.PortMap)
		for port, bindings := range req.PortBindings {
			key, err := networkTypes.ParsePort(port)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("port binding %q: %w", port, err)
			}
			pb := make([]networkTypes.PortBinding, len(bindings))
			for i, b := range bindings {
				var hostIP netip.Addr
				if b.HostIP != "" {
					hostIP, err = netip.ParseAddr(b.HostIP)
					if err != nil {
						return nil, nil, nil, fmt.Errorf("port binding %q host IP %q: %w", port, b.HostIP, err)
					}
				}
				pb[i] = networkTypes.PortBinding{HostIP: hostIP, HostPort: b.HostPort}
			}
			portMap[key] = pb
		}
		hc.PortBindings = portMap
	}
	if req.NetworkMode != nil && *req.NetworkMode != "" {
		hc.NetworkMode = container.NetworkMode(*req.NetworkMode)
	}
	if req.Privileged != nil {
		hc.Privileged = *req.Privileged
	}
	if req.ReadonlyRootfs != nil {
		hc.ReadonlyRootfs = *req.ReadonlyRootfs
	}
	if req.Dns != nil {
		dns := make([]netip.Addr, 0, len(req.Dns))
		for _, server := range req.Dns {
			addr, err := netip.ParseAddr(server)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("dns server %q: %w", server, err)
			}
			dns = append(dns, addr)
		}
		hc.DNS = dns
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
	if req.Devices != nil {
		hc.Devices = *req.Devices
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
	if req.MemorySwap != nil {
		hc.MemorySwap = *req.MemorySwap
	}
	if req.MemoryReservation != nil {
		hc.MemoryReservation = *req.MemoryReservation
	}
	if req.NanoCPUs != nil {
		hc.NanoCPUs = *req.NanoCPUs
	}
	if req.CPUShares != nil {
		hc.CPUShares = *req.CPUShares
	}
	if req.CPUPeriod != nil {
		hc.CPUPeriod = *req.CPUPeriod
	}
	if req.CPUQuota != nil {
		hc.CPUQuota = *req.CPUQuota
	}
	if req.CpusetCpus != nil {
		hc.CpusetCpus = *req.CpusetCpus
	}
	if req.CpusetMems != nil {
		hc.CpusetMems = *req.CpusetMems
	}
	if req.BlkioWeight != nil {
		hc.BlkioWeight = *req.BlkioWeight
	}
	if req.RestartPolicy != nil {
		hc.RestartPolicy = *req.RestartPolicy
	}
	if mounts != nil {
		applyMountEdits(hc, *mounts)
	}

	return &cfg, hc, replacementNetworkingConfig(info), nil
}

func replacementNetworkingConfig(info container.InspectResponse) *networkTypes.NetworkingConfig {
	if info.NetworkSettings == nil || len(info.NetworkSettings.Networks) == 0 {
		return nil
	}

	endpointsConfig := make(map[string]*networkTypes.EndpointSettings)
	for name, ep := range info.NetworkSettings.Networks {
		if ep == nil {
			continue
		}
		endpointsConfig[name] = &networkTypes.EndpointSettings{
			IPAMConfig: ep.IPAMConfig,
			Links:      ep.Links,
			Aliases:    ep.Aliases,
			MacAddress: ep.MacAddress,
			DriverOpts: ep.DriverOpts,
			GwPriority: ep.GwPriority,
		}
	}
	if len(endpointsConfig) == 0 {
		return nil
	}
	return &networkTypes.NetworkingConfig{
		EndpointsConfig: endpointsConfig,
	}
}

// NetworkConnect connects or reconnects a container to a Docker network.
func (s *Service) NetworkConnect(ctx context.Context, envID, id string, req NetworkConnectRequest) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	settings, err := endpointSettingsFromConnectRequest(req)
	if err != nil {
		return err
	}
	if req.Reconnect {
		previous, hasPrevious, err := currentNetworkEndpointSettings(ctx, cli, id, req.Network)
		if err != nil {
			return err
		}
		if _, err := cli.NetworkDisconnect(ctx, req.Network, client.NetworkDisconnectOptions{Container: id, Force: true}); err != nil {
			return fmt.Errorf("disconnecting container %s from network %s: %w", id, req.Network, err)
		}
		if _, err := cli.NetworkConnect(ctx, req.Network, client.NetworkConnectOptions{Container: id, EndpointConfig: settings}); err != nil {
			if hasPrevious {
				if _, restoreErr := cli.NetworkConnect(ctx, req.Network, client.NetworkConnectOptions{Container: id, EndpointConfig: previous}); restoreErr != nil {
					return fmt.Errorf("connecting container %s to network %s: %w; restoring previous endpoint settings: %w", id, req.Network, err, restoreErr)
				}
			}
			return fmt.Errorf("connecting container %s to network %s: %w", id, req.Network, err)
		}
		return nil
	}

	if _, err := cli.NetworkConnect(ctx, req.Network, client.NetworkConnectOptions{Container: id, EndpointConfig: settings}); err != nil {
		return fmt.Errorf("connecting container %s to network %s: %w", id, req.Network, err)
	}
	return nil
}

func endpointSettingsFromConnectRequest(req NetworkConnectRequest) (*networkTypes.EndpointSettings, error) {
	settings := &networkTypes.EndpointSettings{
		Aliases: cleanStringSlice(req.Aliases),
	}
	if mac := strings.TrimSpace(req.MacAddress); mac != "" {
		hw, err := net.ParseMAC(mac)
		if err != nil {
			return nil, fmt.Errorf("mac address %q: %w", mac, err)
		}
		settings.MacAddress = networkTypes.HardwareAddr(hw)
	}

	ipv4, err := optionalAddr(req.IPv4Address)
	if err != nil {
		return nil, fmt.Errorf("ipv4 address: %w", err)
	}
	ipv6, err := optionalAddr(req.IPv6Address)
	if err != nil {
		return nil, fmt.Errorf("ipv6 address: %w", err)
	}
	if ipv4.IsValid() || ipv6.IsValid() {
		settings.IPAMConfig = &networkTypes.EndpointIPAMConfig{
			IPv4Address: ipv4,
			IPv6Address: ipv6,
		}
	}

	return settings, nil
}

// optionalAddr parses an operator-entered IP, where blank means "let the daemon pick".
func optionalAddr(raw string) (netip.Addr, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%q: %w", raw, err)
	}
	return addr, nil
}

func currentNetworkEndpointSettings(ctx context.Context, cli *client.Client, id, networkName string) (*networkTypes.EndpointSettings, bool, error) {
	infoRes, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("inspecting container network settings: %w", err)
	}
	info := infoRes.Container
	if info.NetworkSettings == nil {
		return nil, false, nil
	}
	endpoint, ok := info.NetworkSettings.Networks[networkName]
	if !ok || endpoint == nil {
		for _, candidate := range info.NetworkSettings.Networks {
			if candidate != nil && candidate.NetworkID == networkName {
				endpoint = candidate
				ok = true
				break
			}
		}
	}
	if !ok || endpoint == nil {
		return nil, false, nil
	}
	return &networkTypes.EndpointSettings{
		IPAMConfig: endpoint.IPAMConfig,
		Links:      endpoint.Links,
		Aliases:    endpoint.Aliases,
		MacAddress: endpoint.MacAddress,
		DriverOpts: endpoint.DriverOpts,
		GwPriority: endpoint.GwPriority,
	}, true, nil
}

func cleanStringSlice(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// NetworkDisconnect disconnects a container from a Docker network.
func (s *Service) NetworkDisconnect(ctx context.Context, envID, id, network string, force bool) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}
	_, err = cli.NetworkDisconnect(ctx, network, client.NetworkDisconnectOptions{Container: id, Force: force})
	return err
}

// Logs returns container logs.
func (s *Service) Logs(ctx context.Context, envID, id string, query LogsQuery) (string, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
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

	opts := client.ContainerLogsOptions{
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
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("closing reader failed", "error", err)
		}
	}()

	const maxContainerLogsBytes = 16 * 1024 * 1024
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(reader, maxContainerLogsBytes+1)); err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	if buf.Len() > maxContainerLogsBytes {
		buf.Truncate(maxContainerLogsBytes)
	}

	// Demux if container is not TTY
	infoRes, inspectErr := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	info := infoRes.Container
	if inspectErr == nil && !info.Config.Tty {
		demuxed := docker.DemuxDockerStream(buf.Bytes())
		return demuxed.Stdout + demuxed.Stderr, nil
	}

	return buf.String(), nil
}

// Stats returns container resource usage statistics.
func (s *Service) Stats(ctx context.Context, envID, id string, stream bool) (io.ReadCloser, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	// For non-streaming (one-shot) stats, apply a deadline so the call
	// doesn't hang indefinitely. Streaming callers manage their own
	// context lifecycle.
	var cancel context.CancelFunc
	if !stream {
		ctx, cancel = context.WithTimeout(ctx, docker.CallTimeout)
	}

	resp, err := cli.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: stream, IncludePreviousSample: true})
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("fetching stats for %s: %w", id, err)
	}
	if cancel != nil {
		return &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
	}

	return resp.Body, nil
}

// Top returns running processes in a container.
func (s *Service) Top(ctx context.Context, envID, id string) (container.TopResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.TopResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	top, err := cli.ContainerTop(ctx, id, client.ContainerTopOptions{})
	if err != nil {
		return container.TopResponse{}, fmt.Errorf("getting top for %s: %w", id, err)
	}

	return container.TopResponse{Processes: top.Processes, Titles: top.Titles}, nil
}

// DetectShells probes a container for available shells.
func (s *Service) DetectShells(ctx context.Context, envID, id string) ([]ShellResult, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
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
	execConfig := client.ExecCreateOptions{
		Cmd:          []string{shell, "-c", "echo ok"},
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := cli.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return false
	}

	hijack, err := cli.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
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
		if _, err := io.Copy(&output, hijack.Reader); err != nil {
			slog.Debug("containers: exec output stream ended with error", "error", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-execCtx.Done():
		return false
	}

	// Check exec exit code
	inspect, err := cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	if s.pool.IsAgentEnvContext(ctx, envID) {
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

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	tmpFile := fmt.Sprintf("/tmp/.mcharbor-ls-%d", time.Now().UnixNano())

	// Create exec that writes ls output to a temp file
	execResp, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		Cmd: []string{"/bin/sh", "-c", fmt.Sprintf("ls -la %s > %s 2>&1", shellQuote(dirPath), tmpFile)},
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	// Start exec detached — standard POST through RoundTrip
	if _, err := cli.ExecStart(ctx, execResp.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return nil, fmt.Errorf("exec start: %w", err)
	}

	// Poll until exec finishes
	for {
		inspect, err := cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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
	readerRes, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: tmpFile})
	if err != nil {
		return nil, fmt.Errorf("copy output file: %w", err)
	}
	reader := readerRes.Content
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("closing reader failed", "error", err)
		}
	}()

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
		cleanExec, cerr := cli.ExecCreate(cleanCtx, id, client.ExecCreateOptions{
			Cmd: []string{"/bin/sh", "-c", "rm -f " + tmpFile},
		})
		if cerr == nil {
			if _, err := cli.ExecStart(cleanCtx, cleanExec.ID, client.ExecStartOptions{Detach: true}); err != nil {
				slog.Warn("containers: temp file cleanup exec failed", "error", err, "container", id)
			}
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

	execResp, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		Cmd:          []string{"/bin/sh", "-c", "ls -la " + shellQuote(dirPath)},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, err
	}

	hijack, err := cli.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, err
	}
	defer hijack.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan struct{})
	go func() {
		if _, err := stdcopy.StdCopy(&stdout, &stderr, hijack.Reader); err != nil {
			slog.Debug("containers: exec output stream ended with error", "error", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Check exit code
	inspect, err := cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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
		size, parseErr := strconv.ParseInt(fields[4], 10, 64)
		if parseErr != nil {
			size = 0
		}

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
	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	readerRes, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: dirPath})
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such file or directory") ||
			strings.Contains(errMsg, "not a directory") ||
			strings.Contains(errMsg, "Could not find the file") {
			return []FileEntry{}, nil
		}
		return nil, fmt.Errorf("listing files in %s for %s: %w", dirPath, id, err)
	}
	reader := readerRes.Content
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("closing reader failed", "error", err)
		}
	}()

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

	if s.pool.IsAgentEnvContext(ctx, envID) {
		output, err = s.execViaDetached(ctx, envID, id, cmd)
	} else {
		cli, cErr := s.getClient(ctx, envID)
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

	execResp, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		Cmd:          []string{"/bin/sh", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	hijack, err := cli.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer hijack.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan struct{})
	go func() {
		if _, err := stdcopy.StdCopy(&stdout, &stderr, hijack.Reader); err != nil {
			slog.Debug("containers: exec output stream ended with error", "error", err)
		}
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

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return "", err
	}

	tmpFile := fmt.Sprintf("/tmp/.mcharbor-svc-%d", time.Now().UnixNano())

	execResp, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		Cmd: []string{"/bin/sh", "-c", fmt.Sprintf("%s > %s 2>&1", cmd, tmpFile)},
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}

	if _, err := cli.ExecStart(ctx, execResp.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return "", fmt.Errorf("exec start: %w", err)
	}

	for {
		inspect, err := cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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

	readerRes, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: tmpFile})
	if err != nil {
		return "", fmt.Errorf("copy output file: %w", err)
	}
	reader := readerRes.Content
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("closing reader failed", "error", err)
		}
	}()

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
		cleanExec, cerr := cli.ExecCreate(cleanCtx, id, client.ExecCreateOptions{
			Cmd: []string{"/bin/sh", "-c", "rm -f " + tmpFile},
		})
		if cerr == nil {
			if _, err := cli.ExecStart(cleanCtx, cleanExec.ID, client.ExecStartOptions{Detach: true}); err != nil {
				slog.Warn("containers: temp file cleanup exec failed", "error", err, "container", id)
			}
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
		switch state {
		case "running":
			status = "running"
		case "stopped", "exited":
			status = "stopped"
		case "fatal", "backoff":
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
	if s.stats != nil {
		snap, err := s.stats.Snapshot(ctx, envID)
		if err != nil {
			return nil, fmt.Errorf("reading live stats: %w", err)
		}
		out := make([]BulkContainerMetric, len(snap.Samples))
		for i, m := range snap.Samples {
			out[i] = BulkContainerMetric{
				ID: m.ID, Name: m.Name, CPUPercent: m.CPUPercent,
				MemUsage: m.MemUsage, MemLimit: m.MemLimit, MemPercent: m.MemPercent,
				NetRx: m.NetRx, NetTx: m.NetTx, BlockRead: m.BlockRead, BlockWrite: m.BlockWrite, PIDs: m.PIDs,
			}
		}
		return out, nil
	}
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	// List running containers
	runningRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: false})
	running := runningRes.Items
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
			resp, sErr := cli.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
			if sErr != nil {
				ch <- statsResult{err: sErr}
				return
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					slog.Warn("closing resp failed", "error", err)
				}
			}()

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

// Prune removes every stopped container in envID and returns the daemon's report.
func (s *Service) Prune(ctx context.Context, envID string) (container.PruneReport, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return container.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	containersRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Size: true})
	containers := containersRes.Items
	if err != nil {
		return container.PruneReport{}, fmt.Errorf("listing containers for prune: %w", err)
	}

	report := container.PruneReport{}
	for _, c := range containers {
		if c.State == "running" || docker.ClassifySummary(c.ID, c.Names, c.Image, c.Labels) != docker.SelfTargetNone {
			continue
		}
		if _, err := cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{}); err != nil {
			return container.PruneReport{}, fmt.Errorf("pruning container %s: %w", c.ID, err)
		}
		report.ContainersDeleted = append(report.ContainersDeleted, c.ID)
		if c.SizeRw > 0 {
			report.SpaceReclaimed += uint64(c.SizeRw)
		}
	}
	return report, nil
}

// execCommand runs a command inside a container, handling both agent and local environments.
// Returns stdout output and any error.
func (s *Service) execCommand(ctx context.Context, envID, id string, cmd []string) (string, error) {
	shellCmd := strings.Join(cmd, " ")
	if s.pool.IsAgentEnvContext(ctx, envID) {
		return s.execViaDetached(ctx, envID, id, shellCmd)
	}
	cli, err := s.getClient(ctx, envID)
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

	_, err := cli.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: destDir, Content: &buf})
	return err
}

// FileContent reads a file from a container and returns its content.
func (s *Service) FileContent(ctx context.Context, envID, id, filePath string) ([]byte, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	readerRes, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: filePath})
	if err != nil {
		return nil, fmt.Errorf("copying file from container: %w", err)
	}
	reader := readerRes.Content
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("closing reader failed", "error", err)
		}
	}()

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
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	dir := path.Dir(filePath)
	fileName := path.Base(filePath)

	return s.copyContentToContainer(ctx, cli, id, dir, fileName, content, 0644)
}

// UploadFile uploads a file from a reader into a container directory.
func (s *Service) UploadFile(ctx context.Context, envID, id, destDir, fileName string, reader io.Reader, size int64) error {
	return s.UploadFiles(ctx, envID, id, destDir, []UploadFileItem{{
		RelativePath: fileName,
		Reader:       reader,
		Size:         size,
	}}, nil)
}

// UploadFileItem describes one file to copy into a container directory.
type UploadFileItem struct {
	RelativePath string
	Reader       io.Reader
	Size         int64
}

// UploadFiles uploads a set of files and directories into a container directory.
func (s *Service) UploadFiles(ctx context.Context, envID, id, destDir string, files []UploadFileItem, directories []string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	// Large directory uploads can spend most of their time streaming the tar into Docker.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}

	reader, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- writeUploadTar(writer, files, directories)
	}()

	_, copyErr := cli.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: destDir, Content: reader})
	if copyErr != nil {
		if err := reader.CloseWithError(copyErr); err != nil {
			slog.Warn("containers: close upload pipe reader failed", "error", err)
		}
	} else if err := reader.Close(); err != nil {
		slog.Warn("containers: close upload pipe reader failed", "error", err)
	}

	writeErr := <-writeDone
	if copyErr != nil {
		return fmt.Errorf("copying upload to container: %w", copyErr)
	}
	return writeErr
}

func writeUploadTar(pipeWriter *io.PipeWriter, files []UploadFileItem, directories []string) error {
	tw := tar.NewWriter(pipeWriter)
	err := writeUploadTarEntries(tw, files, directories)
	if closeErr := tw.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("closing tar: %w", closeErr)
	}
	if err != nil {
		if closeErr := pipeWriter.CloseWithError(err); closeErr != nil {
			slog.Warn("containers: close upload pipe writer with error failed", "error", closeErr)
		}
		return err
	}
	return pipeWriter.Close()
}

func writeUploadTarEntries(tw *tar.Writer, files []UploadFileItem, directories []string) error {
	writtenDirs := make(map[string]bool)
	for _, dir := range directories {
		cleanDir, err := cleanUploadRelativePath(dir)
		if err != nil {
			return err
		}
		if err := writeUploadDirectory(tw, cleanDir, writtenDirs); err != nil {
			return err
		}
	}

	for _, file := range files {
		cleanPath, err := cleanUploadRelativePath(file.RelativePath)
		if err != nil {
			return err
		}
		if dir := path.Dir(cleanPath); dir != "." {
			if err := writeUploadDirectory(tw, dir, writtenDirs); err != nil {
				return err
			}
		}
		if err := writeUploadFile(tw, cleanPath, file); err != nil {
			return err
		}
	}
	return nil
}

func writeUploadFile(tw *tar.Writer, cleanPath string, file UploadFileItem) error {
	hdr := &tar.Header{
		Name: cleanPath,
		Mode: 0644,
		Size: file.Size,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := io.Copy(tw, file.Reader); err != nil {
		return fmt.Errorf("writing tar body: %w", err)
	}
	return nil
}

func cleanUploadRelativePath(rawPath string) (string, error) {
	cleanPath := path.Clean(strings.ReplaceAll(rawPath, "\\", "/"))
	cleanPath = strings.TrimPrefix(cleanPath, "./")
	if cleanPath == "." || cleanPath == "" {
		return "", fmt.Errorf("upload path is empty")
	}
	if strings.HasPrefix(cleanPath, "/") || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", fmt.Errorf("invalid upload path %q", rawPath)
	}
	return cleanPath, nil
}

func writeUploadDirectory(tw *tar.Writer, dir string, written map[string]bool) error {
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	for i := range parts {
		current := strings.Join(parts[:i+1], "/")
		if written[current] {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     current + "/",
			Typeflag: tar.TypeDir,
			Mode:     0755,
		}); err != nil {
			return fmt.Errorf("writing tar directory header: %w", err)
		}
		written[current] = true
	}
	return nil
}

// CreateDirectory creates a directory (including parents) inside a container.
func (s *Service) CreateDirectory(ctx context.Context, envID, id, dirPath string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}
	_, err = s.execCommand(ctx, envID, id, []string{"mkdir", "-p", shellQuote(dirPath)})
	return err
}

// RenameFile renames or moves a file/directory inside a container.
func (s *Service) RenameFile(ctx context.Context, envID, id, oldPath, newPath string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}
	_, err = s.execCommand(ctx, envID, id, []string{"mv", shellQuote(oldPath), shellQuote(newPath)})
	return err
}

// ChangePermissions changes the file mode of a path inside a container.
func (s *Service) ChangePermissions(ctx context.Context, envID, id, filePath, mode string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}
	_, err = s.execCommand(ctx, envID, id, []string{"chmod", mode, shellQuote(filePath)})
	return err
}

// DeleteFile removes a file or directory inside a container.
func (s *Service) DeleteFile(ctx context.Context, envID, id, filePath string, recursive bool) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if err := docker.EnsureContainerMutable(ctx, cli, id); err != nil {
		return err
	}
	cmd := []string{"rm", "-f", shellQuote(filePath)}
	if recursive {
		cmd = []string{"rm", "-rf", shellQuote(filePath)}
	}
	_, err = s.execCommand(ctx, envID, id, cmd)
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
	cli, err := s.getClient(ctx, envID)
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
			inspCtx, inspCancel := context.WithTimeout(ctx, docker.CallTimeout)
			defer inspCancel()

			imgInspect, err := cli.ImageInspect(inspCtx, ctr.ImageID)
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
			distCtx, distCancel := context.WithTimeout(ctx, docker.CallTimeout)
			defer distCancel()

			// Use the image reference (e.g., "nginx:latest") for the registry query
			ref := ctr.Image
			distInspect, err := cli.DistributionInspect(distCtx, ref, client.DistributionInspectOptions{})
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

// builtInDockerNetworks lists Docker's automatic networks that should never be auto-removed.
var builtInDockerNetworks = map[string]struct{}{
	"bridge": {},
	"host":   {},
	"none":   {},
}

// OrphanedNetwork reports a user-defined network that will have no containers
// attached once the given container is removed.
type OrphanedNetwork struct {
	ID   string
	Name string
}

// FindOrphanedNetworks inspects the container's network attachments and returns
// the user-defined networks that would be empty after this container is removed.
// A network is considered orphaned when it is user-defined (not in
// builtInDockerNetworks) and currently has exactly one attached container,
// and that container is the one being inspected.
func (s *Service) FindOrphanedNetworks(ctx context.Context, envID, containerID string, info container.InspectResponse) ([]OrphanedNetwork, error) {
	if info.NetworkSettings == nil || len(info.NetworkSettings.Networks) == 0 {
		return nil, nil
	}

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	orphans := make([]OrphanedNetwork, 0)
	seen := make(map[string]struct{})

	for netName := range info.NetworkSettings.Networks {
		if _, isBuiltIn := builtInDockerNetworks[netName]; isBuiltIn {
			continue
		}
		if _, dup := seen[netName]; dup {
			continue
		}
		seen[netName] = struct{}{}

		inspCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		netRes, err := cli.NetworkInspect(inspCtx, netName, client.NetworkInspectOptions{})
		net := netRes.Network
		cancel()
		if err != nil {
			// Skip networks we cannot inspect — best effort.
			continue
		}
		if len(net.Containers) != 1 {
			continue
		}
		onlyID, ok := soleKey(net.Containers)
		if !ok || onlyID != containerID {
			continue
		}
		orphans = append(orphans, OrphanedNetwork{ID: net.ID, Name: net.Name})
	}

	return orphans, nil
}

// soleKey returns the single key of m when m has exactly one entry.
func soleKey(m map[string]networkTypes.EndpointResource) (string, bool) {
	if len(m) != 1 {
		return "", false
	}
	for k := range m {
		return k, true
	}
	return "", false
}

// RemoveNetwork removes a Docker network by ID or name. It returns an error
// when the network is still in use or the daemon refuses removal.
func (s *Service) RemoveNetwork(ctx context.Context, envID, networkID string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	rmCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, err := cli.NetworkRemove(rmCtx, networkID, client.NetworkRemoveOptions{}); err != nil {
		return fmt.Errorf("removing network %s: %w", networkID, err)
	}
	return nil
}

// AuditName resolves the human-readable name used in audit entries for a
// container. It falls back to the short ID when the container cannot be
// inspected (removed, daemon offline, agent unreachable), so callers never
// have to handle an error just to label a log row.
func (s *Service) AuditName(ctx context.Context, envID, id string) string {
	info, err := s.Inspect(ctx, envID, id)
	if err != nil {
		return shortContainerID(id)
	}
	return normalizeContainerName(info.Name, id)
}

// secretFileModeLabel records on the container itself that it was built with
// file-based secret injection, so a later Recreate rebuilds it the same way.
// The request that asked for it does not survive; the container's own config
// does.
const secretFileModeLabel = "com.mcharbor.secret-files" //nolint:gosec // G101 false positive: a Docker label key, not a credential

// prepareSecrets turns a container's environment into what Docker should
// actually receive.
//
// Two modes. Ordinarily every `${secret:NAME}` is resolved inline and the
// plaintext ends up in the container's environment — where `docker inspect`,
// and therefore any other Docker manager on the host, reads it. In file mode
// the value goes into a read-only volume at /run/secrets instead, and the
// environment carries `<VAR>_FILE` pointing at it.
//
// Returns the environment to set and, in file mode, the mount to add.
func (s *Service) prepareSecrets(
	ctx context.Context,
	cli *client.Client,
	envID, owner, image string,
	env []string,
	fileMode bool,
) ([]string, *mount.Mount, error) {
	if s.resolver == nil || len(env) == 0 {
		return env, nil, nil
	}

	scope := secrets.ResolveScope{
		EnvironmentID: envID,
		ResourceType:  "container",
		ResourceID:    owner,
		ResourceName:  owner,
	}

	if !fileMode {
		resolved, err := s.resolver.ResolveSlice(ctx, env, scope)
		if err != nil {
			slog.Error("resolving container secrets failed", "env", envID, "container", owner, "error", err)
			return nil, nil, err
		}
		return resolved, nil, nil
	}

	planner, ok := s.resolver.(secretFilePlanner)
	if !ok {
		// A resolver that cannot plan file mode is a build without the vault.
		// Falling back to inline resolution would put the values exactly where
		// the operator asked for them not to be.
		return nil, nil, fmt.Errorf("file-based secret injection is unavailable")
	}

	plan, resolvedEnv, err := planner.PlanFileModeSlice(ctx, env, scope)
	if err != nil {
		slog.Error("planning container secret files failed", "env", envID, "container", owner, "error", err)
		return nil, nil, err
	}
	if len(plan.Files) == 0 {
		// Nothing referenced a secret, so there is no volume to create and
		// nothing the mode would change.
		return resolvedEnv, nil, nil
	}

	volumeName, err := docker.MaterialiseSecretVolume(ctx, cli, owner, image, plan.Files)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("container secrets delivered as files",
		"container", owner, "volume", volumeName, "variables", plan.Replaced)

	m := docker.SecretVolumeMount(volumeName, secrets.SecretMountPath)
	return resolvedEnv, &m, nil
}

// secretFilePlanner is the half of the vault that file mode needs, declared
// separately from SecretResolver so the resolver interface stays the narrow
// thing every other call site uses.
type secretFilePlanner interface {
	PlanFileModeSlice(ctx context.Context, env []string, scope secrets.ResolveScope) (*secrets.FileModePlan, []string, error)
}
