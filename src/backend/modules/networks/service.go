// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// ErrBuiltIn is returned when an operation targets a Docker built-in network
// (bridge, host, none) which McHarbor treats as non-deletable to protect the
// host's networking stack.
var ErrBuiltIn = errors.New("built-in network")

// builtInNetworkNames is the set of Docker built-in network names that are
// always present on every Docker host and must never be removed.
var builtInNetworkNames = map[string]struct{}{
	"bridge": {},
	"host":   {},
	"none":   {},
}

// IsBuiltInName reports whether the given network name is a Docker built-in.
func IsBuiltInName(name string) bool {
	_, ok := builtInNetworkNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Service wraps Docker SDK network operations.
type Service struct {
	pool *docker.ClientPool
	// agents is how the host's own link inventory is reached: /sys/class/net is
	// network-namespaced, so McHarbor's container cannot read it and only an
	// agent sharing the host netns can answer. Nil-safe — a deployment without
	// agents simply gets no agent-sourced interfaces.
	agents *coreagent.AgentPool
	// linkerFor replaces the executor choice in tests; nil in production.
	linkerFor func(ctx context.Context, envID string) (hostLinker, string)
}

// NewService creates a new network service.
func NewService(pool *docker.ClientPool, agents *coreagent.AgentPool) *Service {
	return &Service{pool: pool, agents: agents}
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(ctx context.Context, envID string) (*client.Client, error) {
	c, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

// List returns all networks. The Docker REST API does not populate the
// `Containers` field in the list response, so we cross-reference a single
// container list to count how many containers are attached to each network.
func (s *Service) List(ctx context.Context, envID string) ([]NetworkSummary, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	networksRes, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	networks := networksRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}

	usage, names, err := s.containerUsageByNetworkName(ctx, cli)
	if err != nil {
		// Best-effort: a failure here should not prevent the network list
		// from rendering. Counts will simply be 0.
		usage = map[string]int{}
		names = map[string][]string{}
	}

	result := make([]NetworkSummary, 0, len(networks))
	for _, n := range networks {
		result = append(result, NetworkSummary{
			ID:             n.ID,
			Name:           n.Name,
			Driver:         n.Driver,
			Scope:          n.Scope,
			Internal:       n.Internal,
			Attachable:     n.Attachable,
			IPAM:           n.IPAM,
			Containers:     usage[n.Name],
			ContainerNames: names[n.Name],
			EnableIPv6:     n.EnableIPv6,
			HostLink:       hostLinkFromLabels(n.Labels),
			Options:        n.Options,
			Labels:         n.Labels,
			Created:        n.Created.Format("2006-01-02T15:04:05Z"),
			BuiltIn:        IsBuiltInName(n.Name),
		})
	}

	return result, nil
}

// containerUsageByNetworkName counts how many containers are attached to each
// network, and names them, both keyed by network name. Containers with
// multiple network attachments contribute once per network.
func (s *Service) containerUsageByNetworkName(ctx context.Context, cli *client.Client) (map[string]int, map[string][]string, error) {
	containersRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if err != nil {
		return nil, nil, fmt.Errorf("listing containers for network usage: %w", err)
	}

	usage := make(map[string]int)
	names := make(map[string][]string)
	for _, c := range containers {
		if c.NetworkSettings == nil {
			continue
		}
		containerName := ""
		if len(c.Names) > 0 {
			containerName = strings.TrimPrefix(c.Names[0], "/")
		}
		for netName := range c.NetworkSettings.Networks {
			usage[netName]++
			if containerName != "" {
				names[netName] = append(names[netName], containerName)
			}
		}
	}
	return usage, names, nil
}

// Create creates a new network. With a HostLink it also creates the host interface, and removes
// the network again if that fails, so a half-made pair is never left behind.
func (s *Service) Create(ctx context.Context, envID string, req CreateRequest) (network.CreateResponse, error) {
	plan, err := s.prepareCreate(ctx, envID, req)
	if err != nil {
		return network.CreateResponse{}, err
	}
	return s.execCreate(ctx, envID, plan)
}

// createPlan is a validated create, split from its execution so Recreate can refuse a bad
// definition before it has disconnected or removed anything.
type createPlan struct {
	name   string
	opts   client.NetworkCreateOptions
	parent string
	link   *hostLinkSpec
	linker hostLinker
}

func (s *Service) prepareCreate(ctx context.Context, envID string, req CreateRequest) (createPlan, error) {
	ipam, err := req.IPAM.toSDK(req.EnableIPv6)
	if err != nil {
		return createPlan{}, err
	}
	labels := stripHostLinkLabels(req.Labels)
	plan := createPlan{name: req.Name, parent: strings.TrimSpace(req.Options["parent"])}
	if req.HostLink != nil {
		spec, err := validateHostLink(req, ipam)
		if err != nil {
			return createPlan{}, err
		}
		reserveHostAddresses(ipam, spec)
		labels = hostLinkLabels(labels, spec)
		plan.link = &spec
		var reason string
		if plan.linker, reason = s.hostLinkerFor(ctx, envID); plan.linker == nil {
			return createPlan{}, &HostLinkUnavailableError{Reason: reason}
		}
	}
	if len(labels) == 0 {
		labels = nil
	}
	enableIPv6 := req.EnableIPv6
	plan.opts = client.NetworkCreateOptions{
		Driver:     req.Driver,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		EnableIPv6: &enableIPv6,
		Options:    req.Options,
		Labels:     labels,
		IPAM:       ipam,
	}
	return plan, nil
}

func (s *Service) execCreate(ctx context.Context, envID string, plan createPlan) (network.CreateResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return network.CreateResponse{}, err
	}

	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	resp, err := cli.NetworkCreate(callCtx, plan.name, plan.opts)
	if err != nil {
		return network.CreateResponse{}, fmt.Errorf("creating network: %w", err)
	}

	if plan.link != nil {
		if err := ensureHostLink(ctx, plan.linker, resp.ID, plan.parent, plan.link.info()); err != nil {
			rbCtx, rbCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
			defer rbCancel()
			_, rmErr := cli.NetworkRemove(rbCtx, resp.ID, client.NetworkRemoveOptions{})
			logRollback("network remove", resp.ID, rmErr)
			return network.CreateResponse{}, err
		}
	}

	return network.CreateResponse{ID: resp.ID, Warning: strings.Join(resp.Warning, "\n")}, nil
}

// Inspect returns detailed network information.
func (s *Service) Inspect(ctx context.Context, envID, id string) (network.Inspect, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return network.Inspect{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	netRes, err := cli.NetworkInspect(ctx, id, client.NetworkInspectOptions{Verbose: true})
	if err != nil {
		return network.Inspect{}, fmt.Errorf("inspecting network %s: %w", id, err)
	}
	net := netRes.Network

	return net, nil
}

// Remove removes a network. Built-in networks (bridge, host, none) are
// rejected with ErrBuiltIn to protect the host's networking stack. A network that carries a host
// link has the link removed first, and the removal stops if that fails, so the interface is never
// orphaned from the record that names it.
func (s *Service) Remove(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	// Inspect the network first so we can reject built-ins by name before
	// asking the Docker daemon to remove them. NetworkInspect works for
	// bridge and host, and the lookup-by-id catches calls that pass a
	// built-in ID without a name.
	netRes, err := cli.NetworkInspect(callCtx, id, client.NetworkInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting network %s before remove: %w", id, err)
	}
	net := netRes.Network
	if IsBuiltInName(net.Name) {
		return fmt.Errorf("%w: %s", ErrBuiltIn, net.Name)
	}

	info := hostLinkFromLabels(net.Labels)
	var linker hostLinker
	if info != nil {
		var reason string
		linker, reason = s.hostLinkerFor(ctx, envID)
		switch {
		case linker != nil:
			if err := removeHostLink(ctx, linker, net.ID, info.Name); err != nil {
				return err
			}
		case reason == UnavailableContainerized:
			slog.Warn("network removal: no executor can reach the host link, leaving it", "env", envID, "network", net.ID, "link", info.Name)
		default:
			return &HostLinkUnavailableError{Reason: reason}
		}
	}

	if _, err := cli.NetworkRemove(callCtx, id, client.NetworkRemoveOptions{}); err != nil {
		if linker != nil {
			rbCtx, rbCancel := context.WithTimeout(context.WithoutCancel(ctx), docker.CallTimeout)
			defer rbCancel()
			logRollback("host link restore", net.ID, ensureHostLink(rbCtx, linker, net.ID, strings.TrimSpace(net.Options["parent"]), *info))
		}
		return fmt.Errorf("removing network %s: %w", id, err)
	}

	return nil
}

// Connect connects a container to a network.
func (s *Service) Connect(ctx context.Context, envID, networkID string, req ConnectRequest) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if req.Container, err = s.guardContainer(ctx, envID, req.Container); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, err := cli.NetworkConnect(ctx, networkID, client.NetworkConnectOptions{Container: req.Container, EndpointConfig: &network.EndpointSettings{}}); err != nil {
		return fmt.Errorf("connecting container %s to network %s: %w", req.Container, networkID, err)
	}

	return nil
}

// Disconnect disconnects a container from a network.
func (s *Service) Disconnect(ctx context.Context, envID, networkID string, req DisconnectRequest) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}
	if req.Container, err = s.guardContainer(ctx, envID, req.Container); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, err := cli.NetworkDisconnect(ctx, networkID, client.NetworkDisconnectOptions{Container: req.Container, Force: req.Force}); err != nil {
		return fmt.Errorf("disconnecting container %s from network %s: %w", req.Container, networkID, err)
	}

	return nil
}

// Prune removes unused networks, and the host link of each labelled network it removed.
func (s *Service) Prune(ctx context.Context, envID string) (network.PruneReport, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return network.PruneReport{}, err
	}

	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	linked := map[string]hostLinkOwner{}
	if listRes, err := cli.NetworkList(callCtx, client.NetworkListOptions{}); err == nil {
		for _, n := range listRes.Items {
			if info := hostLinkFromLabels(n.Labels); info != nil {
				linked[n.Name] = hostLinkOwner{id: n.ID, name: info.Name}
			}
		}
	}

	reportRes, err := cli.NetworkPrune(callCtx, client.NetworkPruneOptions{})
	if err != nil {
		return network.PruneReport{}, fmt.Errorf("pruning networks: %w", err)
	}
	report := reportRes.Report

	for _, deleted := range report.NetworksDeleted {
		owner, ok := linked[deleted]
		if !ok {
			continue
		}
		linker, reason := s.hostLinkerFor(ctx, envID)
		if linker == nil {
			slog.Warn("prune: host link left behind, no executor", "env", envID, "network", owner.id, "link", owner.name, "reason", reason)
			continue
		}
		if err := removeHostLink(ctx, linker, owner.id, owner.name); err != nil {
			slog.Warn("prune: removing host link failed", "env", envID, "network", owner.id, "link", owner.name, "error", err)
		}
	}

	return report, nil
}

// hostLinkOwner is a pruned network's id and the name of the interface it owned.
type hostLinkOwner struct{ id, name string }
