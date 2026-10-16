// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers vulnerability scan module routes with no plugin
// runtime attached.
//
// A host mounted this way lists no scanner adapters and cannot start a
// scan, which is the correct behaviour for a build with the plugin
// runtime disabled — not an error. Production wiring uses
// MountWithRuntime.
func Mount(app *router.AppDeps) {
	MountWithRuntime(app, nil)
}

// MountWithRuntime registers the module against the loaded plugin
// runtime so the `scanner-` family's engines are reachable.
//
// The /scans/adapters and /scans/configs subtrees are host-owned even
// though every engine behind them ships as a plugin: the scan ledger,
// retention and the history page are the host's, and an operator
// configuring a scanner is doing administration, not using a plugin's
// own UI.
//
// This replaced a background goroutine that polled the `scanners`
// settings category with exponential backoff, rebuilding a built-in
// registry from a Clair URL. Nothing polls now — an adapter is
// registered when its plugin loads, and a config row is read on the
// request that needs it.
func MountWithRuntime(app *router.AppDeps, runtime any) {
	configs := NewConfigService(app.DB, app.Encryption, runtime)
	// Runs here because the plugin runtime is loaded by now, so an installed engine's row is kept.
	if removed, err := configs.pruneUnusedSeedRows(context.Background()); err != nil {
		app.Logger.Warn("scans: removing unused seeded scanners failed", "error", err)
	} else if removed > 0 {
		app.Logger.Info("scans: removed scanners seeded for engines that were never installed", "count", removed)
	}
	svc := NewService(app.DB, configs, app.Logger)
	h := &Handler{app: app, service: svc, configs: configs}

	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/scans", func(r *apispec.Router) {
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/", h.HandleList, apispec.Scoped(), apispec.Paginated(), apispec.Query("search", ""), apispec.Query("status", ""))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansManage)).Post("/", h.HandleStartScan, apispec.Scoped(), apispec.Request(StartScanInput{}))
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/summary", h.HandleSummary, apispec.Scoped())
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/scanners", h.HandleAvailableScanners)
			r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/by-image", h.HandleScanByImage, apispec.Scoped(), apispec.Paginated(), apispec.Query("image", ""))

			// Scanner administration.
			//
			// Reading the adapter and instance lists needs both
			// scans.view and settings.manage: the response names every
			// configured engine and its non-secret settings, which is
			// administrative configuration rather than scan data.
			// Writes and tests take settings.manage, matching the gate
			// the replaced /settings/scanners form carried.
			//
			// Both static segments are declared before the /{id}
			// subtree so chi routes /adapters and /configs rather than
			// treating either word as a scan id.
			r.With(
				rbac.RequirePermission(app.RBACService, rbac.PermScansView),
				rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
			).Get("/adapters", h.HandleScannerAdapters)

			r.Route("/configs", func(r *apispec.Router) {
				r.With(
					rbac.RequirePermission(app.RBACService, rbac.PermScansView),
					rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage),
				).Get("/", h.HandleScannerConfigs)
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
					Post("/", h.HandleCreateScannerConfig, apispec.Request(saveScannerRequest{}))
				r.Route("/{id}", func(r *apispec.Router) {
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Put("/", h.HandleSaveScannerConfig, apispec.Request(saveScannerRequest{}))
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Post("/test", h.HandleTestScannerConfig)
					r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).
						Delete("/", h.HandleDeleteScannerConfig)
				})
			})

			r.Group(func(r *apispec.Router) {
				r.Use(rbac.PinEnvToResource(h.scanEnvironment))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/{id}", h.HandleGetScan)
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansManage)).Delete("/{id}", h.HandleDelete)
				r.With(rbac.RequireGlobalPermission(app.RBACService, rbac.PermSettingsManage)).Delete("/", h.HandlePurge, apispec.Query("vacuum", ""))
				r.With(rbac.RequirePermission(app.RBACService, rbac.PermScansView)).Get("/{id}/vulnerabilities", h.HandleGetVulnerabilities, apispec.Paginated())
			})
		})
	})
}
