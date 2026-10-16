// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deviceauth

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
)

// Mount registers the device flow. The two terminal-facing routes sit in the
// rate-limited auth group because the terminal has no session yet; approving
// creates an API key, so it takes the same key as creating one by hand.
func Mount(app *router.AppDeps) {
	keys := apikeys.NewService(app.DB, app.RBACService)
	h := &Handler{app: app, svc: NewService(app.DB, keys), grants: keys}

	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Route("/device", func(r *apispec.Router) {
			r.Post("/code", h.HandleStart,
				apispec.Exempt("device sign-in bootstrap: the terminal has no principal yet"),
				apispec.Request(StartInput{}), apispec.Response(StartResult{}))
			r.Post("/token", h.HandleToken,
				apispec.Exempt("device sign-in poll: the secret device code is the credential"),
				apispec.Request(TokenInput{}), apispec.Response(TokenResult{}))
		})
	})

	manage := rbac.RequirePermission(app.RBACService, rbac.PermAPIKeysManage)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/device/requests", func(r *apispec.Router) {
			r.With(manage).Get("/{userCode}", h.HandleLookup, apispec.Response(Request{}))
			r.With(manage).Post("/{userCode}/approve", h.HandleApprove, apispec.Request(ApproveInput{}))
			r.With(manage).Post("/{userCode}/deny", h.HandleDeny)
		})
	})
}
