// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// selfAccessOverridesQuery is every direct grant of a key to a user, for all environments or
// one named in the IN list the caller appends; the caller drops expired rows. Stack- and
// row-scoped grants are left out, as the role query leaves out stack-scoped assignments.
const selfAccessOverridesQuery = `SELECT u.id, u.username, COALESCE(p.expires_at, '')
 FROM user_permissions p JOIN users u ON u.id = p.user_id
 WHERE p.effect = 'grant' AND p.permission_key = ? AND p.stack_name IS NULL AND p.resource_id IS NULL
   AND (p.environment_id IS NULL OR p.environment_id IN (`

// gatherSelfAccessOverrides adds the users granted containers.manage on a self environment
// through a user permission override: a role review never sees them, yet the grant is just
// as admin-equivalent there. envArgs are the self environment ids.
func (s *Service) gatherSelfAccessOverrides(ctx context.Context, envArgs []any, in *Inputs) error {
	now := time.Now()
	holders, err := s.containersManageOverrideHolders(ctx, envArgs, now)
	if err != nil {
		return err
	}
	// bounded by the override rows read above (LIMIT 1000), in practice a handful.
	for _, h := range holders {
		guarded, err := s.holdsGlobalHostSelf(ctx, h.id, now)
		if err != nil {
			return err
		}
		in.SelfAccessUsers = append(in.SelfAccessUsers, h.name)
		if !guarded {
			in.SelfAccessUsersUnguarded = append(in.SelfAccessUsersUnguarded, h.name)
		}
	}
	return nil
}

// overrideHolder is a user holding a direct grant.
type overrideHolder struct{ id, name string }

// containersManageOverrideHolders reads the distinct users with an unexpired direct
// containers.manage grant on one of envArgs or on every environment.
func (s *Service) containersManageOverrideHolders(ctx context.Context, envArgs []any, now time.Time) ([]overrideHolder, error) {
	args := append([]any{string(rbac.PermContainersManage)}, envArgs...)
	query := selfAccessOverridesQuery + placeholders(len(envArgs)) + ")) ORDER BY u.username LIMIT 1000" // SQL identifier: placeholders emits only "?" markers, one per self environment
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading containers.manage overrides: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "user_permissions") }()
	var holders []overrideHolder
	seen := map[string]bool{}
	for rows.Next() {
		var h overrideHolder
		var expires string
		if err := rows.Scan(&h.id, &h.name, &expires); err != nil {
			return nil, fmt.Errorf("reading containers.manage overrides: %w", err)
		}
		if t := parseTime(expires); !t.IsZero() && !t.After(now) {
			continue
		}
		if !seen[h.id] {
			seen[h.id] = true
			holders = append(holders, h)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading containers.manage overrides: %w", err)
	}
	return holders, nil
}

// holdsGlobalHostSelf reports whether userID holds host_self.manage globally, through a
// global role (directly or by group) or a global grant override, and no global deny.
func (s *Service) holdsGlobalHostSelf(ctx context.Context, userID string, now time.Time) (bool, error) {
	granted, denied, err := s.globalOverride(ctx, userID, now)
	if err != nil {
		return false, err
	}
	if denied {
		return false, nil
	}
	if granted {
		return true, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.permissions FROM roles r
 JOIN (SELECT role_id FROM user_roles WHERE user_id = ? AND environment_id IS NULL AND stack_name IS NULL
       UNION ALL
       SELECT gr.role_id FROM group_roles gr JOIN group_members gm ON gm.group_id = gr.group_id
        WHERE gm.user_id = ? AND gr.environment_id IS NULL AND gr.stack_name IS NULL) a ON a.role_id = r.id
 LIMIT 1000`, userID, userID)
	if err != nil {
		return false, fmt.Errorf("reading global roles of %s: %w", userID, err)
	}
	defer func() { closeutil.Report(rows.Close(), "roles") }()
	for rows.Next() {
		var permsJSON string
		if err := rows.Scan(&permsJSON); err != nil {
			return false, fmt.Errorf("reading global roles of %s: %w", userID, err)
		}
		var perms []string
		if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
			continue
		}
		for _, p := range perms {
			if p == string(rbac.PermWildcard) || p == string(rbac.PermHostSelfManage) {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

// globalOverride reads userID's unexpired global grant and deny overrides of host_self.manage.
func (s *Service) globalOverride(ctx context.Context, userID string, now time.Time) (granted, denied bool, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT effect, COALESCE(expires_at, '') FROM user_permissions
 WHERE user_id = ? AND permission_key = ? AND environment_id IS NULL AND stack_name IS NULL AND resource_id IS NULL
 LIMIT 1000`, userID, string(rbac.PermHostSelfManage))
	if err != nil {
		return false, false, fmt.Errorf("reading host_self.manage overrides of %s: %w", userID, err)
	}
	defer func() { closeutil.Report(rows.Close(), "user_permissions") }()
	for rows.Next() {
		var effect, expires string
		if err := rows.Scan(&effect, &expires); err != nil {
			return false, false, fmt.Errorf("reading host_self.manage overrides of %s: %w", userID, err)
		}
		if t := parseTime(expires); !t.IsZero() && !t.After(now) {
			continue
		}
		switch effect {
		case "grant":
			granted = true
		case "deny":
			denied = true
		}
	}
	return granted, denied, rows.Err()
}
