// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// breakGlassActive reports whether userID has an open, unexpired break-glass
// session. Like elevations it is read per resolution and never cached, so the
// wildcard stops the moment expires_at passes or the session is ended.
//
// A missing table reads as "no session": hand-built test schemas and a node
// mid-migration must keep resolving role permissions rather than fail closed.
func (s *Service) breakGlassActive(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.db == nil || userID == "" {
		return false, nil
	}
	// Break-glass is a person's emergency session; a service principal never
	// has one, so the wildcard can never reach an unattended run.
	if service, err := s.IsServicePrincipal(ctx, userID); err != nil || service {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM break_glass_sessions
		  WHERE user_id = ? AND ended_at IS NULL AND expires_at > ?
		  LIMIT 1`, userID, now).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("reading break-glass session: %w", err)
	}
	return true, nil
}

// BreakGlassActive exposes the same check to the simulator and to the
// break-glass module, so both read the one rule the permission path enforces.
func (s *Service) BreakGlassActive(ctx context.Context, userID string) (bool, error) {
	return s.breakGlassActive(ctx, userID)
}
