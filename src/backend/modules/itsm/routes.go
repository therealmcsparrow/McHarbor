// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the ITSM routes over svc.
func Mount(app *router.AppDeps, svc *Service) {
	h := &Handler{app: app, svc: svc}
	alertsView := rbac.RequirePermission(app.RBACService, rbac.PermAlertsView)
	alertsManage := rbac.RequirePermission(app.RBACService, rbac.PermAlertsManage)
	commsView := rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsView)
	commsManage := rbac.RequirePermission(app.RBACService, rbac.PermCommunicationsManage)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/itsm", func(r *apispec.Router) {
			r.With(alertsView).Get("/links", h.HandleLinks, apispec.Paginated(), apispec.Scoped(),
				apispec.Query("state", ""), apispec.Query("channel", ""), apispec.Response([]Link{}))
			r.With(alertsManage).Post("/links/{id}/resync", h.HandleResync, apispec.Response(Link{}))
			r.With(alertsManage).Post("/links/{id}/retry", h.HandleRetry)
			r.With(commsView).Get("/channels", h.HandleChannels, apispec.Response([]ChannelInfo{}))
			r.With(commsManage).Post("/channels/{id}/webhook-token", h.HandleRotateToken, apispec.Response(RotateResult{}))
			r.With(commsManage).Post("/channels/{id}/webhook-secret", h.HandleSetSecret, apispec.Request(SecretInput{}))
		})
	})
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.With(middleware.RateLimit(30, app.ClientKey)).Post("/incident-webhooks/{token}", h.HandleInbound,
			apispec.Exempt("public ITSM callback: authenticated by the per-channel token and the plugin's signature check"))
	})
}
