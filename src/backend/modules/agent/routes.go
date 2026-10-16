// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// AgentWSServiceName is the service name the agent WebSocket handler is
// registered under, for callers that must serve it off the router.
const AgentWSServiceName = "agent.ws"

// Mount registers agent module routes without the offline replay bridges;
// buffered events and metrics from an agent are then acknowledged and
// dropped. main.go uses MountWithOfflineBridges instead.
func Mount(app *router.AppDeps) {
	mountRoutes(app, NewHandler(app))
}

// MountWithOfflineBridges registers the module and injects the activity and
// metrics collaborators the offline replay path delegates to (see bridges.go).
func MountWithOfflineBridges(app *router.AppDeps, classifier OfflineEventClassifier, ingester OfflineMetricsIngester) {
	h := NewHandler(app)
	h.service.SetOfflineBridges(classifier, ingester)
	mountRoutes(app, h)
}

func mountRoutes(app *router.AppDeps, h *Handler) {
	// Demo mode serves this handler on an in-memory listener for its simulated
	// agents, so they take the real handshake without the network rate limit.
	app.RegisterService(AgentWSServiceName, http.HandlerFunc(h.HandleAgentWS))

	// Auth routes — no session middleware
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Get("/agent/ws", h.HandleAgentWS, apispec.WebSocket(), apispec.Query("token", ""))
		r.Get("/agent/install/{token}", h.HandleInstallScript, apispec.Query("insecure", ""), apispec.Query("method", ""))
		r.Get("/agent/binary/{os}/{arch}", h.HandleAgentBinary)
		// The install script probes with `curl -fsI` before downloading; without
		// HEAD chi answers 405 and every install falls back to the public release.
		r.Head("/agent/binary/{os}/{arch}", h.HandleAgentBinary)
		// Agent-token authenticated inside the handler: a container agent loads
		// its replacement image from here instead of from a registry.
		r.Get("/agent/image/{os}/{arch}", h.HandleAgentImage, apispec.Binary())
	})

	// Admin endpoints — session-protected
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/agents", func(r *apispec.Router) {
			r.Get("/", h.HandleListAgents)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/direct-transfer-test", h.HandleDirectTransferTest, apispec.Request(DirectTransferTestRequest{}))
			// Before "/{envId}", or chi reads "probe" as an environment id.
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).
				Post("/probe", h.HandleProbe, apispec.Request(DeployRequest{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).
				Post("/install-prereqs", h.HandleInstallPrereqs, apispec.Request(DeployRequest{}))

			r.Route("/{envId}", func(r *apispec.Router) {
				r.Use(rbac.PinEnvToPath("envId"))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/status", h.HandleStatus)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/logs", h.HandleAgentLogs, apispec.Paginated())
				// No new permission key: the health series exposes nothing the
				// agent log and status endpoints beside it do not already show,
				// and it is read-only.
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsView)).Get("/health/history", h.HandleHealthHistory, apispec.Query("limit", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/regenerate-token", h.HandleRegenerateToken)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/deploy", h.HandleDeploy, apispec.Request(DeployRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/redeploy", h.HandleRedeploy, apispec.Request(DeployRequest{}))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Post("/install-token", h.HandleCreateInstallToken)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Get("/access", h.HandleAccess)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Get("/access/secrets", h.HandleAccessSecrets)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Put("/access", h.HandleSaveAccess)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermEnvironmentsManage)).Delete("/access", h.HandleForgetAccess)
			})
		})
	})
}
