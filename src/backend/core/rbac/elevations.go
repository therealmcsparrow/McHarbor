// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"fmt"
	"time"
)

// activeElevatedPermissions returns the permission keys a user currently holds
// through an approved, unexpired elevation request.
//
// Until this existed, `modules/elevations` was a complete request/approve API
// whose approvals granted nothing: no code outside that module ever read
// `elevation_requests`, so an approver could grant "temporary access" that the
// authorization path never consulted. This is the half that makes the feature
// real.
//
// Three deliberate choices:
//
//   - **The expiry is checked here, not trusted from `status`.** The module's
//     `SweepExpired` only runs when a handler happens to list requests, so a row
//     can sit at `status = 'approved'` well past its window. Filtering on
//     `approved_until > now` means a lapsed grant stops working at the moment it
//     lapses, whether or not anything swept it.
//
//   - **Not cached.** `effectivePermissions` caches the role-derived set,
//     which is correct because roles change rarely and invalidate explicitly.
//     An elevation is time-bounded, so caching it would keep granting the
//     permission after `approved_until` until something happened to evict the
//     entry. A privilege that outlives its window is exactly the bug this
//     feature would be blamed for, so the cost of one indexed lookup per
//     resolution is the right trade.
//
//   - **Environment scope is honoured.** A NULL `environment_id` is a global
//     grant; a set one only applies to that environment. A grant for one
//     environment must not leak into another.
//
// Times are compared as strings: both sides are UTC RFC3339, which orders
// lexicographically, and the module writes `approved_until` with
// `time.Now().UTC().Add(...).Format(time.RFC3339)`.
func (s *Service) activeElevatedPermissions(ctx context.Context, userID, envID string) ([]Permission, error) {
	if s == nil || s.db == nil || userID == "" {
		return nil, nil
	}

	grants, err := s.activeElevations(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	var out, envScoped []Permission
	for _, grant := range grants {
		if grant.EnvironmentID == "" {
			out = append(out, grant.Permission)
		} else {
			envScoped = append(envScoped, grant.Permission)
		}
	}
	// The same rule as role assignments: an elevation approved for one
	// environment cannot carry a global key into it.
	envScoped, err = s.environmentScoped(ctx, envScoped)
	if err != nil {
		return nil, err
	}
	return append(out, envScoped...), nil
}

// ElevationGrant is one approved, unexpired elevation that currently adds a
// permission to a user's interactive session.
type ElevationGrant struct {
	RequestID     string     `json:"requestId"`
	Permission    Permission `json:"permission"`
	EnvironmentID string     `json:"environmentId,omitempty"`
	ApprovedUntil string     `json:"approvedUntil"`
}

// activeElevations is the single query behind both the authorization path and
// the permission explainer, so the two can never disagree about a grant.
func (s *Service) activeElevations(ctx context.Context, userID, envID string) ([]ElevationGrant, error) {
	if s == nil || s.db == nil || userID == "" {
		return nil, nil
	}
	// An elevation is approved for a person in a session. A service principal
	// has neither, so a row naming one — however it got there — grants nothing.
	if service, err := s.IsServicePrincipal(ctx, userID); err != nil || service {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, permission_key, COALESCE(environment_id, ''), approved_until FROM elevation_requests
		  WHERE requester_id = ?
		    AND status = 'approved'
		    AND approved_until IS NOT NULL
		    AND approved_until > ?
		    AND (environment_id IS NULL OR environment_id = '' OR environment_id = ?)
		  LIMIT 1000`,
		userID, now, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("reading active elevations: %w", err)
	}
	defer closeRows(rows)

	var out []ElevationGrant
	for rows.Next() {
		var grant ElevationGrant
		var key string
		if err := rows.Scan(&grant.RequestID, &key, &grant.EnvironmentID, &grant.ApprovedUntil); err != nil {
			return nil, fmt.Errorf("scanning active elevation: %w", err)
		}
		if key != "" {
			grant.Permission = Permission(key)
			out = append(out, grant)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating active elevations: %w", err)
	}
	return out, nil
}

// withElevations appends elevated permissions to a role-derived set, skipping
// any the user already holds so the result stays a set.
func withElevations(base, elevated []Permission) []Permission {
	if len(elevated) == 0 {
		return base
	}
	seen := make(map[Permission]struct{}, len(base))
	for _, p := range base {
		seen[p] = struct{}{}
	}
	out := base
	for _, p := range elevated {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
