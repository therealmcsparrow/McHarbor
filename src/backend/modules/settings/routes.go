// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers settings module routes.
func Mount(app *router.AppDeps) {
	h := NewHandler(app)

	// The login page renders the custom wordmark before anyone has a session,
	// so the read side of branding cannot live behind auth. It exposes only
	// the two images an operator uploaded to be shown to every visitor.
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/branding", h.HandleGetBranding)
	})

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/settings", func(r *apispec.Router) {
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/", h.HandleList, apispec.Query("category", ""))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/", h.HandleBulkUpdate, apispec.Request(BulkUpdateInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/agent", h.HandleGetAgentSettings)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/agent", h.HandleUpdateAgentSettings, apispec.Request(AgentSettingsInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/scanners", h.HandleGetScannerSettings)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/scanners", h.HandleUpdateScannerSettings, apispec.Request(ScannerSettingsInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/retention", h.HandleGetRetentionSettings)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/retention", h.HandleUpdateRetentionSettings, apispec.Request(RetentionSettingsInput{}))
			// GET /toast is authenticated-only on purpose: every signed-in
			// user needs the app-wide default to render their own toasts, and
			// it exposes nothing sensitive. Writing it is settings.manage,
			// like every other app-wide default in this group.
			r.Get("/toast", h.HandleGetToastSettings)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/toast", h.HandleUpdateToastSettings, apispec.Request(ToastSettingsInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/toast/reset-overrides", h.HandleResetUserToastOverrides)
			// GET /appearance is authenticated-only for the same reason as
			// /toast: every signed-in user needs the app-wide theme, font and
			// base font size to paint their own UI, not just the operators
			// who can change them. Writing it is settings.manage.
			r.Get("/appearance", h.HandleGetAppearanceSettings)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/appearance", h.HandleUpdateAppearanceSettings, apispec.Request(AppearanceSettingsInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/appearance/reset-overrides", h.HandleResetUserAppearanceOverrides)
			// Reading branding is public (above); writing it is settings.manage,
			// like every other app-wide default in this group.
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/branding", h.HandleUpdateBranding, apispec.Request(BrandingSettingsInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/timezone", h.HandleGetTimezone)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/timezone", h.HandleUpdateTimezone, apispec.Request(TimezoneInput{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/tls", h.HandleGetTLS)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/tls", h.HandleUpdateTLS, apispec.Request(TLSUpdateRequest{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/tls/regenerate", h.HandleRegenerateTLS)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/tls/ca", h.HandleDownloadCA)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/backup-key/status", h.HandleGetBackupKeyStatus)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/backup-key/generate", h.HandleGenerateBackupKey)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Post("/backup-key/install", h.HandleInstallBackupKey, apispec.Request(BackupKeyInstallRequest{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/security-posture", h.HandleSecurityPosture, apispec.Response(SecurityPosture{}))
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsView)).Get("/{key}", h.HandleGetByKey)
			r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Put("/{key}", h.HandleSetByKey, apispec.Request(SettingInput{}))
		})
	})
}
