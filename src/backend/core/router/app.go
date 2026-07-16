// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// AppDeps holds all shared dependencies injected into module handlers.
type AppDeps struct {
	Config         *config.Config
	DB             *sql.DB
	DockerPool     *docker.ClientPool
	KubernetesPool *kubernetes.ClientPool
	AgentPool      *agent.AgentPool
	AuthService    *auth.Service
	RBACService    *rbac.Service
	AuditLog       *audit.Logger
	Encryption     *encryption.Service
	BackupCrypto   *backupcrypto.Service
	Logger         *slog.Logger
	Compact        *db.CompactManager
	StaticDir      string

	// Module mount functions (set during initialization)
	mountPublic    []func(chi.Router)
	mountAuth      []func(chi.Router)
	mountProtected []func(chi.Router)

	// services is a tiny in-process service-locator for stateful
	// module singletons that need to be reachable from HTTP
	// handlers but don't fit the simple per-module Mount() pattern
	// (typically because they need a reference to a long-lived
	// connection such as the advisory-lock coordinator). Modules
	// register with RegisterService() and read with LookupService().
	services map[string]any

	// bgCtx is the long-lived application context handed to
	// background workers (the self-update checker, the backup
	// retention pruner, etc.). It is wired by main.go via
	// RegisterBackgroundContext and read via ContextOrBackground.
	bgCtx context.Context
}

// NewAppDeps creates a new AppDeps with all core services.
func NewAppDeps(cfg *config.Config, database *sql.DB, dockerPool *docker.ClientPool, k8sPool *kubernetes.ClientPool, agentPool *agent.AgentPool, authSvc *auth.Service, rbacSvc *rbac.Service, auditLog *audit.Logger, enc *encryption.Service, backupCrypto *backupcrypto.Service, logger *slog.Logger) *AppDeps {
	return &AppDeps{
		Config:         cfg,
		DB:             database,
		DockerPool:     dockerPool,
		KubernetesPool: k8sPool,
		AgentPool:      agentPool,
		AuthService:    authSvc,
		RBACService:    rbacSvc,
		AuditLog:       auditLog,
		Encryption:     enc,
		BackupCrypto:   backupCrypto,
		Logger:         logger,
		Compact:        db.NewCompactManager(database, logger),
		StaticDir:      "./static",
	}
}

// RegisterPublicRoutes registers a module's public routes (no auth middleware or auth rate limit).
func (a *AppDeps) RegisterPublicRoutes(mount func(chi.Router)) {
	a.mountPublic = append(a.mountPublic, mount)
}

// RegisterAuthRoutes registers a module's auth routes (no auth middleware).
func (a *AppDeps) RegisterAuthRoutes(mount func(chi.Router)) {
	a.mountAuth = append(a.mountAuth, mount)
}

// RegisterProtectedRoutes registers a module's protected routes.
func (a *AppDeps) RegisterProtectedRoutes(mount func(chi.Router)) {
	a.mountProtected = append(a.mountProtected, mount)
}

// MountPublicRoutes mounts all registered public routes.
func (a *AppDeps) MountPublicRoutes(r chi.Router) {
	for _, mount := range a.mountPublic {
		mount(r)
	}
}

// MountAuthRoutes mounts all registered auth routes.
func (a *AppDeps) MountAuthRoutes(r chi.Router) {
	for _, mount := range a.mountAuth {
		mount(r)
	}
}

// MountProtectedRoutes mounts all registered protected module routes.
func (a *AppDeps) MountProtectedRoutes(r chi.Router) {
	for _, mount := range a.mountProtected {
		mount(r)
	}
}

// RegisterService publishes a singleton service under `name` so
// HTTP handlers can look it up with LookupService(). The intended
// use is for stateful cross-cutting concerns (advisory-lock
// coordinator, cluster status, metrics scraper) that need a
// long-lived reference passed in from main(). Lookup is by
// interface{}; callers must type-assert the returned value.
func (a *AppDeps) RegisterService(name string, svc any) {
	if a.services == nil {
		a.services = make(map[string]any)
	}
	a.services[name] = svc
}

// LookupService returns a previously-registered service by name, or
// nil if no such service was registered. The boolean reports
// whether the lookup succeeded so callers can distinguish a
// missing service from one registered as a typed nil.
func (a *AppDeps) LookupService(name string) (any, bool) {
	if a.services == nil {
		return nil, false
	}
	svc, ok := a.services[name]
	return svc, ok
}

// ContextOrBackground returns a context usable for background
// goroutines. It is wired by main.go with the application's
// root context. Modules that need to run a periodic worker
// (e.g. the self-update checker) should call this once at
// startup and pass the result into the goroutine. Until main.go
// has had a chance to register a context, a fresh
// context.Background() is returned so callers can compile
// without an init-order hazard.
func (a *AppDeps) ContextOrBackground() context.Context {
	if a.bgCtx != nil {
		return a.bgCtx
	}
	return context.Background()
}

// RegisterBackgroundContext wires the long-lived application
// context that the AppDeps will hand out to background workers.
func (a *AppDeps) RegisterBackgroundContext(ctx context.Context) {
	a.bgCtx = ctx
}
