// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// mfaEnrollmentOwed reads MFAEnrollmentOwed at request time rather than at mount time,
// so the gate works whichever order the mfa module and router.New run in.
func (a *AppDeps) mfaEnrollmentOwed(ctx context.Context, user *auth.User) (bool, error) {
	if a == nil || a.MFAEnrollmentOwed == nil {
		return false, nil
	}
	return a.MFAEnrollmentOwed(ctx, user)
}
