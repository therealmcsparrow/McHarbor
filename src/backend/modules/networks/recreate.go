// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/redact"
)

// ErrRecreateRestored means the new definition could not be created after the old network was
// removed, and the original network was put back with its containers; nothing changed.
var ErrRecreateRestored = errors.New("the new network definition was refused; the original network was restored")

// ErrRecreateLost means the new definition failed and restoring the original failed too, so the
// network no longer exists. The containers are left disconnected.
var ErrRecreateLost = errors.New("the network could not be recreated or restored")

// RecreateResult is the body of POST /networks/{id}/recreate.
type RecreateResult struct {
	ID      string             `json:"Id"`
	Warning string             `json:"Warning,omitempty"`
	Results []ReconnectOutcome `json:"reconnected"`
}

// ReconnectOutcome reports one container's way back onto the recreated network. AddressKept is
// false when the container came back on a new address — the case the operator must hear about.
type ReconnectOutcome struct {
	Container    string `json:"container"`
	Name         string `json:"name"`
	PreviousIPv4 string `json:"previousIpv4,omitempty"`
	PreviousIPv6 string `json:"previousIpv6,omitempty"`
	AddressKept  bool   `json:"addressKept"`
	Error        string `json:"error,omitempty"`
}

// endpointSnapshot is everything a container's attachment carried, so reconnecting gives it back
// the same IP, MAC and DNS aliases instead of whatever the daemon would hand out next.
type endpointSnapshot struct {
	id, name   string
	ipv4, ipv6 netip.Addr
	settings   network.EndpointSettings
}

// Recreate replaces network id with the definition in req and puts every attached container back
// on it with its previous addresses, MAC and aliases. The new definition is validated before
// anything is touched; if Docker still refuses it, the original network is restored.
func (s *Service) Recreate(ctx context.Context, envID, id string, req CreateRequest) (RecreateResult, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return RecreateResult{}, err
	}
	old, err := s.Inspect(ctx, envID, id)
	if err != nil {
		return RecreateResult{}, err
	}
	if IsBuiltInName(old.Name) {
		return RecreateResult{}, fmt.Errorf("%w: %s", ErrBuiltIn, old.Name)
	}
	plan, err := s.prepareCreate(ctx, envID, req)
	if err != nil {
		return RecreateResult{}, err
	}
	restorePlan, err := s.prepareCreate(ctx, envID, createRequestFromInspect(old))
	if err != nil {
		return RecreateResult{}, fmt.Errorf("reading the current definition back: %w", err)
	}
	snaps, err := s.snapshotEndpoints(ctx, envID, cli, old)
	if err != nil {
		return RecreateResult{}, err
	}

	for i, snap := range snaps {
		if err := s.disconnect(ctx, cli, old.ID, snap.id); err != nil {
			s.reconnectAll(ctx, cli, old.ID, snaps[:i], restorePlan.opts.IPAM)
			return RecreateResult{}, fmt.Errorf("disconnecting %s: %w", snap.name, err)
		}
	}
	if err := s.Remove(ctx, envID, old.ID); err != nil {
		s.reconnectAll(ctx, cli, old.ID, snaps, restorePlan.opts.IPAM)
		return RecreateResult{}, err
	}

	created, err := s.execCreate(ctx, envID, plan)
	if err != nil {
		restored, rErr := s.execCreate(context.WithoutCancel(ctx), envID, restorePlan)
		if rErr != nil {
			slog.Error("network recreate: restoring the original failed", "env", envID, "network", old.Name, "error", rErr)
			return RecreateResult{}, fmt.Errorf("%w: %w (restore: %w)", ErrRecreateLost, err, rErr)
		}
		s.reconnectAll(context.WithoutCancel(ctx), cli, restored.ID, snaps, restorePlan.opts.IPAM)
		return RecreateResult{}, fmt.Errorf("%w: %w", ErrRecreateRestored, err)
	}

	return RecreateResult{
		ID:      created.ID,
		Warning: created.Warning,
		Results: s.reconnectAll(ctx, cli, created.ID, snaps, plan.opts.IPAM),
	}, nil
}

// snapshotEndpoints records every attached container's endpoint and runs the self-container guard
// on each, so a refusal happens before the first disconnect.
func (s *Service) snapshotEndpoints(ctx context.Context, envID string, cli *client.Client, net network.Inspect) ([]endpointSnapshot, error) {
	ids := make([]string, 0, len(net.Containers))
	for cid := range net.Containers {
		ids = append(ids, cid)
	}
	sort.Strings(ids)
	snaps := make([]endpointSnapshot, 0, len(ids))
	for _, cid := range ids { // bounded by the network's attached containers
		guarded, err := s.guardContainer(ctx, envID, cid)
		if err != nil {
			return nil, err
		}
		callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
		res, err := cli.ContainerInspect(callCtx, guarded, client.ContainerInspectOptions{})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("inspecting container %s: %w", cid, err)
		}
		snap := endpointSnapshot{id: res.Container.ID, name: net.Containers[cid].Name}
		if res.Container.NetworkSettings != nil {
			if ep := res.Container.NetworkSettings.Networks[net.Name]; ep != nil {
				snap.settings = network.EndpointSettings{
					Aliases:    ep.Aliases,
					Links:      ep.Links,
					DriverOpts: ep.DriverOpts,
					GwPriority: ep.GwPriority,
					MacAddress: ep.MacAddress,
				}
				snap.ipv4, snap.ipv6 = ep.IPAddress, ep.GlobalIPv6Address
				if ep.IPAMConfig != nil {
					if ep.IPAMConfig.IPv4Address.IsValid() {
						snap.ipv4 = ep.IPAMConfig.IPv4Address
					}
					if ep.IPAMConfig.IPv6Address.IsValid() {
						snap.ipv6 = ep.IPAMConfig.IPv6Address
					}
				}
			}
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

// reconnectAll attaches every snapshot to networkID. An address is pinned only when ipam has a
// subnet holding it; Docker refuses a pinned address on a network without user subnets, and a
// pinned address that is refused anyway is retried unpinned so the container is never left off.
func (s *Service) reconnectAll(ctx context.Context, cli *client.Client, networkID string, snaps []endpointSnapshot, ipam *network.IPAM) []ReconnectOutcome {
	out := make([]ReconnectOutcome, 0, len(snaps))
	for _, snap := range snaps { // bounded by the snapshot taken before the recreate
		res := ReconnectOutcome{Container: snap.id, Name: snap.name}
		if snap.ipv4.IsValid() {
			res.PreviousIPv4 = snap.ipv4.String()
		}
		if snap.ipv6.IsValid() {
			res.PreviousIPv6 = snap.ipv6.String()
		}
		settings := snap.settings
		pinned := &network.EndpointIPAMConfig{}
		kept := true
		for _, addr := range []netip.Addr{snap.ipv4, snap.ipv6} {
			if !addr.IsValid() {
				continue
			}
			if !inUserSubnet(ipam, addr) {
				kept = false
				continue
			}
			if addr.Is4() {
				pinned.IPv4Address = addr
			} else {
				pinned.IPv6Address = addr
			}
		}
		if pinned.IPv4Address.IsValid() || pinned.IPv6Address.IsValid() {
			settings.IPAMConfig = pinned
		}
		err := s.connect(ctx, cli, networkID, snap.id, settings)
		if err != nil && settings.IPAMConfig != nil {
			settings.IPAMConfig = nil
			kept = false
			err = s.connect(ctx, cli, networkID, snap.id, settings)
		}
		res.AddressKept = kept && err == nil
		if err != nil {
			slog.Warn("network recreate: reconnect failed", "network", networkID, "container", snap.id, "error", err)
			res.Error = redact.ErrorDetail(err.Error())
		}
		out = append(out, res)
	}
	return out
}

func inUserSubnet(ipam *network.IPAM, addr netip.Addr) bool {
	if ipam == nil {
		return false
	}
	for _, cfg := range ipam.Config {
		if cfg.Subnet.IsValid() && cfg.Subnet.Contains(addr) {
			return true
		}
	}
	return false
}

func (s *Service) connect(ctx context.Context, cli *client.Client, networkID, containerID string, settings network.EndpointSettings) error {
	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	_, err := cli.NetworkConnect(callCtx, networkID, client.NetworkConnectOptions{Container: containerID, EndpointConfig: &settings})
	return err
}

func (s *Service) disconnect(ctx context.Context, cli *client.Client, networkID, containerID string) error {
	callCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	_, err := cli.NetworkDisconnect(callCtx, networkID, client.NetworkDisconnectOptions{Container: containerID})
	return err
}

// createRequestFromInspect turns a live network back into the request that would make it, which
// is how Recreate restores the original after Docker refuses the new definition.
func createRequestFromInspect(net network.Inspect) CreateRequest {
	req := CreateRequest{
		Name:       net.Name,
		Driver:     net.Driver,
		Internal:   net.Internal,
		Attachable: net.Attachable,
		EnableIPv6: net.EnableIPv6,
		Options:    net.Options,
		Labels:     net.Labels,
		IPAM:       &IPAMRequest{Driver: net.IPAM.Driver, Options: net.IPAM.Options},
	}
	for _, cfg := range net.IPAM.Config {
		c := IPAMConfigRequest{}
		if cfg.Subnet.IsValid() {
			c.Subnet = cfg.Subnet.String()
		}
		if cfg.IPRange.IsValid() {
			c.IPRange = cfg.IPRange.String()
		}
		if cfg.Gateway.IsValid() {
			c.Gateway = cfg.Gateway.String()
		}
		for k, v := range cfg.AuxAddress {
			if c.AuxAddress == nil {
				c.AuxAddress = map[string]string{}
			}
			c.AuxAddress[k] = v.String()
		}
		req.IPAM.Config = append(req.IPAM.Config, c)
	}
	if info := hostLinkFromLabels(net.Labels); info != nil {
		req.HostLink = &HostLinkRequest{Name: info.Name, IPv4: info.IPv4, IPv6: info.IPv6, IPv6Auto: info.IPv6Auto}
	}
	return req
}
