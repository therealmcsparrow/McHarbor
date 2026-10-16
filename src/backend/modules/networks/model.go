// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"github.com/moby/moby/api/types/network"
)

// CreateRequest is the JSON body for POST /networks.
type CreateRequest struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	Internal   bool              `json:"internal,omitempty"`
	Attachable bool              `json:"attachable,omitempty"`
	EnableIPv6 bool              `json:"enableIPv6,omitempty"`
	IPAM       *IPAMRequest      `json:"ipam,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	// HostLink asks for a host-side interface on the macvlan parent, so the host can reach the
	// containers on the network. It is stored as labels and never sent to Docker as written.
	HostLink *HostLinkRequest `json:"hostLink,omitempty"`
}

// HostLinkRequest names the host interface to create beside a macvlan network. IPv6 is a CIDR, or
// IPv6Auto asks for SLAAC; the two are mutually exclusive.
type HostLinkRequest struct {
	Name     string `json:"name"`
	IPv4     string `json:"ipv4,omitempty"`
	IPv6     string `json:"ipv6,omitempty"`
	IPv6Auto bool   `json:"ipv6Auto,omitempty"`
}

// HostLinkInfo is the host link a network carries, read back from its labels. IPv6 is "auto" for SLAAC.
type HostLinkInfo struct {
	Name     string `json:"name"`
	IPv4     string `json:"ipv4,omitempty"`
	IPv6     string `json:"ipv6,omitempty"`
	IPv6Auto bool   `json:"ipv6Auto,omitempty"`
}

// IPAMRequest mirrors network.IPAM with string addresses, so a malformed subnet or range is
// reported against its field instead of failing the whole body decode.
type IPAMRequest struct {
	Driver  string              `json:"Driver,omitempty"`
	Options map[string]string   `json:"Options,omitempty"`
	Config  []IPAMConfigRequest `json:"Config,omitempty"`
}

// IPAMConfigRequest is one subnet block of an IPAMRequest; the keys match the Docker API's.
type IPAMConfigRequest struct {
	Subnet     string            `json:"Subnet,omitempty"`
	IPRange    string            `json:"IPRange,omitempty"`
	Gateway    string            `json:"Gateway,omitempty"`
	AuxAddress map[string]string `json:"AuxiliaryAddresses,omitempty"`
}

// ConnectRequest is the JSON body for POST /networks/{id}/connect.
type ConnectRequest struct {
	Container string `json:"container"`
}

// DisconnectRequest is the JSON body for POST /networks/{id}/disconnect.
type DisconnectRequest struct {
	Container string `json:"container"`
	Force     bool   `json:"force,omitempty"`
}

// NetworkSummary is a simplified network info for list responses.
// JSON tags use PascalCase to match Docker API convention and frontend types.
type NetworkSummary struct {
	ID         string            `json:"Id"`
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Scope      string            `json:"Scope"`
	Internal   bool              `json:"Internal"`
	Attachable bool              `json:"Attachable"`
	EnableIPv6 bool              `json:"EnableIPv6"`
	IPAM       network.IPAM      `json:"IPAM"`
	Containers int               `json:"Containers"`
	Options    map[string]string `json:"Options"`
	Labels     map[string]string `json:"Labels"`
	Created    string            `json:"Created"`
	BuiltIn    bool              `json:"BuiltIn"`
	// ContainerNames lists the containers attached to this network,
	// leading "/" trimmed, alongside Containers' count.
	ContainerNames []string `json:"ContainerNames,omitempty"`
	// HostLink is the host interface this network carries, parsed from its labels.
	HostLink *HostLinkInfo `json:"HostLink,omitempty"`
}
