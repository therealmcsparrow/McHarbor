// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers identity provider module routes, picking up
// whatever plugin runtime is on app.PluginAdapters at call time.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, app.PluginAdapters)
}

// MountWithRuntime registers identity provider module routes with the
// plugin runtime injected explicitly. Every provider type is
// plugin-owned after the Phase 3 port, so this module must be mounted
// *after* bootPluginRuntime — a nil runtime makes Create reject every
// provider_type with ErrIdentityInvalidType and breaks the OIDC/SAML
// login flow. Mirrors communications.MountWithRuntime and
// containerbackups.MountWithRuntime.
func MountWithRuntime(app *router.AppDeps, adapters any) {
	h := NewHandlerWithRuntime(app, adapters)

	// Public routes (no auth middleware) — for OIDC login flow
	app.RegisterAuthRoutes(func(r *apispec.Router) {
		r.Get("/identity-providers/enabled", h.HandleEnabledProviders)
		r.Get("/identity-providers/{id}/authorize", h.HandleAuthorize)
		r.Get("/identity-providers/callback", h.HandleCallback, apispec.Query("code", ""), apispec.Query("error", ""), apispec.Query("error_description", ""), apispec.Query("state", ""))
		r.Get("/identity-providers/{id}/metadata", h.HandleSAMLMetadata)
		// The ACS endpoint is an unauthenticated callback: the IdP POSTs the
		// SAML assertion here before any McHarbor session exists. Its trust
		// comes from assertion signature validation in HandleSAMLACS, not
		// from a permission.
		r.Get("/identity-providers/{id}/acs", h.HandleSAMLACS)
		r.Post("/identity-providers/{id}/acs", h.HandleSAMLACS, apispec.Exempt("IdP callback: trust comes from the SAML assertion signature"))
	})

	// Protected routes — admin CRUD
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/identity-providers", func(r *apispec.Router) {
			// Identity providers are host configuration: settings.manage keeps
			// unlocking everything, and the identity_providers.* keys unlock
			// their own tier. Reads include client ids, so view is not
			// widened to settings.view.
			view := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsManage, rbac.PermIdentityProvidersView)
			manage := rbac.RequireAnyGlobalPermission(app.RBACService, rbac.PermSettingsManage, rbac.PermIdentityProvidersManage)
			r.With(view).Get("/", h.HandleList)
			r.With(view).Get("/endpoint-urls", h.HandleEndpointURLs, apispec.Query("providerId", ""), apispec.Response(ProviderEndpointURLs{}))
			r.With(manage).Post("/metadata-import", h.HandleMetadataImport, apispec.Request(MetadataImportInput{}), apispec.Response(MetadataImportResult{}))
			r.With(manage).Post("/mapping-preview", h.HandleMappingPreview, apispec.Request(MappingPreviewInput{}), apispec.Response(MappingPreview{}))
			r.With(view).Get("/{id}", h.HandleGet)
			r.With(manage).Post("/", h.HandleCreate, apispec.Request(CreateProviderInput{}))
			r.With(manage).Put("/{id}", h.HandleUpdate, apispec.Request(UpdateProviderInput{}))
			r.With(manage).Delete("/{id}", h.HandleDelete)
			r.With(manage).Post("/{id}/test", h.HandleTest)
			r.With(manage).Get("/{id}/groups", h.HandleFetchGroups)
		})
	})
}
