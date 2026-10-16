// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// PrincipalKindService marks a users row that is a service principal: a real
// principal with ordinary role assignments that acts unattended and never signs in.
const PrincipalKindService = "service"

// PrincipalKindUser is every ordinary, interactive account.
const PrincipalKindUser = "user"

// principalKinds caches the kind per service and user id. A kind never changes
// after the row is created and ids are never reused, so an entry cannot go stale.
var principalKinds sync.Map

type principalKey struct {
	svc    *Service
	userID string
}

// IsServicePrincipal reports whether userID is a service principal. A schema
// without principal_kind (a hand-built test schema, a node mid-migration) reads
// as an ordinary user, so role resolution keeps working rather than failing closed.
func (s *Service) IsServicePrincipal(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.db == nil || userID == "" {
		return false, nil
	}
	if cached, ok := principalKinds.Load(principalKey{s, userID}); ok {
		return cached.(bool), nil
	}
	var kind string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(principal_kind, 'user') FROM users WHERE id = ?`, userID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no such column") || strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "Unknown column") || strings.Contains(msg, "Invalid column name") ||
			strings.Contains(msg, "no such table") {
			return false, nil
		}
		return false, fmt.Errorf("reading principal kind: %w", err)
	}
	service := kind == PrincipalKindService
	principalKinds.Store(principalKey{s, userID}, service)
	return service, nil
}
