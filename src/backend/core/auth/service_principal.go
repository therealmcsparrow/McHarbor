// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// PrincipalKindService is the users.principal_kind of a service principal. It
// mirrors rbac.PrincipalKindService; auth cannot import rbac without a cycle.
const PrincipalKindService = "service"

// ErrServicePrincipal is returned when something tries to give a service
// principal a session or a password. It acts unattended and never signs in.
var ErrServicePrincipal = errors.New("auth: a service principal cannot sign in")

// IsServicePrincipal reports whether userID is a service principal. A schema
// without the column reads as an ordinary user; any other read error reads as a
// service principal, so a failed lookup refuses a session rather than grants one.
func (s *Service) IsServicePrincipal(userID string) bool {
	return s.isServicePrincipal(context.Background(), userID)
}

// IsServicePrincipalContext is IsServicePrincipal bound to the caller's context, so a cancelled request stops the principal lookup.
func (s *Service) IsServicePrincipalContext(ctx context.Context, userID string) bool {
	return s.isServicePrincipal(ctx, userID)
}

// isServicePrincipal is IsServicePrincipal with the caller's context.
func (s *Service) isServicePrincipal(ctx context.Context, userID string) bool {
	if s == nil || s.db == nil || userID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, authSettingsQueryTimeout)
	defer cancel()
	var kind string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(principal_kind, 'user') FROM users WHERE id = ?`, userID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no such column") ||
			strings.Contains(msg, "does not exist") || strings.Contains(msg, "Unknown column") ||
			strings.Contains(msg, "Invalid column name") {
			return false
		}
		return true
	}
	return kind == PrincipalKindService
}
