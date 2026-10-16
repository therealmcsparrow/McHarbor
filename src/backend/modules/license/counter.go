// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package license enforces the seat licence: activation, periodic check-in,
// usage sampling and the over-limit notices shown to operators.
package license

import (
	"context"
	"fmt"
)

// count reads what this install is currently using.
//
// A seat is an active environment, however it is reached: an agent and a
// direct socket, TCP, TLS, SSH or Podman connection each manage one host, so
// each costs one seat. The agent behind an environment is not counted again
// (agents no longer claim a seat of their own), and the instance is not a seat
// beyond the environments it manages. The agent count is kept for the usage
// history only.
//
// Every number here stays on this install. The licence handshake has no
// usage endpoint, and the check-in body carries the key, the instance id,
// the hostname and the build version — nothing from this function.
func (s *Service) count(ctx context.Context) (counts, error) {
	var c counts

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM environments WHERE is_active = 1`,
	).Scan(&c.servers); err != nil {
		return counts{}, fmt.Errorf("counting active environments: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM environments WHERE connection_type = 'agent' AND is_active = 1`,
	).Scan(&c.agents); err != nil {
		return counts{}, fmt.Errorf("counting agent environments: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM environments`,
	).Scan(&c.environments); err != nil {
		return counts{}, fmt.Errorf("counting environments: %w", err)
	}

	// Reported for context at renewal only — users are not a billed
	// unit today, and no path here treats them as one.
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE is_active = 1`,
	).Scan(&c.users); err != nil {
		return counts{}, fmt.Errorf("counting users: %w", err)
	}

	return c, nil
}
