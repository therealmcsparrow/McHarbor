// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"github.com/therealmcsparrow/mcharbor/core/router"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// ServiceBundle carries the capability implementations the AI tools call
// into. Each field mirrors the same-named injector on ai_tools.Deps and is
// typed `any` for the same reason: the tool does the type assertion to the
// narrow interface it needs, so this package never imports the sibling
// modules that own the concrete services. The app builder
// (internal/bootstrap.NewAIToolBundle) populates it once at startup and
// main.go passes it to WireTools. A nil field is tolerated — the matching
// tool reports "not wired" at call time.
type ServiceBundle struct {
	ContainerList    any
	ContainerExec    any
	ContainerInspect any
	ImageList        any
	VolumeList       any
	NetworkList      any
	StackList        any
	EnvironmentList  any
	UserList         any
	AuditSearch      any
	WorkflowList     any
	CommandExecutor  any
	AlertCatalog     any

	// Authoring capabilities. Nil is tolerated: the matching create tool reports
	// "not wired" at call time rather than failing startup.
	WorkflowAuthor     any
	AlertAuthor        any
	UpdatePolicyAuthor any
	ScheduleAuthor     any
}

// WireTools builds the ai_tools Deps from the bundle and the running
// app and installs them via ai_tools.SetDeps. Safe to call once at
// startup. The DB cache is installed when app.DB is available.
func WireTools(app *router.AppDeps, bundle *ServiceBundle) {
	if app == nil {
		return
	}
	var b ServiceBundle
	if bundle != nil {
		b = *bundle
	}
	d := &ai_tools.Deps{
		DB:               app.DB,
		RBAC:             app.RBACService,
		Audit:            app.AuditLog,
		PluginAdapters:   app.PluginAdapters,
		ContainerList:    b.ContainerList,
		ContainerExec:    b.ContainerExec,
		ContainerInspect: b.ContainerInspect,
		ImageList:        b.ImageList,
		VolumeList:       b.VolumeList,
		NetworkList:      b.NetworkList,
		StackList:        b.StackList,
		EnvironmentList:  b.EnvironmentList,
		UserList:         b.UserList,
		AuditSearch:      b.AuditSearch,
		WorkflowList:     b.WorkflowList,
		CommandExecutor:  b.CommandExecutor,
		AlertCatalog:     b.AlertCatalog,

		WorkflowAuthor:     b.WorkflowAuthor,
		AlertAuthor:        b.AlertAuthor,
		UpdatePolicyAuthor: b.UpdatePolicyAuthor,
		ScheduleAuthor:     b.ScheduleAuthor,
	}
	if app.DB != nil {
		d.Cache = ai_tools.NewDBCache(app.DB, 0, 0)
	}
	ai_tools.SetDeps(d)
}
