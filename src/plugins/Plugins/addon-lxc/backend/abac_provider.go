// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"

	abacspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/abac"
)

// ABACResourceTypes declares system containers. It binds no route: a prefix of
// /api/system-containers/{id} would also catch backups/… and backup-plans/…, whose
// second segment is not a container id.
func (p *LXCPlugin) ABACResourceTypes() []abacspec.ResourceType {
	return []abacspec.ResourceType{{
		Type:     "system_container",
		Label:    "System container",
		Category: "ct",
		Attributes: []abacspec.Attribute{
			{Path: "resource.attrs.osFamily", Type: "string", Description: "Distribution family"},
			{Path: "resource.attrs.privileged", Type: "bool", Description: "Runs privileged"},
			{Path: "resource.attrs.poolId", Type: "string", Description: "Storage pool the root filesystem lives in"},
		},
	}}
}

// ResolveABACResource loads one system container for policy evaluation.
func (p *LXCPlugin) ResolveABACResource(ctx context.Context, _, _, id string) (*abacspec.Resource, error) {
	if p.svc == nil || id == "" {
		return nil, nil
	}
	ct, err := p.svc.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &abacspec.Resource{
		ID: ct.ID, Name: ct.Name, State: ct.State, Environment: ct.EnvironmentID,
		Labels: map[string]string{},
		Attrs:  map[string]any{"osFamily": ct.OSFamily, "privileged": ct.Privileged, "poolId": ct.PoolID},
	}, nil
}

var _ abacspec.Provider = (*LXCPlugin)(nil)
