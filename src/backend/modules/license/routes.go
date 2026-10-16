// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the licence routes.
//
// Two of them carry no permission on purpose. `/license/status` and
// `/license/notice-ack` are what the banner reads and writes, and the
// banner is shown to every user — gating them would leave a user staring
// at a notice they cannot explain. Neither payload contains a customer
// record, a billing address, or key material; everything that does sits
// behind license.view.
func Mount(app *router.AppDeps, service *Service) {
	h := NewHandler(app, service)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/license", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLicenseView)).Get("/", h.HandleGet)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLicenseView)).Get("/usage", h.HandleUsage, apispec.Paginated())

			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLicenseManage)).Post("/activate", h.HandleActivate, apispec.Request(ActivateInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLicenseManage)).Post("/deactivate", h.HandleDeactivate)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermLicenseManage)).Post("/refresh", h.HandleRefresh)

			// no permission: the licence banner is shown to every user, so
			// reading its state and dismissing it cannot be gated. Neither
			// payload carries a customer record, billing data or key material.
			r.Get("/status", h.HandleStatus)
			r.With(rbac.RequireSelfService(app.RBACService)).Post("/notice-ack", h.HandleNoticeAck, apispec.Request(NoticeAckInput{}))
		})
	})
}
