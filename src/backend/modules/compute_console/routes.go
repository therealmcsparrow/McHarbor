// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the console gateway.
//
// Two routes with deliberately different protection:
//
//   - POST /api/compute/console/ticket is an ordinary authenticated request.
//     It carries the console check, made in the handler with the key the
//     plugin declaring that guest kind names, because one route serves them all.
//   - GET /api/compute/console/{ticket} is the WebSocket upgrade and is
//     mounted PUBLIC. That is not an oversight: a browser cannot attach an
//     Authorization header to an upgrade, and the ticket is the credential —
//     single-use, 30 seconds, bound to one user, one guest and one console
//     kind. Putting a session token in the URL instead would leak it into
//     every proxy log between the browser and here.
func Mount(app *router.AppDeps, bridge *agentrpc.Bridge) {
	var gateway consoleGateway
	if bridge != nil {
		gateway = bridge
	}
	h := NewHandler(app, gateway)

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/compute/console", func(r *apispec.Router) {
			// Exempt: gated in the handler, not on the route. One route
			// serves every declared guest kind, so the console check is made
			// once the guest kind, and with it the declaring plugin's key, is known.
			r.Post("/ticket", h.HandleIssueTicket, apispec.Exempt("gated in handler: the console key the guest kind's plugin declares, once the guest kind is known"), apispec.Request(ticketRequest{}))
		})
	})

	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/compute/console/{ticket}", h.HandleWS, apispec.WebSocket())
	})
}
