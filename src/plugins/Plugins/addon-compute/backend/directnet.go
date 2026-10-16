// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// Agent message types for the two network operations. They are named here
// rather than imported because the host's protocol constants are not part of
// the plugin surface.
const (
	msgNetEnsure = "net_ensure_request"
	msgNetRemove = "net_remove_request"
)

// agentNetwork is the wire shape of a network as the agent reads it.
//
// It is also the wire shape of a bridged or managed network (wireNetwork in
// agent_bridge.go): the direct mode's three extra fields are empty there and
// omitted, so both kinds cross in the one struct.
type agentNetwork struct {
	NetworkID      string `json:"networkId,omitempty"`
	Name           string `json:"name,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Bridge         string `json:"bridge,omitempty"`
	VLANTag        int    `json:"vlanTag,omitempty"`
	SubnetCIDR     string `json:"subnetCidr,omitempty"`
	GatewayIP      string `json:"gatewayIp,omitempty"`
	DHCPEnabled    bool   `json:"dhcpEnabled,omitempty"`
	DHCPRangeStart string `json:"dhcpRangeStart,omitempty"`
	DHCPRangeEnd   string `json:"dhcpRangeEnd,omitempty"`
	NATEnabled     bool   `json:"natEnabled,omitempty"`
	UplinkLink     string `json:"uplinkLink,omitempty"`
	LinkMode       string `json:"linkMode,omitempty"`
	HostAccess     bool   `json:"hostAccess,omitempty"`
	HostAccessIP   string `json:"hostAccessIp,omitempty"`
	HostLinkName   string `json:"hostLinkName,omitempty"`
	IPv6Enabled    bool   `json:"ipv6Enabled,omitempty"`
	HostAccessIPv6 string `json:"hostAccessIpv6,omitempty"`
}

// isDirectMode reports whether a mode stacks guests on an adapter rather than
// attaching them to a bridge.
//
// ipvlan is withdrawn and validation refuses it on create, but a row recorded
// before it went still owns a host shim on its agent, and deleting one has to
// reach the same teardown.
func isDirectMode(mode string) bool {
	return mode == "macvlan" || mode == "ipvlan"
}

// ensureDirectNetwork asks the agent to prepare a macvlan network.
func (s *Service) ensureDirectNetwork(ctx context.Context, envID string, net agentNetwork) error {
	return s.sendNetCommand(ctx, envID, msgNetEnsure, net)
}

// removeDirectNetwork asks the agent to release what a direct network owns.
func (s *Service) removeDirectNetwork(ctx context.Context, envID string, net agentNetwork) error {
	return s.sendNetCommand(ctx, envID, msgNetRemove, net)
}

func (s *Service) sendNetCommand(ctx context.Context, envID, messageType string, net agentNetwork) error {
	payload, err := json.Marshal(net)
	if err != nil {
		return fmt.Errorf("encoding the network: %w", err)
	}
	_, err = s.rpc.send(ctx, envID, messageType, payload)
	return err
}

// directNetworkFrom builds the wire shape from a create request.
func directNetworkFrom(id string, req NetworkCreateRequest) agentNetwork {
	return agentNetwork{
		NetworkID:    id,
		Name:         req.Name,
		Mode:         req.Mode,
		VLANTag:      req.VLANTag,
		UplinkLink:   req.UplinkLink,
		LinkMode:     req.LinkMode,
		HostAccess:   req.HostAccess,
		HostAccessIP: req.HostAccessIP,

		HostLinkName:   req.HostLinkName,
		IPv6Enabled:    req.IPv6Enabled,
		HostAccessIPv6: req.HostAccessIPv6,
	}
}

// directRemovalFrom builds the teardown payload for a stored network.
//
// HostLinkName travels so the agent removes the shim the operator named rather
// than looking for the generated default and leaving theirs behind.
func directRemovalFrom(n Network) agentNetwork {
	return agentNetwork{
		NetworkID: n.ID, Name: n.Name, Mode: n.Mode,
		VLANTag: n.VLANTag, UplinkLink: n.UplinkLink,
		HostLinkName: n.HostLinkName,
	}
}

// directNeedsTeardown reports whether an edit has to take the old host
// attachments down before the new ones go up.
//
// A rename must: the agent finds a shim by name, so ensuring the new name
// would leave the old interface on the host. Turning IPv6 off must too, because
// ensure only ever adds or replaces addresses and would keep the old v6 ones.
func directNeedsTeardown(existing Network, req NetworkCreateRequest) bool {
	return existing.UplinkLink != req.UplinkLink ||
		existing.VLANTag != req.VLANTag ||
		existing.LinkMode != req.LinkMode ||
		(existing.HostAccess && !req.HostAccess) ||
		existing.HostLinkName != req.HostLinkName ||
		(existing.IPv6Enabled && !req.IPv6Enabled)
}
