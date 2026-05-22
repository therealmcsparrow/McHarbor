// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"github.com/docker/docker/api/types/network"
)

// CreateRequest is the JSON body for POST /networks.
type CreateRequest struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	Internal   bool              `json:"internal,omitempty"`
	Attachable bool              `json:"attachable,omitempty"`
	IPAM       *network.IPAM     `json:"ipam,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
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
	IPAM       network.IPAM      `json:"IPAM"`
	Containers int               `json:"Containers"`
	Options    map[string]string `json:"Options"`
	Labels     map[string]string `json:"Labels"`
	Created    string            `json:"Created"`
}
