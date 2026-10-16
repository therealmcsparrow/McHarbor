// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// AuthzRequest describes an authorization decision request. It carries
// every signal the engine needs: the subject (user id), the action
// (permission key), and the optional resource context (environment,
// stack, owner). Stack/owner are mutually relevant — for an owner-scope
// permission a missing owner_user_id is a denial.
//
// Passing this struct (instead of positional args) makes call sites
// self-documenting and lets us add new fields without breaking every
// caller.
type AuthzRequest struct {
	UserID             string
	Permission         Permission
	EnvironmentID      string              // empty = global
	StackName          string              // empty = not stack-scoped
	OwnerUserID        string              // for owner-scope permissions
	ResourceType       string              // optional: 'container', 'stack', 'backup_plan', ...
	ResourceID         string              // optional row id
	ResourceAttributes *ResourceAttributes // populated by middleware for ABAC policies
	Request            *RequestAttributes  // the HTTP call, for request.* in policies
	AllowSystemBypass  bool                // default true; set false to deny system user too
	// APIKey is set when the request authenticated with a Bearer API
	// key. The key's own permission set is checked first; the owner
	// path below then runs unchanged, which yields the intersection.
	APIKey *APIKeyPrincipal
}

// Authorize returns the authorization decision for the request. It
// performs role-based lookup, then applies user-level overrides from
// the user_permissions table. The decision reasons are intentionally
// coarse ("allowed" vs "denied") — we do not leak permission names
// back to the user (see /api/auth/permissions/catalog which is
// access-controlled).
func (s *Service) Authorize(req AuthzRequest) (allowed bool, err error) {
	return s.AuthorizeContext(context.Background(), req)
}

// AuthorizeContext is Authorize bound to ctx, so the override and role lookups stop when the request is cancelled.
func (s *Service) AuthorizeContext(ctx context.Context, req AuthzRequest) (allowed bool, err error) {
	return s.authorize(ctx, req, true)
}

// authorize is AuthorizeContext with the ABAC-denied audit under record; Explain
// passes false so replaying someone's decision leaves no audit trail of its own.
func (s *Service) authorize(ctx context.Context, req AuthzRequest, record bool) (allowed bool, err error) {
	if req.UserID == "" {
		return false, nil
	}

	// abacEnabled gates the ABAC layer behind the rbac_v3_enabled
	// setting. A nil rolloutV3 means the rollout is OFF (legacy mode)
	// so we never skip the ABAC evaluation when an explicit rollout
	// flag is configured but unresolved.
	if s == nil {
		return false, nil
	}

	// System user bypass (AUTH_DISABLE=true). Disabled when AllowSystemBypass=false.
	if req.AllowSystemBypass && (req.UserID == "system" || req.UserID == "__legacy_owner__") {
		return true, nil
	}

	// 0. API key fence. A key must itself carry the permission (through
	// its roles or explicit scopes) and must be allowed in the target
	// environment before the owner's decision is consulted.
	if req.APIKey != nil {
		if !req.APIKey.allowsEnvironment(req.EnvironmentID) {
			return false, nil
		}
		keyPerms, keyErr := s.keyPermissions(ctx, req.APIKey)
		if keyErr != nil {
			return false, keyErr
		}
		if !hasPermission(keyPerms, req.Permission) {
			return false, nil
		}
	}

	// 1. Check user-level deny overrides first (deny wins).
	denied, err := s.matchOverride(ctx, req, "deny")
	if err != nil {
		return false, err
	}
	if denied {
		return false, nil
	}

	// 2. Check user-level grant overrides.
	granted, err := s.matchOverride(ctx, req, "grant")
	if err != nil {
		return false, err
	}
	if granted {
		return true, nil
	}

	// 3. Owner-scope fast path: if permission scope is 'owner' and the
	// resource's owner_user_id matches the caller, allow.
	if s.isOwner(req) {
		return true, nil
	}

	// 4. ABAC attribute-based policy check. Runs only when both a
	// policy service and an attribute bundle are configured AND the
	// feature is enabled via the rollout flag (rbac_v3_enabled). When
	// the flag is off we skip the ABAC layer entirely and fall through
	// to the role-based check below — admins can preview ABAC policies
	// without affecting the live gate. Resource attributes are
	// populated by the calling middleware via ResourceAttributes.
	if s.policies != nil && s.attributes != nil && s.abacEnabled(ctx) {
		// Env is left nil so authorizeWithAttributes looks it up; a pre-seeded
		// Env{ID} would skip that lookup and hide env.name from every policy.
		attrs := &Attributes{
			Resource: req.ResourceAttributes,
			Request:  req.Request,
		}
		return s.authorizeWithAttributes(ctx, req, attrs, record)
	}

	// 5. Standard role-based check (owner path; the key fence already ran).
	return s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
}

// matchOverride returns true when a user_permissions row matches the
// request for the given effect. Rows are walked most-specific first and the
// first one whose non-null fields all match wins; expired rows are ignored.
func (s *Service) matchOverride(ctx context.Context, req AuthzRequest, effect string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT environment_id, stack_name, resource_type, resource_id, expires_at
		  FROM user_permissions
		 WHERE user_id = ? AND permission_key = ? AND effect = ?
		 ORDER BY
		    CASE WHEN resource_id     IS NOT NULL THEN 4 ELSE 0 END +
		    CASE WHEN resource_type   IS NOT NULL THEN 2 ELSE 0 END +
		    CASE WHEN stack_name      IS NOT NULL THEN 1 ELSE 0 END +
		    CASE WHEN environment_id  IS NOT NULL THEN 1 ELSE 0 END
		 DESC
		 LIMIT 1000
	`, req.UserID, string(req.Permission), effect)
	if err != nil {
		return false, err
	}
	defer closeRows(rows)

	for rows.Next() {
		var envID, stackName, resType, resID sql.NullString
		var expiresAt sql.NullString
		if err := rows.Scan(&envID, &stackName, &resType, &resID, &expiresAt); err != nil {
			return false, err
		}
		if !overrideApplies(overrideRow{
			EnvironmentID: envID,
			StackName:     stackName,
			ResourceType:  resType,
			ResourceID:    resID,
			ExpiresAt:     expiresAt,
		}, req, time.Now()) {
			continue
		}
		return true, nil
	}
	return false, rows.Err()
}

// isOwner returns true if the request's permission scope is 'owner'
// AND the caller is the resource's owner. Resources without an owner
// (NULL or the legacy placeholder) are NOT considered owned by anyone
// for the purposes of the owner rule, so the caller falls back to
// the role-based check.
func (s *Service) isOwner(req AuthzRequest) bool {
	if req.OwnerUserID == "" || req.OwnerUserID == "__legacy_owner__" {
		return false
	}
	if req.OwnerUserID != req.UserID {
		return false
	}
	// Only owner-scoped permissions get the fast path.
	scope, ok := s.scopeFor(req.Permission)
	if !ok {
		return false
	}
	return scope == "owner"
}

// scopeFor returns the scope string for a permission from the in-memory
// catalog. Cached at startup — no DB hit per request.
func (s *Service) scopeFor(p Permission) (string, bool) {
	s.scopeOnce.Do(func() {
		s.scopeMap = make(map[Permission]string, len(CatalogSeed()))
		for _, e := range CatalogSeed() {
			s.scopeMap[e.Key] = e.Scope
		}
	})
	v, ok := s.scopeMap[p]
	return v, ok
}

// RequirePermissionV2 is the v2 middleware. It calls Authorize with the
// request's environment context only (no owner). Use this for new
// routes; legacy routes can keep using RequirePermission.
func RequirePermissionV2(svc *Service, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeEnvironment, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}
			envID := response.ParseEnvID(r)
			resType, resID := ResourceFromRoute(r)
			allowed, err := svc.AuthorizeContext(r.Context(), AuthzRequest{
				UserID:        user.ID,
				Permission:    perm,
				EnvironmentID: envID,
				ResourceType:  resType,
				ResourceID:    resID,
				Request:       RequestFromHTTP(r),
				APIKey:        APIKeyFromContext(r.Context()),
			})
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				RecordDenied(r, user, perm, envID)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwnerOrPermission allows when EITHER the caller is the
// resource owner OR holds the named permission. Used for routes like
// "edit my container" — admins keep blanket access, owners can edit
// their own.
//
// The OwnerUserIDResolver callback should fetch the owner_user_id for
// the resource identified by the request. Returning ("", nil) means
// "this resource has no owner, fall through to role check".
func RequireOwnerOrPermission(svc *Service, perm Permission, resolve OwnerUserIDResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeOwner, false, perm); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}
			envID := response.ParseEnvID(r)
			ownerID, resolveErr := resolve(r)
			if resolveErr != nil {
				// An unresolvable owner means the resource has none, so the role check decides.
				slog.WarnContext(r.Context(), "rbac: resolving resource owner failed", "error", resolveErr)
				ownerID = ""
			}

			resType, resID := ResourceFromRoute(r)
			allowed, err := svc.AuthorizeContext(r.Context(), AuthzRequest{
				UserID:        user.ID,
				Permission:    perm,
				EnvironmentID: envID,
				OwnerUserID:   ownerID,
				ResourceType:  resType,
				ResourceID:    resID,
				Request:       RequestFromHTTP(r),
				APIKey:        APIKeyFromContext(r.Context()),
			})
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				RecordDenied(r, user, perm, envID)
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OwnerUserIDResolver looks up the owner_user_id of a resource from the
// request. It is called once per request by RequireOwnerOrPermission.
// Implementations should query a single row by id and return ("", nil)
// when the resource has no owner (legacy / NULL).
type OwnerUserIDResolver func(r *http.Request) (ownerUserID string, err error)

// OwnerByParam is a convenience constructor that resolves the owner
// from a chi URL parameter by calling the supplied lookup function.
// Pass it directly to RequireOwnerOrPermission.
func OwnerByParam(param string, lookup func(r *http.Request, resourceID string) (string, error)) OwnerUserIDResolver {
	return func(r *http.Request) (string, error) {
		return lookup(r, chi.URLParam(r, param))
	}
}

// RequireAccountPermission authorizes self- or admin-scoped account
// permissions. The target user id is taken from a URL parameter and
// the caller is allowed when EITHER:
//   - the caller IS the target user (PermAccountSelfManage)
//   - the caller holds PermAccountManage
func RequireAccountPermission(svc *Service, userIDParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeAccount, false); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}
			target := chi.URLParam(r, userIDParam)
			if target == "" {
				response.BadRequestCode(w, r, i18n.ErrInvalidBody)
				return
			}

			// Self always allowed for own-account ops, except through an
			// API key, which must carry account.self.manage itself so a
			// narrow automation key cannot change its owner's password
			// or MFA.
			if target == user.ID {
				if key := APIKeyFromContext(r.Context()); key != nil {
					keyPerms, err := svc.keyPermissions(r.Context(), key)
					if err != nil {
						response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
						return
					}
					if !hasPermission(keyPerms, PermAccountSelfManage) {
						RecordDenied(r, user, PermAccountSelfManage, "")
						response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}

			allowed, err := svc.AuthorizeContext(r.Context(), AuthzRequest{
				UserID:     user.ID,
				Permission: PermAccountManage,
				APIKey:     APIKeyFromContext(r.Context()),
			})
			if err != nil {
				response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
				return
			}
			if !allowed {
				RecordDenied(r, user, PermAccountManage, "")
				response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSelfService guards a route that acts only on the caller's own account.
// A session passes: it is the person acting on themselves. An API key must carry
// account.self.manage, so a narrow automation key cannot enrol MFA, revoke
// sessions or request elevations in its owner's name.
func RequireSelfService(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if p, ok := probed(next, ScopeAccount, false, PermAccountSelfManage); ok {
			return p
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
				return
			}
			if key := APIKeyFromContext(r.Context()); key != nil {
				keyPerms, err := svc.keyPermissions(r.Context(), key)
				if err != nil {
					response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
					return
				}
				if !hasPermission(keyPerms, PermAccountSelfManage) {
					RecordDenied(r, user, PermAccountSelfManage, "")
					response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
