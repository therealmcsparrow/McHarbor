// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the per-user MFA routes. Every route is protected:
// enrolling, confirming, and disabling all act on the *caller's own*
// account, taken from the session, never from a path parameter — so
// there is no permission check to make and no way to touch another
// user's factor.
//
// The second login step lives in the auth module instead, because it
// runs before a session exists.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)
	app.MFAEnrollmentOwed = h.enforcer.EnrollmentOwed

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/auth/mfa", func(r *apispec.Router) {
			r.Get("/status", h.HandleStatus)
			// Exempt: self-service MFA on the caller's own account
			// (account.self.manage); no user id is taken from the request.
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/enroll/begin", h.HandleBeginEnrollment)
			// Exempt: self-service MFA enrolment confirmation, own account only.
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/enroll/confirm", h.HandleConfirmEnrollment, apispec.Request(confirmRequest{}))
			// Exempt: self-service MFA disable, own account only.
			r.With(rbac.RequireSelfService(app.RBACService)).Delete("/", h.HandleDisable)
		})
	})
}
