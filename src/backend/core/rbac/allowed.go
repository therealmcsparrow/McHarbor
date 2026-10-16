// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"log/slog"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// AllowedOn answers the route gate's question for an environment the request
// names somewhere other than `?env=`, such as a move's target in the body. The
// route middleware only ever checks `?env=`, so a handler that acts on a second
// environment must ask this, or a grant on one environment reaches the other.
func (s *Service) AllowedOn(r *http.Request, envID string, perm Permission) (bool, error) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		return false, nil
	}
	if user.ID == "system" {
		return true, nil
	}
	return s.decideRoute(r, user.ID, envID, perm)
}

// GateOn is AllowedOn for a handler: it answers 403 (recording the denial) or
// 500 itself and reports whether the handler may continue. An empty envID is
// the request's own environment, which the route gate has already checked.
func (s *Service) GateOn(w http.ResponseWriter, r *http.Request, envID string, perm Permission) bool {
	if envID == "" {
		return true
	}
	allowed, err := s.AllowedOn(r, envID, perm)
	if err != nil {
		slog.Error("rbac: deciding on a named environment", "env", envID, "permission", perm, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if !allowed {
		RecordDenied(r, auth.UserFromContext(r.Context()), perm, envID)
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	return true
}
