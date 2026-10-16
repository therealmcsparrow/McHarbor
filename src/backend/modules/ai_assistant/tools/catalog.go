// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Descriptor is the LLM-facing view of a tool. The catalog endpoint
// returns a []Descriptor filtered by the caller's effective permissions.
type Descriptor struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	InputSchema     map[string]any `json:"inputSchema"`
	ReadOnly        bool           `json:"readOnly"`
	RequiresConsent bool           `json:"requiresConsent"`
}

// Authorizer is the subset of rbac.Service that the catalog needs.
// Defined as an interface so tests can stub it.
type Authorizer interface {
	HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error)
}

// CatalogFor returns every tool whose Required permissions the user
// holds. Tools with no required permissions are always included.
// envID is empty when the user is browsing globally (e.g. the AI
// settings tab).
func CatalogFor(ctx context.Context, authz Authorizer, userID, envID string) []Descriptor {
	all := All()
	out := make([]Descriptor, 0, len(all))
	for _, t := range all {
		if len(t.Required) == 0 {
			out = append(out, descriptorFor(t))
			continue
		}
		ok := true
		var firstErr error
		for _, perm := range t.Required {
			allowed, err := authz.HasPermission(ctx, userID, envID, perm)
			if err != nil {
				firstErr = err
				break
			}
			if !allowed {
				ok = false
				break
			}
		}
		if firstErr == nil && ok {
			out = append(out, descriptorFor(t))
		}
	}
	return out
}

func descriptorFor(t *Tool) Descriptor {
	return Descriptor{
		Name:            t.Name,
		Description:     t.Description,
		InputSchema:     t.InputSchema,
		ReadOnly:        t.ReadOnly,
		RequiresConsent: t.RequiresConsent,
	}
}
