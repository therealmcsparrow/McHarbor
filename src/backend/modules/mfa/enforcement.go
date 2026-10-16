// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// enrollmentLookup is the one Service method the enforcer needs, narrowed so tests can
// stand in for the user_mfa table.
type enrollmentLookup interface {
	IsEnabled(ctx context.Context, userID string) (bool, error)
}

// Enforcer applies security.mfa_required to a user: whether the policy covers them, and
// whether they still owe enrolment. It is what the session gate and the login flag call.
type Enforcer struct {
	enrollments enrollmentLookup
	policy      *rbac.MFAPolicy
}

// NewEnforcer returns an Enforcer reading enrolment from svc and the requirement from policy.
func NewEnforcer(svc *Service, policy *rbac.MFAPolicy) *Enforcer {
	return &Enforcer{enrollments: svc, policy: policy}
}

// Required reports whether the policy covers user, enrolled or not. It is what refuses
// a covered user switching their own factor off.
func (e *Enforcer) Required(ctx context.Context, user *auth.User) (bool, error) {
	if e == nil || user == nil {
		return false, nil
	}
	return e.policy.AppliesTo(ctx, user.ID)
}

// EnrollmentOwed reports whether user is covered by the policy and has not enrolled.
// API-key requests are never held: a key is its own credential, not a password session.
func (e *Enforcer) EnrollmentOwed(ctx context.Context, user *auth.User) (bool, error) {
	if e == nil || user == nil || rbac.APIKeyFromContext(ctx) != nil {
		return false, nil
	}
	required, err := e.Required(ctx, user)
	if err != nil || !required {
		return false, err
	}
	enabled, err := e.enrollments.IsEnabled(ctx, user.ID)
	if err != nil {
		return false, err
	}
	return !enabled, nil
}
