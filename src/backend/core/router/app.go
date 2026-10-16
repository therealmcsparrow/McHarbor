// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package router defines the AppDeps container that wires core services into modules.
package router

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/apikeyusage"
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/firstrun"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// AppDeps holds all shared dependencies injected into module handlers.
type AppDeps struct {
	Config      *config.Config
	DB          *sql.DB
	DockerPool  *docker.ClientPool
	AgentPool   *agent.AgentPool
	AuthService *auth.Service
	RBACService *rbac.Service
	AuditLog    *audit.Logger
	// FirstRun guards the routes a fresh install answers before any account exists.
	FirstRun     *firstrun.Gate
	Encryption   *encryption.Service
	BackupCrypto *backupcrypto.Service
	Logger       *slog.Logger
	Compact      *db.CompactManager
	StaticDir    string

	// Spec collects every route as it is mounted, so the OpenAPI document is
	// generated from the router that runs rather than maintained beside it.
	Spec *apispec.Registry

	// PluginAdapters is the plugin runtime held as `any` so this
	// core package doesn't have to import the plugin runtime module.
	PluginAdapters any

	// AIEnv is the AI environment provider supplied by the bootstrap layer.
	AIEnv any

	// SecretAppliers knows which module can re-materialise which resource
	// type when a secret is rotated. Modules register into it at mount time
	// because core/secrets cannot import them, and an empty registry is a
	// valid one — it simply reports every resource as unsupported.
	SecretAppliers *secrets.ApplierRegistry

	// APIKeyUsage receives per-API-key request telemetry. Nil until
	// main.go wires the recorder, and nil-safe everywhere it is read, so
	// a request that arrives during startup records nothing rather than
	// panicking.
	APIKeyUsage apikeyusage.Sink

	// ResolveClientAddress attributes a request to an origin address,
	// honouring forwarding headers only from configured trusted proxies.
	// Nil means "peer only", which is the secure default.
	ResolveClientAddress apikeyusage.AddressResolverFunc

	// StatsHub is the shared live container-stats cache every stats reader goes
	// through. Nil in tests that do not wire it; readers fall back to pulling.
	StatsHub *statshub.Hub

	// StatsPushSink receives agents' pushed stats batches. Nil means agents are
	// told to stop pushing.
	StatsPushSink agent.StatsPushSink

	// NodeLink carries an agent call to the node that holds the agent. Nil on a
	// single-node install, where every agent is already here.
	NodeLink *nodelink.Link

	// PluginRoutes is the live routing table for plugin-contributed
	// routes. Unlike the module mount functions below — which are
	// drained once, when router.New builds the tree — it can be changed
	// while the server is running, which is what an App Store install,
	// enable, update or uninstall actually needs. See
	// core/router/pluginroutes.go.
	PluginRoutes *PluginRouter

	// ApprovalGate holds destructive requests that an approval rule covers. Set by
	// the approvals module at mount time; nil leaves the protected chain unchanged.
	ApprovalGate func(http.Handler) http.Handler

	// MFAEnrollmentOwed reports whether a user's session is held at MFA enrolment by the
	// security.mfa_required policy. Set by the mfa module at mount time; nil gates nothing.
	MFAEnrollmentOwed func(ctx context.Context, user *auth.User) (bool, error)

	// Root is the finished router, kept so an approved request can be replayed
	// through exactly the chain a live request takes. Nil until router.New returns.
	Root http.Handler

	// Module mount functions (set during initialization)
	mountPublic    []func(*apispec.Router)
	mountAuth      []func(*apispec.Router)
	mountProtected []func(*apispec.Router)

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
	bgCtx context.Context //nolint:containedctx // process-lifetime root context handed to background workers; there is no per-call ctx at that layer
}

// NewAppDeps creates a new AppDeps with all core services.
func NewAppDeps(cfg *config.Config, database *sql.DB, dockerPool *docker.ClientPool, agentPool *agent.AgentPool, authSvc *auth.Service, rbacSvc *rbac.Service, auditLog *audit.Logger, enc *encryption.Service, backupCrypto *backupcrypto.Service, logger *slog.Logger) *AppDeps {
	return &AppDeps{
		Config:       cfg,
		DB:           database,
		DockerPool:   dockerPool,
		AgentPool:    agentPool,
		AuthService:  authSvc,
		RBACService:  rbacSvc,
		AuditLog:     auditLog,
		FirstRun:     firstrun.New(func() bool { return authSvc != nil && !authSvc.HasAnyUser() }, logger),
		Encryption:   enc,
		BackupCrypto: backupCrypto,
		Logger:       logger,
		Compact:      db.NewCompactManager(database, logger),
		StaticDir:    "./static",
		// Constructed here rather than in router.New because the plugin
		// runtime boots first (main.go) and starts handing routes over
		// before the router exists. An empty plugin router serves 404,
		// which is the right answer until something registers.
		PluginRoutes: NewPluginRouter(logger),
		// Built here so a module that mounts before the first request has a
		// registry to record into; the spec is read once, after mounting.
		Spec: apispec.NewRegistry(),
		// Built here rather than at first use so a module that mounts before
		// the secrets module has somewhere to register.
		SecretAppliers: secrets.NewApplierRegistry(),
	}
}

// RegisterPublicRoutes registers a module's public routes (no auth middleware or auth rate limit).
func (a *AppDeps) RegisterPublicRoutes(mount func(*apispec.Router)) {
	a.mountPublic = append(a.mountPublic, mount)
}

// RegisterAuthRoutes registers a module's auth routes (no auth middleware).
func (a *AppDeps) RegisterAuthRoutes(mount func(*apispec.Router)) {
	a.mountAuth = append(a.mountAuth, mount)
}

// RegisterProtectedRoutes registers a module's protected routes.
func (a *AppDeps) RegisterProtectedRoutes(mount func(*apispec.Router)) {
	a.mountProtected = append(a.mountProtected, mount)
}

// specRouter wraps the chi router the three bands are mounted on. The prefix is
// /api because router.New mounts all of them inside that Route.
func (a *AppDeps) specRouter(r chi.Router, group apispec.Group) *apispec.Router {
	if a.Spec == nil {
		a.Spec = apispec.NewRegistry()
	}
	return apispec.NewRouter(r, a.Spec, "/api", group)
}

// MountPublicRoutes mounts all registered public routes.
func (a *AppDeps) MountPublicRoutes(r chi.Router) {
	rt := a.specRouter(r, apispec.GroupPublic)
	for _, mount := range a.mountPublic {
		mount(rt)
	}
}

// MountAuthRoutes mounts all registered auth routes.
func (a *AppDeps) MountAuthRoutes(r chi.Router) {
	rt := a.specRouter(r, apispec.GroupAuth)
	for _, mount := range a.mountAuth {
		mount(rt)
	}
}

// MountProtectedRoutes mounts all registered protected module routes.
func (a *AppDeps) MountProtectedRoutes(r chi.Router) {
	rt := a.specRouter(r, apispec.GroupProtected)
	for _, mount := range a.mountProtected {
		mount(rt)
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
