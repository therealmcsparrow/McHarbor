// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package ai_tools (file: tools.go) collects the read tools shipped
// in Phase 1. Each tool is registered in init(); see registry.go.
package ai_tools

import (
	"database/sql"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Deps holds the runtime dependencies tools need. Wired in main.go
// via SetDeps. Tools that touch services read them from here so the
// ai_assistant package doesn't import every other module directly.
type Deps struct {
	DB             *sql.DB
	DockerPool     any
	RBAC           *rbac.Service
	Audit          *audit.Logger
	Cache          Cache
	PluginAdapters any
	// Service injectors. Each is an `any`; the tool does a type
	// assertion to the interface it actually needs. The ai_tools
	// package depends on rbac + audit but not on the module services
	// directly — this keeps the dependency graph narrow.
	ContainerExec    any
	ContainerList    any
	ContainerInspect any
	ImageList        any
	ImageInspect     any
	VolumeList       any
	NetworkList      any
	StackList        any
	StackInspect     any
	EnvironmentList  any
	WorkflowList     any
	WorkflowInspect  any
	UserList         any
	AuditSearch      any
	CommandExecutor  any
	AlertCatalog     any

	// The authoring injectors below back the consent-gated create tools; each is
	// gated on the owning module's manage permission before the model ever sees it.
	WorkflowAuthor     any
	AlertAuthor        any
	UpdatePolicyAuthor any
	ScheduleAuthor     any
}

var (
	depsMu sync.RWMutex
	deps   *Deps
)

// SetDeps installs the tool dependencies. Called once at startup.
func SetDeps(d *Deps) {
	depsMu.Lock()
	defer depsMu.Unlock()
	deps = d
}

// GetDeps returns the installed deps or nil. Tools use the exported
// getters below instead of dereferencing deps directly so a nil
// install fails gracefully in dev mode.
func GetDeps() *Deps {
	depsMu.RLock()
	defer depsMu.RUnlock()
	return deps
}

// HasDeps reports whether SetDeps has run. Useful for the catalog
// endpoint, which can return an empty list in tests before wiring.
func HasDeps() bool {
	depsMu.RLock()
	defer depsMu.RUnlock()
	return deps != nil
}

// GetCache returns the installed Cache, or nil when no cache has been
// wired. Handlers should treat nil as "no caching" and continue.
func GetCache() Cache {
	d := GetDeps()
	if d == nil {
		return nil
	}
	return d.Cache
}
