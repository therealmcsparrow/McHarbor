// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExplainRequest names the subject, the permission and the scope a route gate
// resolves it in, which is everything the simulator needs to replay a decision.
type ExplainRequest struct {
	UserID        string
	EnvironmentID string
	StackName     string
	Permission    Permission
	// Scope is how the route resolves the key; empty means ScopeEnvironment.
	Scope PermScope
	// Method and Path describe the route, for policies that read request.*.
	Method string
	Path   string
}

// ExplainReason is one source that contributed to a decision, so an admin can
// see which role, elevation or policy to change instead of guessing.
type ExplainReason struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Effect string `json:"effect,omitempty"`
}

// Reason kinds reported by Explain.
const (
	ReasonRole          = "role"
	ReasonWildcardRole  = "wildcard_role"
	ReasonElevation     = "elevation"
	ReasonBreakGlass    = "break_glass"
	ReasonOverrideGrant = "override_grant"
	ReasonOverrideDeny  = "override_deny"
	ReasonABACAllow     = "abac_allow"
	ReasonABACDeny      = "abac_deny"
	ReasonNoGrant       = "no_grant"
)

// Explanation is the simulator's answer. Allowed is the decision the route gates
// (RequirePermission and friends) take; PolicyAllowed is Authorize's, which also
// applies per-user overrides, owner rules and ABAC policies when those are on.
type Explanation struct {
	Permission    string          `json:"permission"`
	Scope         string          `json:"scope"`
	Allowed       bool            `json:"allowed"`
	PolicyAllowed bool            `json:"policyAllowed"`
	ABACEnabled   bool            `json:"abacEnabled"`
	Reasons       []ExplainReason `json:"reasons"`
}

// Explain computes a decision with the same functions the live gates call and
// then lists the sources behind it. It never writes audit rows, and it drops any
// API key in ctx so the caller's own key cannot narrow someone else's answer.
func (s *Service) Explain(ctx context.Context, req ExplainRequest) (*Explanation, error) {
	if s == nil || req.UserID == "" || req.Permission == "" {
		return nil, fmt.Errorf("explain: user and permission are required")
	}
	ctx = WithAPIKey(ctx, nil)
	scope := req.Scope
	if scope == "" {
		scope = ScopeEnvironment
	}
	out := &Explanation{Permission: string(req.Permission), Scope: string(scope), ABACEnabled: s.abacEnabled(ctx)}

	var err error
	switch scope {
	case ScopeGlobal:
		out.Allowed, err = s.HasPermission(ctx, req.UserID, "", req.Permission)
	case ScopeStack:
		out.Allowed, err = s.HasStackPermission(ctx, req.UserID, req.EnvironmentID, req.StackName, req.Permission)
	default:
		// The environment-scoped route gate lets a matching policy decide
		// first, so the simulator asks the same PolicyDecision it does, minus the audit.
		matched, allow := s.policyDecision(ctx, AuthzRequest{UserID: req.UserID, Permission: req.Permission,
			EnvironmentID: req.EnvironmentID, Request: &RequestAttributes{Method: req.Method, Path: req.Path,
				ReadOnly: req.Method == "GET" || req.Method == "HEAD" || req.Method == "OPTIONS"}}, false)
		if matched {
			out.Allowed = allow
		} else {
			out.Allowed, err = s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
		}
	}
	if err != nil {
		return nil, err
	}
	authz := AuthzRequest{UserID: req.UserID, Permission: req.Permission, EnvironmentID: req.EnvironmentID, StackName: req.StackName}
	if out.PolicyAllowed, err = s.authorize(ctx, authz, false); err != nil {
		return nil, err
	}

	reasons, err := s.explainSources(ctx, req, authz)
	if err != nil {
		return nil, err
	}
	if len(reasons) == 0 {
		reasons = []ExplainReason{{Kind: ReasonNoGrant}}
	}
	out.Reasons = reasons
	return out, nil
}

// explainSources lists every source that grants or denies the permission.
func (s *Service) explainSources(ctx context.Context, req ExplainRequest, authz AuthzRequest) ([]ExplainReason, error) {
	var reasons []ExplainReason
	emergency, err := s.breakGlassActive(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if emergency {
		reasons = append(reasons, ExplainReason{Kind: ReasonBreakGlass, Effect: "allow"})
	}
	roles, err := s.grantingRoles(ctx, req)
	if err != nil {
		return nil, err
	}
	reasons = append(reasons, roles...)

	elevated, err := s.activeElevatedPermissions(ctx, req.UserID, req.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if hasPermission(elevated, req.Permission) {
		reasons = append(reasons, ExplainReason{Kind: ReasonElevation, Effect: "allow"})
	}

	if denied, err := s.matchOverride(ctx, authz, "deny"); err != nil {
		return nil, err
	} else if denied {
		reasons = append(reasons, ExplainReason{Kind: ReasonOverrideDeny, Effect: "deny"})
	}
	if granted, err := s.matchOverride(ctx, authz, "grant"); err != nil {
		return nil, err
	} else if granted {
		reasons = append(reasons, ExplainReason{Kind: ReasonOverrideGrant, Effect: "allow"})
	}

	if s.policies != nil && s.attributes != nil && s.abacExec != nil && s.abacEnabled(ctx) {
		policies, err := s.policies.LoadActiveForPermission(ctx, string(req.Permission))
		if err != nil {
			return nil, err
		}
		if p := s.explainMatchingPolicy(ctx, policies, authz); p != nil {
			kind := ReasonABACAllow
			if p.Effect == "deny" {
				kind = ReasonABACDeny
			}
			reasons = append(reasons, ExplainReason{Kind: kind, ID: p.ID, Name: p.Name, Effect: p.Effect})
		}
	}
	return reasons, nil
}

// grantingRoles returns the roles that carry the permission for the subject,
// resolved from the same assignment queries the permission cache is built from.
func (s *Service) grantingRoles(ctx context.Context, req ExplainRequest) ([]ExplainReason, error) {
	roleIDs, err := s.collectRoleIDs(ctx, req.UserID, req.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if req.Scope == ScopeStack && req.StackName != "" {
		scoped, err := s.collectScopedRoleIDs(ctx, req.UserID, req.EnvironmentID)
		if err != nil {
			return nil, err
		}
		roleIDs = appendUniqueRoleIDs(roleIDs, scoped[req.StackName])
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(roleIDs))
	marks := make([]string, len(roleIDs))
	for i, id := range roleIDs {
		args[i] = id
		marks[i] = "?"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, permissions FROM roles WHERE id IN (`+strings.Join(marks, ",")+`) ORDER BY name LIMIT 1000`, args...) // SQL identifier: marks holds only "?" placeholders
	if err != nil {
		return nil, fmt.Errorf("reading granting roles: %w", err)
	}
	defer closeRows(rows)
	var out []ExplainReason
	for rows.Next() {
		var id, name, raw string
		if err := rows.Scan(&id, &name, &raw); err != nil {
			return nil, fmt.Errorf("scanning granting role: %w", err)
		}
		var perms []string
		if json.Unmarshal([]byte(raw), &perms) != nil {
			continue
		}
		for _, p := range perms {
			if Permission(p) == PermWildcard {
				out = append(out, ExplainReason{Kind: ReasonWildcardRole, ID: id, Name: name, Effect: "allow"})
				break
			}
			if Permission(p) == req.Permission {
				out = append(out, ExplainReason{Kind: ReasonRole, ID: id, Name: name, Effect: "allow"})
				break
			}
		}
	}
	return out, rows.Err()
}

// explainMatchingPolicy names the policy that would decide authz, through the same
// evalPolicies walk the live gates use, with record off so a preview writes no
// ABAC-denied audit rows. It hydrates user and environment but no resource, and an
// environment whose lookup fails still exposes its id to policies.
func (s *Service) explainMatchingPolicy(ctx context.Context, policies []ABACPolicy, authz AuthzRequest) *ABACPolicy {
	attrs := &Attributes{}
	if u, err := FetchUserAttributes(ctx, s.db, authz.UserID); err == nil {
		attrs.User = u
	}
	if authz.EnvironmentID != "" {
		attrs.Env, attrs.envUnresolved = s.envAttributes(ctx, authz.EnvironmentID)
	}
	return s.evalPolicies(ctx, policies, authz, attrs, false)
}
