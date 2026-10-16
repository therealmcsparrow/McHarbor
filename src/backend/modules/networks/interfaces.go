// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Interface sources, reported so the UI can say where a suggestion came from.
const (
	// InterfaceSourceAgent is a link the environment's agent enumerated on the
	// host itself, with its state and addressing.
	InterfaceSourceAgent = "agent"
	// InterfaceSourceInUse is a parent already referenced by a macvlan or
	// ipvlan network on this host. It proves the name is valid there without
	// proving anything about the link's current state.
	InterfaceSourceInUse = "in-use"
)

// parentInterfaceKinds are the link kinds a macvlan or ipvlan parent may be.
//
// A bridge is accepted by the kernel and defeats the purpose of macvlan, so it
// is left out; veth and tap are container plumbing, not uplinks.
var parentInterfaceKinds = map[string]struct{}{
	"physical": {},
	"bond":     {},
	"vlan":     {},
}

// HostInterface is one candidate macvlan or ipvlan parent on a Docker host.
type HostInterface struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind,omitempty"`
	Source    string   `json:"source"`
	Up        bool     `json:"up"`
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
	// InUseBy names the networks already using this link as their parent, which
	// is a hard warning: a second macvlan network on the same parent is legal
	// and almost never what the operator meant.
	InUseBy []string `json:"inUseBy,omitempty"`
	// HasDefaultRoute marks the link the host's own default route leaves by.
	// Attaching macvlan to it works, but the host loses the ability to talk to
	// its own containers on that network, which is the classic surprise.
	HasDefaultRoute bool `json:"hasDefaultRoute,omitempty"`
}

// HostInterfacesResponse is the body of GET /networks/host-interfaces.
type HostInterfacesResponse struct {
	Interfaces []HostInterface `json:"interfaces"`
	// AgentAvailable is false when nothing could enumerate the host, which is
	// the difference between "this host has no usable NIC" and "McHarbor
	// cannot see this host's NICs".
	AgentAvailable bool `json:"agentAvailable"`
	// Unavailable says why nothing could enumerate the host when AgentAvailable is false:
	// "agent_unreachable" for an agent environment whose agent did not answer, "containerized"
	// for a local environment McHarbor reads from inside a container.
	Unavailable string `json:"unavailable,omitempty"`
	// HostLinkAvailable says whether a host link can be created on this environment; when it
	// cannot, HostLinkUnavailable is the reason (agent_unreachable, agent_outdated, containerized).
	HostLinkAvailable   bool   `json:"hostLinkAvailable"`
	HostLinkUnavailable string `json:"hostLinkUnavailable,omitempty"`
}

// Reasons HostInterfacesResponse.Unavailable carries.
const (
	UnavailableAgentUnreachable = "agent_unreachable"
	UnavailableContainerized    = "containerized"
)

// HostInterfaces returns the links that can serve as a macvlan or ipvlan parent
// on envID.
//
// Two sources, because neither is complete on its own: the agent knows the
// host's real links but only exists on agent environments, while the parents of
// existing macvlan networks are readable through Docker anywhere and are proof
// a name works on that host.
func (s *Service) HostInterfaces(ctx context.Context, envID string) (HostInterfacesResponse, error) {
	inUse, err := s.parentsInUse(ctx, envID)
	if err != nil {
		return HostInterfacesResponse{}, err
	}

	links, agentAvailable, unavailable := s.hostLinks(ctx, envID)

	byName := make(map[string]*HostInterface, len(links)+len(inUse))
	order := make([]string, 0, len(links)+len(inUse))

	for _, link := range links {
		name := strings.TrimSpace(link.Name)
		if name == "" || link.Managed {
			continue
		}
		if _, ok := parentInterfaceKinds[strings.ToLower(link.Kind)]; !ok {
			continue
		}
		byName[name] = &HostInterface{
			Name:            name,
			Kind:            strings.ToLower(link.Kind),
			Source:          InterfaceSourceAgent,
			Up:              link.Up,
			MAC:             link.MAC,
			MTU:             link.MTU,
			Addresses:       link.Addresses,
			HasDefaultRoute: link.HasDefaultRoute,
		}
		order = append(order, name)
	}

	for name, networks := range inUse {
		if existing, ok := byName[name]; ok {
			existing.InUseBy = networks
			continue
		}
		byName[name] = &HostInterface{
			Name:    name,
			Source:  InterfaceSourceInUse,
			InUseBy: networks,
		}
		order = append(order, name)
	}

	sort.Strings(order)
	out := make([]HostInterface, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}

	linker, linkReason := s.hostLinkerFor(ctx, envID)
	return HostInterfacesResponse{Interfaces: out, AgentAvailable: agentAvailable, Unavailable: unavailable, HostLinkAvailable: linker != nil, HostLinkUnavailable: linkReason}, nil
}

// hostLinks enumerates envID's host: through its agent, or, for a McHarbor running directly
// on the host behind a local Docker socket, from McHarbor's own network namespace. When neither
// works it says why, because "inside a container" was the wrong advice for both other cases.
func (s *Service) hostLinks(ctx context.Context, envID string) ([]coreagent.ComputeHostLink, bool, string) {
	links, agentErr := s.agentLinks(ctx, envID)
	if agentErr == nil {
		return links, true, ""
	}
	if s.pool != nil && s.pool.IsAgentEnvContext(ctx, envID) {
		slog.Warn("host interfaces: the environment's agent did not list its links", "env", envID, "error", agentErr)
		return nil, false, UnavailableAgentUnreachable
	}
	if nativeHost.Load() && s.localSocketEnv(ctx, envID) {
		links, err := localLinks()
		if err != nil {
			slog.Warn("host interfaces: reading this host's links failed", "env", envID, "error", err)
			return nil, false, UnavailableContainerized
		}
		return links, true, ""
	}
	return nil, false, UnavailableContainerized
}

// agentLinks asks the environment's agent for the host's link inventory.
func (s *Service) agentLinks(ctx context.Context, envID string) ([]coreagent.ComputeHostLink, error) {
	if s.agents == nil {
		return nil, errors.New("no agent pool")
	}
	transport := s.agents.ResolveTransport(envID)
	if transport == nil {
		return nil, errors.New("the agent is not connected")
	}
	links, err := transport.NetDetect(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("agent net detect: %w", err)
	}
	return links, nil
}

// localSocketEnv reports whether envID's Docker daemon is reached over a local unix socket,
// i.e. runs on the machine McHarbor itself runs on.
func (s *Service) localSocketEnv(ctx context.Context, envID string) bool {
	if s.pool == nil {
		return false
	}
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return false
	}
	return strings.HasPrefix(cli.DaemonHost(), "unix://")
}

// parentsInUse maps each parent option of the host's existing macvlan and
// ipvlan networks to the networks that reference it.
func (s *Service) parentsInUse(ctx context.Context, envID string) (map[string][]string, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	listRes, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	list := listRes.Items

	parents := make(map[string][]string)
	for _, n := range list {
		switch n.Driver {
		case "macvlan", "ipvlan":
		default:
			continue
		}
		parent := strings.TrimSpace(n.Options["parent"])
		if parent == "" {
			continue
		}
		// A parent may carry a VLAN suffix (eth0.10); the link itself is what
		// the picker offers, so both forms are kept as written.
		parents[parent] = append(parents[parent], n.Name)
	}
	return parents, nil
}
