// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/clusterinfo"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Mount registers the read-only posture endpoint behind compliance.view. The export
// flag only changes the redaction default and records an audit entry.
func Mount(app *router.AppDeps) {
	svc := NewService(app.DB, Sources{
		Config:           app.Config,
		BackupKeySet:     func() bool { return app.BackupCrypto != nil },
		BackupKeyID:      func() string { return backupKeyID(app) },
		HA:               func(ctx context.Context) HAState { return haState(ctx, app) },
		SharedDBCheck:    func() bool { return db.DriverOf() != db.DriverSQLite },
		SelfEnvironments: SelfEnvironmentsFrom(app.DB, app.DockerPool),
	}, app.Logger)
	h := &handler{app: app, svc: svc}
	view := rbac.RequirePermission(app.RBACService, rbac.PermComplianceView)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.With(view).Get("/compliance/posture", h.handlePosture,
			apispec.Query("sections", "Comma-separated section IDs; empty means all"),
			apispec.Query("redact", "1 blanks sensitive facts; defaults to 1 when export=1"),
			apispec.Query("export", "1 records the download in the audit log"),
			apispec.Response(Report{}))
	})
}

func backupKeyID(app *router.AppDeps) string {
	if app.BackupCrypto == nil {
		return ""
	}
	return app.BackupCrypto.KeyID()
}

// haState reads the cluster service at request time; the service is registered after
// routes are mounted, so a lookup at Mount would always miss.
func haState(ctx context.Context, app *router.AppDeps) HAState {
	st := HAState{Driver: string(db.DriverOf()), PluginStore: pluginStoreKind(os.Getenv("MCHARBOR_PLUGIN_STORE"))}
	if ha, ok := app.LookupService("ha"); ok {
		if ctl, ok := ha.(clusterinfo.HAMode); ok {
			st.Mode = ctl.Mode()
		}
	}
	svc, ok := app.LookupService("cluster")
	if !ok {
		return st
	}
	p, ok := svc.(clusterinfo.Provider)
	if !ok {
		return st
	}
	cs := p.Info()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	st.Known = true
	st.Ready, _ = cs.Ready(ctx)
	if reg := cs.Membership(); reg != nil {
		st.NodeID, st.Role = reg.NodeID(), reg.Role()
	}
	return st
}

// pluginStoreKind reduces MCHARBOR_PLUGIN_STORE to its kind, dropping the path or
// storage-location ID a shared-fs or object store carries.
func pluginStoreKind(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "local"
	}
	if i := strings.IndexByte(v, ':'); i >= 0 {
		return v[:i]
	}
	return v
}
