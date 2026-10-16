// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"slices"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Runtime is the whole of what a runbook step may reach. It is a narrow
// interface rather than the Docker client so a reviewer can see at a glance
// that no step can exec, build, or write a file.
type Runtime interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRestart(ctx context.Context, containerID string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error)
}

// catalogEntry is one action or diagnostic a step may name, with the
// permissions it is checked against. This is a data table on purpose: a switch
// would hide an ungated entry, and the whole point is that a reviewer can read
// the gating of every step kind in one place.
type catalogEntry struct {
	Key         string
	Description string

	// DescriptionKey and DescriptionNS replace Description for a plugin entry: the plugin
	// owns the wording, so the editor resolves the key in the plugin's own i18n namespace.
	DescriptionKey string
	DescriptionNS  string
	ParamSpecs     []ParamSpec
	Target         string
	Required       []rbac.Permission
	Destructive    bool

	// SelfProtected marks an entry that starts, stops or restarts a container, which a McHarbor
	// container may only take from a user holding host_self.manage.
	SelfProtected bool

	// Streams marks an entry the offline read cache can never serve, because
	// the service worker never caches a stream.
	Streams bool

	// Run performs a host entry against the Docker runtime. A diagnostic returns
	// what to show the operator; an action returns a short confirmation line.
	Run func(ctx context.Context, rt Runtime, step Step) (string, error)

	// Domain and Call describe a plugin entry instead: the route that whichever
	// plugin serves Domain declares, dispatched in-process so the plugin's key is
	// enforced by the plugin. Plugin is that provider's slug; a plugin entry is built
	// from a loaded plugin's descriptor, so it exists only while that plugin does.
	Domain string
	Call   *pluginCall
	Plugin string
}

// containerParams is the one parameter every Docker container entry takes.
var containerParams = []ParamSpec{{Name: "container", Type: ParamTarget, Target: TargetContainer}}

// targetParam names the parameter holding the object the entry acts on, or ""
// for an entry that acts on the run's environment itself.
func (e catalogEntry) targetParam() string {
	for _, p := range e.ParamSpecs {
		if p.Type == ParamTarget && p.Target == e.Target {
			return p.Name
		}
	}
	return ""
}

// paramSpec returns the declared parameter called name.
func (e catalogEntry) paramSpec(name string) (ParamSpec, bool) {
	for _, p := range e.ParamSpecs {
		if p.Name == name {
			return p, true
		}
	}
	return ParamSpec{}, false
}

// actionCatalog is every action a runbook step may execute: the host's own plus
// whatever loaded plugins declare. Every entry declares its permissions, and the
// service refuses a step whose action key is not in here — a free-text command
// step would be a remote execution primitive behind a text field.
func (s *Service) actionCatalog() []catalogEntry {
	actions, _ := s.pluginCatalog()
	return append(hostActions(), actions...)
}

// hostActions is every action the host itself executes against the Docker runtime.
func hostActions() []catalogEntry {
	return []catalogEntry{
		{
			Key:           "container.restart",
			Description:   "Restart the container this run is against",
			ParamSpecs:    containerParams,
			Target:        TargetContainer,
			Required:      []rbac.Permission{rbac.PermContainersManage},
			Destructive:   true,
			SelfProtected: true,
			Run: func(ctx context.Context, rt Runtime, step Step) (string, error) {
				target := stepTarget(step)
				if _, err := rt.ContainerRestart(ctx, target, client.ContainerRestartOptions{}); err != nil {
					return "", err
				}
				return "restarted " + target, nil
			},
		},
		{
			Key:           "container.stop",
			Description:   "Stop the container this run is against",
			ParamSpecs:    containerParams,
			Target:        TargetContainer,
			Required:      []rbac.Permission{rbac.PermContainersManage},
			Destructive:   true,
			SelfProtected: true,
			Run: func(ctx context.Context, rt Runtime, step Step) (string, error) {
				target := stepTarget(step)
				if _, err := rt.ContainerStop(ctx, target, client.ContainerStopOptions{}); err != nil {
					return "", err
				}
				return "stopped " + target, nil
			},
		},
		{
			Key:           "container.start",
			Description:   "Start the container this run is against",
			ParamSpecs:    containerParams,
			Target:        TargetContainer,
			Required:      []rbac.Permission{rbac.PermContainersManage},
			Destructive:   true,
			SelfProtected: true,
			Run: func(ctx context.Context, rt Runtime, step Step) (string, error) {
				target := stepTarget(step)
				if _, err := rt.ContainerStart(ctx, target, client.ContainerStartOptions{}); err != nil {
					return "", err
				}
				return "started " + target, nil
			},
		},
	}
}

// diagnosticCatalog is every read-only diagnostic a step may run, host and plugin.
// Read-only still means gated: log output is the most sensitive thing on the platform.
func (s *Service) diagnosticCatalog() []catalogEntry {
	_, diagnostics := s.pluginCatalog()
	return append(hostDiagnostics(), diagnostics...)
}

// hostDiagnostics is every read-only diagnostic the host runs against the Docker runtime.
func hostDiagnostics() []catalogEntry {
	return []catalogEntry{
		{
			Key:         "container.state",
			Description: "The container's current state, health and exit code",
			ParamSpecs:  containerParams,
			Target:      TargetContainer,
			Required:    []rbac.Permission{rbac.PermContainersView},
			Run: func(ctx context.Context, rt Runtime, step Step) (string, error) {
				infoRes, err := rt.ContainerInspect(ctx, stepTarget(step), client.ContainerInspectOptions{})
				if err != nil {
					return "", err
				}
				info := infoRes.Container
				return describeState(info), nil
			},
		},
		{
			Key:         "container.logs",
			Description: "The last 150 lines of the container's log",
			ParamSpecs:  containerParams,
			Target:      TargetContainer,
			Required:    []rbac.Permission{rbac.PermContainersView},
			Run: func(ctx context.Context, rt Runtime, step Step) (string, error) {
				return readLogs(ctx, rt, stepTarget(step))
			},
		},
	}
}

// lookupAction returns the action entry for key, or false when the key is not
// one a step may name right now.
func (s *Service) lookupAction(key string) (catalogEntry, bool) {
	for _, e := range s.actionCatalog() {
		if e.Key == key {
			return e, true
		}
	}
	return catalogEntry{}, false
}

// lookupDiagnostic returns the diagnostic entry for key, or false when the key
// is not one a step may name.
func (s *Service) lookupDiagnostic(key string) (catalogEntry, bool) {
	for _, e := range s.diagnosticCatalog() {
		if e.Key == key {
			return e, true
		}
	}
	return catalogEntry{}, false
}

// entryFor resolves whichever catalog the step's kind names.
func (s *Service) entryFor(step Step) (catalogEntry, bool) {
	switch step.Kind {
	case StepAction:
		return s.lookupAction(step.ActionKey)
	case StepDiagnostic:
		return s.lookupDiagnostic(step.DiagnosticKey)
	default:
		return catalogEntry{}, false
	}
}

// hostEntryFor resolves a step against the host's own entries only, for the code that
// runs a host entry and so never needs a plugin's.
func hostEntryFor(step Step) (catalogEntry, bool) {
	var entries []catalogEntry
	key := ""
	switch step.Kind {
	case StepAction:
		entries, key = hostActions(), step.ActionKey
	case StepDiagnostic:
		entries, key = hostDiagnostics(), step.DiagnosticKey
	}
	for _, e := range entries {
		if e.Key == key {
			return e, true
		}
	}
	return catalogEntry{}, false
}

// requiredFor names the permissions a step will be checked against, for the
// client to show on a denied control instead of waiting for a 403.
func (s *Service) requiredFor(step Step) []string {
	entry, ok := s.entryFor(step)
	if !ok {
		return nil
	}
	return permissionKeys(entry)
}

// hostRequiredFor is requiredFor over the host's own entries, for a Store that has
// not been wired to a Service.
func hostRequiredFor(step Step) []string {
	entry, ok := hostEntryFor(step)
	if !ok {
		return nil
	}
	return permissionKeys(entry)
}

func permissionKeys(e catalogEntry) []string {
	out := make([]string, 0, len(e.Required))
	for _, p := range e.Required {
		out = append(out, string(p))
	}
	return out
}

// catalogView is the authoring picker's menu. A plugin entry exists only while the
// plugin declaring it is loaded, so the picker never names a step that would fail the
// moment it is pressed.
func (s *Service) catalogView() CatalogView {
	view := CatalogView{Actions: []CatalogEntryView{}, Diagnostics: []CatalogEntryView{}}
	for _, e := range s.actionCatalog() {
		view.Actions = append(view.Actions, entryView(e))
	}
	for _, e := range s.diagnosticCatalog() {
		view.Diagnostics = append(view.Diagnostics, entryView(e))
	}
	return view
}

func entryView(e catalogEntry) CatalogEntryView {
	perms := permissionKeys(e)
	names := make([]string, 0, len(e.ParamSpecs))
	for _, p := range e.ParamSpecs {
		names = append(names, p.Name)
	}
	specs := e.ParamSpecs
	if specs == nil {
		specs = []ParamSpec{}
	}
	return CatalogEntryView{
		Key: e.Key, Description: e.Description, DescriptionKey: e.DescriptionKey, DescriptionNS: e.DescriptionNS,
		Params: names, ParamSpecs: specs,
		RequiredPermissions: perms, Destructive: e.Destructive, Streams: e.Streams,
		Plugin: e.Plugin, Target: e.Target,
	}
}

// stepTarget is the object the step acts on — a container, a VM, a system
// container, a pool, a swarm service or node, a pod or a deployment: the step's
// own parameter when the author pinned one, otherwise the run's subject, which
// the service writes into the step before executing it.
func stepTarget(step Step) string {
	if step.ActionParams == nil {
		return ""
	}
	entry, ok := hostEntryFor(step)
	if !ok {
		return step.ActionParams["container"]
	}
	name := entry.targetParam()
	if name == "" {
		return ""
	}
	return step.ActionParams[name]
}

// subjectFits reports whether a run's subject kind can stand in for an entry's
// target. A subject kind the runbook engine does not own — an alert source type —
// is let through, which is how one runbook serves every object an alert fires for.
// A provider's attach kind fits the target kinds that provider declared for it.
func (s *Service) subjectFits(subjectKind, target string) bool {
	switch subjectKind {
	case AttachContainer:
		return target == TargetContainer
	case AttachStack, AttachEnvironment:
		return false
	}
	if a, ok := s.declaredAttach(subjectKind); ok {
		return slices.Contains(a.Subjects, target)
	}
	return true
}
