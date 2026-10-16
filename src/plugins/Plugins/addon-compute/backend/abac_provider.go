// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"

	abacspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/abac"
)

// ABACResourceTypes declares storage pools and guest networks, the shared compute
// resources both guest add-ons build on.
func (p *ComputePlugin) ABACResourceTypes() []abacspec.ResourceType {
	return []abacspec.ResourceType{
		{
			Type: "storage_pool", Label: "Storage pool", Category: "compute",
			Routes: []abacspec.Route{{Prefix: "/api/compute/pools/{id}", IDParam: "id"}},
			Attributes: []abacspec.Attribute{
				{Path: "resource.attrs.backend", Type: "string", Description: "dir, zfs, btrfs, lvm …"},
				{Path: "resource.attrs.isDefault", Type: "bool", Description: "The environment's default pool"},
			},
		},
		{
			Type: "guest_network", Label: "Guest network", Category: "compute",
			Routes: []abacspec.Route{{Prefix: "/api/compute/networks/{id}", IDParam: "id"}},
			Attributes: []abacspec.Attribute{
				{Path: "resource.attrs.mode", Type: "string", Description: "bridge, nat, routed …"},
				{Path: "resource.attrs.vlanTag", Type: "number", Description: "VLAN tag, 0 when untagged"},
			},
		},
	}
}

// ResolveABACResource loads one pool or network for policy evaluation.
func (p *ComputePlugin) ResolveABACResource(ctx context.Context, resourceType, _, id string) (*abacspec.Resource, error) {
	if p.svc == nil || id == "" {
		return nil, nil
	}
	switch resourceType {
	case "storage_pool":
		pool, err := p.svc.GetPool(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &abacspec.Resource{ID: pool.ID, Name: pool.Name, State: pool.State, Environment: pool.EnvironmentID,
			Labels: map[string]string{}, Attrs: map[string]any{"backend": pool.Backend, "isDefault": pool.IsDefault}}, nil
	case "guest_network":
		n, err := p.svc.GetNetwork(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &abacspec.Resource{ID: n.ID, Name: n.Name, Environment: n.EnvironmentID,
			Labels: map[string]string{}, Attrs: map[string]any{"mode": n.Mode, "vlanTag": n.VLANTag}}, nil
	}
	return nil, nil
}

var _ abacspec.Provider = (*ComputePlugin)(nil)
