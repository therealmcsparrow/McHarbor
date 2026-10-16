// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

// AuthorizeWithAttributes is the ABAC engine entry point. It is called
// from Authorize() when both a policy service and an attribute bundle
// are configured and the v3 rollout flag is on.
//
// Flow:
//  1. If any ABAC component is nil or v3 is off → fall through to
//     legacy HasPermission.
//  2. Lazy-hydrate Attributes (user, env, resource) — fail-soft on
//     provider errors so a misbehaving Docker pool doesn't lock admins
//     out.
//  3. Load active policies for the candidate permission.
//  4. For each policy (priority ascending):
//     a. Skip if resource_type filter does not match.
//     b. Evaluate the expression via the JS executor.
//     c. On match with effect=deny → RecordABACDenied + return
//     (false, nil).
//     d. On match with effect=allow → return (true, nil).
//     e. On expression error or non-match → skip.
//  5. No policy matched → fall through to legacy HasPermission.
func (s *Service) AuthorizeWithAttributes(ctx context.Context, req AuthzRequest, attrs *Attributes) (bool, error) {
	return s.authorizeWithAttributes(ctx, req, attrs, true)
}

// authorizeWithAttributes is AuthorizeWithAttributes with the ABAC-denied audit under
// record, so Explain reaches the same answer without writing audit rows.
func (s *Service) authorizeWithAttributes(ctx context.Context, req AuthzRequest, attrs *Attributes, record bool) (bool, error) {
	// Short-circuit when ABAC isn't wired or the v3 gate is off.
	if s.policies == nil || s.attributes == nil || s.abacExec == nil {
		return s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
	}
	if s.rolloutV3 != nil && !s.rolloutV3.isEnabled(ctx) {
		return s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
	}
	if req.UserID == "" {
		return false, nil
	}

	// Lazy-hydrate attributes. Failures are non-fatal — we evaluate
	// against whatever we have.
	if attrs == nil {
		attrs = &Attributes{}
	}
	if attrs.User == nil {
		if u, err := FetchUserAttributes(ctx, s.db, req.UserID); err == nil {
			attrs.User = u
		}
	}
	if attrs.Env == nil && req.EnvironmentID != "" {
		attrs.Env, attrs.envUnresolved = s.envAttributes(ctx, req.EnvironmentID)
	}
	if attrs.Resource == nil && req.ResourceType != "" && req.ResourceID != "" {
		if r := s.attributes.Resolve(ctx, req); r != nil {
			attrs.Resource = r
		}
	}

	policies, err := s.policies.LoadActiveForPermission(ctx, string(req.Permission))
	if err != nil {
		// DB read failed — never let ABAC block traffic; fall through
		// to the role-based check.
		return s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
	}

	// A deny records itself inside evalPolicies when record is set; any other
	// effect allows, as the "allow" branch always did.
	if p := s.evalPolicies(ctx, policies, req, attrs, record); p != nil {
		return p.Effect != "deny", nil
	}

	// No policy matched — fall through to the role-based check.
	return s.HasPermission(ctx, req.UserID, req.EnvironmentID, req.Permission)
}

// PolicyDecision evaluates only the ABAC policies for req. matched is false when no
// enabled policy applies, and the caller then keeps its role-based answer unchanged.
func (s *Service) PolicyDecision(ctx context.Context, req AuthzRequest) (matched, allow bool) {
	return s.policyDecision(ctx, req, true)
}

// policyDecision is PolicyDecision with the audit side effects under record, so
// Explain can replay the route gate's policy answer without writing denies.
func (s *Service) policyDecision(ctx context.Context, req AuthzRequest, record bool) (matched, allow bool) {
	if s == nil || s.policies == nil || s.attributes == nil || s.abacExec == nil || !s.abacEnabled(ctx) || req.UserID == "" {
		return false, false
	}
	policies, err := s.policies.LoadActiveForPermission(ctx, string(req.Permission))
	if err != nil || len(policies) == 0 {
		return false, false
	}
	attrs := s.decisionAttributes(ctx, req)
	p := s.evalPolicies(ctx, policies, req, attrs, record)
	if p == nil {
		return false, false
	}
	return true, p.Effect != "deny"
}

// decideRoute is the route gate: a matching policy decides, otherwise the role-based
// answer stands exactly as it did before policies were consulted here.
func (s *Service) decideRoute(r *http.Request, userID, envID string, perm Permission) (bool, error) {
	if s.abacEnabled(r.Context()) {
		resType, resID := ResourceFromRoute(r)
		matched, allow := s.PolicyDecision(r.Context(), AuthzRequest{
			UserID: userID, Permission: perm, EnvironmentID: envID,
			ResourceType: resType, ResourceID: resID, Request: RequestFromHTTP(r),
			APIKey: APIKeyFromContext(r.Context()),
		})
		if matched {
			return allow, nil
		}
	}
	return s.HasPermission(r.Context(), userID, envID, perm)
}

// RequestFromHTTP captures the request facts a policy may read.
func RequestFromHTTP(r *http.Request) *RequestAttributes {
	method := r.Method
	return &RequestAttributes{
		Method:   method,
		Path:     r.URL.Path,
		IP:       httpx.ClientIP(r),
		APIKey:   APIKeyFromContext(r.Context()) != nil,
		ReadOnly: method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions,
	}
}
