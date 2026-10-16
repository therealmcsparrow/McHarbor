// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const selfService = "self-service: acts only on the caller's own devices (account.self.manage territory)"

// Mount registers the push routes over svc and publishes svc as the "push"
// service, so modules that raise notifications can find it without an import cycle.
func Mount(app *router.AppDeps, svc *Service) {
	h := NewHandler(app, svc)
	app.RegisterService("push", svc)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/push", func(r *apispec.Router) {
			r.Get("/vapid-public-key", h.HandlePublicKey, apispec.Response(PublicKey{}))
			r.Get("/subscriptions", h.HandleList, apispec.Response([]Subscription{}))
			r.Post("/subscriptions", h.HandleSubscribe, apispec.Exempt(selfService), apispec.Request(SubscribeRequest{}), apispec.Response(Subscription{}))
			r.Delete("/subscriptions/{id}", h.HandleDelete, apispec.Exempt(selfService))
			r.Post("/test", h.HandleTest, apispec.Exempt(selfService), apispec.Response(TestResult{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/vapid/regenerate", h.HandleRegenerate)
		})
	})
}
