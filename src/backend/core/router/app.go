// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"database/sql"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
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
	Logger         *slog.Logger
	StaticDir      string

	// Module mount functions (set during initialization)
	mountAuth      []func(chi.Router)
	mountProtected []func(chi.Router)
}

// NewAppDeps creates a new AppDeps with all core services.
func NewAppDeps(cfg *config.Config, db *sql.DB, dockerPool *docker.ClientPool, k8sPool *kubernetes.ClientPool, agentPool *agent.AgentPool, authSvc *auth.Service, rbacSvc *rbac.Service, auditLog *audit.Logger, enc *encryption.Service, logger *slog.Logger) *AppDeps {
	return &AppDeps{
		Config:         cfg,
		DB:             db,
		DockerPool:     dockerPool,
		KubernetesPool: k8sPool,
		AgentPool:      agentPool,
		AuthService:    authSvc,
		RBACService:    rbacSvc,
		AuditLog:       auditLog,
		Encryption:     enc,
		Logger:         logger,
		StaticDir:      "./static",
	}
}

// RegisterAuthRoutes registers a module's auth routes (no auth middleware).
func (a *AppDeps) RegisterAuthRoutes(mount func(chi.Router)) {
	a.mountAuth = append(a.mountAuth, mount)
}

// RegisterProtectedRoutes registers a module's protected routes.
func (a *AppDeps) RegisterProtectedRoutes(mount func(chi.Router)) {
	a.mountProtected = append(a.mountProtected, mount)
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
