// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/abac"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// registerABACResources publishes the resource types a plugin declares through the
// optional abac.Provider contract, so policies can target them and plugin routes can
// resolve them. A plugin without the contract simply contributes none.
func registerABACResources(slug string, p any) {
	rbac.UnregisterResourceTypes(slug)
	provider, ok := p.(abac.Provider)
	if !ok {
		return
	}
	for _, t := range provider.ABACResourceTypes() {
		if t.Type == "" {
			continue
		}
		info := rbac.ResourceTypeInfo{Type: t.Type, Label: t.Label, Category: t.Category, Source: slug}
		for _, r := range t.Routes {
			info.Routes = append(info.Routes, rbac.ResourceRoute{Prefix: r.Prefix, IDParam: r.IDParam})
		}
		for _, a := range t.Attributes {
			info.Attributes = append(info.Attributes, rbac.ResourceAttribute{Path: a.Path, Type: a.Type, Description: a.Description})
		}
		typ := t.Type
		rbac.RegisterResourceType(info, func(ctx context.Context, envID, id string) (*rbac.ResourceAttributes, error) {
			res, err := provider.ResolveABACResource(ctx, typ, envID, id)
			if err != nil || res == nil {
				return nil, err
			}
			env := res.Environment
			if env == "" {
				env = envID
			}
			return &rbac.ResourceAttributes{Type: typ, ID: res.ID, Name: res.Name, Owner: res.Owner, State: res.State,
				Environment: env, Labels: res.Labels, Attrs: res.Attrs}, nil
		})
	}
}
