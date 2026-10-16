// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"os"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/clusterinfo"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount builds the support service from the app and registers its routes. logs may be nil,
// in which case bundles say the process keeps no log history.
func Mount(app *router.AppDeps, logs *LogRing) *Service {
	svc := NewService(app.ContextOrBackground(), depsFromApp(app, logs))
	mountRoutes(app, NewHandler(svc, app.AuditLog))
	return svc
}

func mountRoutes(app *router.AppDeps, h *Handler) {
	view := rbac.RequirePermission(app.RBACService, rbac.PermSupportView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermSupportManage)
	updatesView := rbac.RequirePermission(app.RBACService, rbac.PermUpdatesView)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/support", func(r *apispec.Router) {
			r.With(view).Get("/diagnostics", h.handleDiagnostics, apispec.Query("check", "Comma-separated check ids; empty runs every check"), apispec.Response(DiagnosticsReport{}))
			r.With(view).Get("/encryption-key", h.handleKeySource, apispec.Response(KeySource{}))
			r.With(view).Get("/inventory", h.handleInventory, apispec.Response(Inventory{}))
			r.With(updatesView).Get("/pre-upgrade", h.handlePreUpgrade, apispec.Query("target", "Version to upgrade to; empty checks the running version"), apispec.Response(PreUpgradeReport{}))
			r.With(view).Get("/bundles", h.handleListBundles, apispec.Response(BundleList{}))
			r.With(view).Get("/bundles/preview", h.handlePreview, apispec.Query("categories", "Comma-separated bundle categories"), apispec.Response(BundlePreview{}))
			r.With(manage).Post("/bundles", h.handleCreate, apispec.Request(BundleRequest{}), apispec.Response(Bundle{}))
			r.With(view).Get("/bundles/{id}", h.handleGet, apispec.Response(Bundle{}))
			r.With(manage).Get("/bundles/{id}/download", h.handleDownload, apispec.Binary())
			r.With(manage).Delete("/bundles/{id}", h.handleDelete)
		})
	})
}

func depsFromApp(app *router.AppDeps, logs *LogRing) Deps {
	cfg := app.Config
	driver, _ := cfg.DBConfig()
	if d := coredb.DriverOf(); d != "" {
		driver = string(d)
	}
	d := Deps{
		DB: app.DB, DataDir: cfg.DataDir, DatabasePath: cfg.DatabasePath, DatabaseDriver: driver,
		EncryptionKeySet: cfg.EncryptionKey != "", SecretSet: cfg.Secret != "", DBDSNSet: cfg.DBDSN != "", TLSEnabled: cfg.TLSEnabled, CatalogURL: cfg.AppStoreCatalogURL,
		Logs: logs, Logger: app.Logger,
		KnownSecrets: []string{cfg.EncryptionKey, cfg.Secret, cfg.DBDSN},
	}
	if app.DockerPool != nil {
		d.Docker = app.DockerPool
	}
	if app.AgentPool != nil {
		d.Agents = app.AgentPool
	}
	d.BackupKeyPresent = func() bool { return app.BackupCrypto != nil }
	if app.BackupCrypto != nil {
		d.KnownSecrets = append(d.KnownSecrets, app.BackupCrypto.KeyMaterialBase64())
	}
	for _, name := range []string{".encryption_key", ".secret"} {
		if raw, err := os.ReadFile(filepath.Join(cfg.DataDir, name)); err == nil {
			d.KnownSecrets = append(d.KnownSecrets, string(raw))
		}
	}
	d.Plugins = func() ([]PluginState, bool) {
		rt, ok := app.PluginAdapters.(*pluginruntime.Runtime)
		if !ok || rt == nil {
			return nil, false
		}
		out := []PluginState{}
		for _, l := range rt.Loaded() {
			out = append(out, PluginState{Slug: l.Slug, Version: l.Version, Loaded: l.Loaded, Error: l.Err})
		}
		sortPlugins(out)
		return out, true
	}
	d.Cluster = func(ctx context.Context) (ClusterView, bool) {
		svc, ok := app.LookupService("cluster")
		if !ok {
			return ClusterView{}, false
		}
		p, ok := svc.(clusterinfo.Provider)
		if !ok {
			return ClusterView{}, false
		}
		cs := p.Info()
		ready, checks := cs.Ready(ctx)
		view := ClusterView{Ready: ready, Checks: map[string]bool{}}
		for _, c := range checks {
			view.Checks[c.Name] = c.OK
		}
		if reg := cs.Membership(); reg != nil {
			view.Extra = map[string]string{"nodeId": reg.NodeID(), "role": reg.Role(), "state": reg.State()}
			if nodes, err := reg.List(ctx); err == nil {
				for _, n := range nodes {
					view.Nodes = append(view.Nodes, map[string]any{
						"id": n.ID, "hostname": n.Hostname, "version": n.Version, "build": n.Build,
						"haRole": n.HARole, "state": n.State, "alive": n.Alive, "agents": n.Agents,
						"heartbeatAt": n.HeartbeatAt,
					})
				}
			}
		}
		return view, true
	}
	d.TLSCertPath = func() string {
		svc, ok := app.LookupService("tls_reloader")
		if !ok {
			return ""
		}
		if rl, ok := svc.(interface{ CertPath() string }); ok {
			return rl.CertPath()
		}
		return ""
	}
	return d
}
