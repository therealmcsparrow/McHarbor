// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers AI assistant routes under /ai-assistant.
func Mount(app *router.AppDeps) {
	mountRoutes(app, NewHandler(app))
}

// MountWithProviders registers the module with the AI-provider connection
// resolver injected (see providers_bridge.go). Used by main.go.
func MountWithProviders(app *router.AppDeps, providers ProviderResolver) {
	mountRoutes(app, NewHandlerWithProviders(app, providers))
}

func mountRoutes(app *router.AppDeps, h *Handler) {
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/ai-assistant", func(r *apispec.Router) {
			// Hard kill switch (§9.6): every route that depends on
			// the LLM is gated behind RequireAIEnabled so that
			// disabling the assistant at the settings level takes
			// the routes offline immediately with 503 + ErrAIDisabled.
			// There is no admin surface left here: settings, the
			// provider catalog and the OAuth device flow all moved to
			// /ai-providers, which owns its own connections and tokens.
			killSwitch := RequireAIEnabled(h.svc)

			// Reading transcripts needs either the view key or the use key;
			// a viewer-only role can audit conversations without chatting.
			r.With(killSwitch, rbac.RequireAnyPermission(app.RBACService, rbac.PermAIAssistantView, rbac.PermAIAssistantUse)).Get("/actions", h.HandleListActions, apispec.Query("limit", ""))
			// Consenting to and executing a proposed action mutates real
			// infrastructure, so it takes the stronger act key.
			r.With(killSwitch, rbac.RequirePermission(app.RBACService, rbac.PermAIAssistantAct)).Post("/actions/{id}/consent", h.HandleConsentAction, apispec.Request(ActionConsentRequest{}))
			r.With(killSwitch, rbac.RequirePermission(app.RBACService, rbac.PermAIAssistantAct)).Post("/actions/{id}/execute", h.HandleExecuteAction)
			r.With(killSwitch, rbac.RequirePermission(app.RBACService, rbac.PermAIAssistantUse)).Post("/chat", h.HandleChat, apispec.Request(ChatRequest{}))
		})
	})
}

func parseIntDefault(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
