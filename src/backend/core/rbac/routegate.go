// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import "net/http"

// RouteDecision answers exactly what RequirePermission would for r, so a
// middleware ahead of the route (the approval gate) queues only callers the route
// itself would let through.
func (s *Service) RouteDecision(r *http.Request, userID, envID string, perm Permission) (bool, error) {
	return s.decideRoute(r, userID, envID, perm)
}
