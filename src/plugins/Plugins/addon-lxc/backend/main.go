// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor System Containers.
//
// Full-system LXC containers on your own hosts: a whole distribution with its
// own init, users and services, in a fraction of a VM's memory and starting in
// about a second. Unprivileged by default, with snapshots, clones, templates
// and a browser console.
//
// # How this differs from the Docker section, and why both exist
//
// A Docker container is one process tree wrapped around one application image;
// a system container is a machine. It boots systemd, runs cron and sshd and
// syslog, holds users, and is administered the way a server is. That is the
// wrong shape for shipping an application and the right shape for a lightweight
// VM, which is exactly the gap this plugin fills — most homelab guests are
// running a distribution, not a hypervisor workload, and a system container
// gives them that at roughly a tenth of the memory.
//
// # What this plugin does not contain
//
// No LXD, no Incus, no `lxc-*` toolchain. McHarbor uses libcontainer directly.
// The reasoning matches the libvirt decision in addon-kvm: LXD and Incus are
// daemons with their own database of containers, profiles and storage pools,
// which then drift from McHarbor's; and everything needed is a namespace, a
// cgroup and a mount, all of which libcontainer already does. Removing the
// daemon removes a translation layer, not a safety layer.
//
// # Where the work actually happens
//
// Not here. The McHarbor process cannot host containers — a
// `docker compose up -d` would kill every running one, and creating a user
// namespace and a delegated cgroup subtree from inside a container is not a
// thing to rely on. So the executor lives in the agent, and this plugin reaches
// it through the host's agent-rpc bridge, which plugin.yaml declares.
//
// # The one thing worth knowing before reading further
//
// cgroup v2 limits apply to a *running* container. Changing a VM's memory means
// a reboot; changing a container's means one write to `memory.max` and the
// change is live. Most of the shape of this plugin's limits path — a separate
// route, a separate protocol message, `applied_limits` stored apart from the
// requested values — follows from taking that seriously rather than treating
// limits as ordinary edit fields.
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginLogger is the host logger's shape.
type pluginLogger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// aiCompleter is the slice of the host's AI handle this plugin uses.
//
// Declared narrowly on purpose: this plugin asks one question — "why will this
// container not start?" — and nothing here should be able to do more with the
// router than that.
type aiCompleter interface {
	Complete(ctx context.Context, req plugin.AIRequest) (plugin.AIResponse, error)
	Available() bool
}

// LXCPlugin is the plugin instance the host loads.
type LXCPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	svc      *Service
	handler  *Handler
	backups  *backupScheduler
	logger   pluginLogger
}

// Plugin is the symbol the host looks up after dlopen.
var Plugin plugin.Plugin = &LXCPlugin{manifest: manifest}

var _ plugin.Plugin = (*LXCPlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-lxc",
	Name:     "System Containers",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Run full-system LXC containers on your own hosts, unprivileged by " +
		"default, with live resource limits, snapshots, clones and a console.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks this
	// slice, so the two lists cannot drift apart.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key:      "ct.view",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows seeing system containers, their mounts, their snapshots, " +
				"their processes and their resource usage.",
		},
		{
			Key:      "ct.manage",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows creating, editing, starting, stopping, freezing, " +
				"snapshotting and cloning system containers, and changing their CPU, " +
				"memory and pid limits. Destroying one needs ct.delete.",
		},
		{
			Key:      "ct.delete",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows destroying a system container and its rootfs, and " +
				"deleting its snapshots.",
		},
		{
			// Separate from manage because a container console is a root shell
			// inside that container, and plenty of operators want to grant
			// start/stop without it — the same reasoning that separates
			// containers.manage from exec.
			Key:      "ct.console",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows attaching a console to a system container. This is a " +
				"root shell inside the container, which is why it is separate from " +
				"ct.manage.",
		},
		{
			Key:      "ct.backup",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows seeing backup schedules and history, creating and editing " +
				"schedules, starting a backup and deleting one. Restoring is separate and " +
				"needs ct.restore.",
		},
		{
			// Separate from ct.backup because a restore replaces a whole
			// filesystem and cannot be undone, which taking a backup cannot.
			Key:      "ct.restore",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows rebuilding a container's filesystem from a backup, replacing " +
				"everything on it now. The container has to be stopped and its name typed out. " +
				"Recreating a deleted container from its backup also needs ct.manage.",
		},
		{
			// Never in a role preset. Each of these removes a boundary between
			// the container and the host itself.
			Key:      "ct.privileged",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows turning off the user namespace, binding arbitrary host " +
				"paths into a container, passing host devices through, and enabling " +
				"nesting. Root in a privileged container is root on the host in every " +
				"way that matters, so this is never granted by a role preset.",
		},
	},
}

// Manifest returns the plugin's manifest.
func (p *LXCPlugin) Manifest() plugin.Manifest { return p.manifest }

// Init wires the plugin to the host.
func (p *LXCPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host
	p.logger = host.Logger

	if newAgentRPC(host.AgentRPC) == nil {
		// Not fatal: the list still renders and explains that no host can run
		// containers, which is more useful than the section refusing to load.
		p.logger.Warn("system containers: the host wired no agent bridge; " +
			"containers will be read-only")
	}

	p.svc = NewService(host.DB, host.AgentRPC, host.BackupLog,
		host.Environments, host.Storage, p.logger)

	// Columns added after the first release. Applied here because a plugin's
	// migrations run only at install and would otherwise never reach an
	// instance that already has this plugin.
	if err := p.svc.EnsureSchema(ctx); err != nil {
		return fmt.Errorf("system containers: updating the schema: %w", err)
	}

	// The AI handle is optional. A nil one means the diagnose endpoint answers
	// "not available" rather than failing.
	var ai aiCompleter
	if host.AI != nil {
		ai = host.AI
	}
	p.handler = NewHandler(p.svc, host.Audit, host.RBAC, ai, p.logger)

	// Give every declared key a label from this plugin's i18n namespace.
	// Without this the role editor lists the raw key, which tells an
	// administrator granting `ct.privileged` nothing about what they are about
	// to hand over. The host removes these again on Shutdown.
	if host.RBAC != nil {
		for _, decl := range p.manifest.AddsPerms {
			host.RBAC.RegisterLabel(decl.Key, "ct", "permissions."+decl.Key+".label")
		}
	}

	// A backup's progress lives in a goroutine, so a reload leaves the row saying
	// "running" with nothing behind it. Not fatal: a plugin that cannot reap
	// still manages containers, and the Backups page showing one stale row is a
	// smaller problem than the section refusing to load.
	if err := p.svc.reapOrphanedBackupRuns(ctx); err != nil {
		p.logger.Warn("system containers: reaping interrupted backup runs failed", "error", err)
	}

	if p.svc.rpc != nil {
		// Backup progress. The agent pushes a frame per stage and per second of
		// copying; nothing subscribed, they would be discarded, and a
		// multi-gigabyte backup would show "archiving" and nothing else for its
		// whole life.
		p.svc.rpc.Subscribe(p.svc.handleAgentFrame)

		// Behind the agent-bridge check deliberately: without a bridge every
		// firing would fail, and a schedule that logs a failure a minute forever
		// is worse than one that does not run.
		p.backups = newBackupScheduler(p.svc, p.logger)
		go p.backups.Start(context.WithoutCancel(ctx))
	}

	p.logger.Info("system containers ready")
	return nil
}

// Shutdown releases the plugin.
func (p *LXCPlugin) Shutdown(_ context.Context) error {
	if p.svc != nil && p.svc.rpc != nil {
		p.svc.rpc.Subscribe(nil)
	}
	if p.backups != nil {
		p.backups.Stop()
	}
	return nil
}

// RegisterRoutes registers the plugin's HTTP surface.
//
// Every route is Protected and its permission is declared in plugin.yaml — the
// host's plugin route registry wraps them in its permission middleware, so
// declaring correctly is enforcing. Two routes need more than the declared key:
// the mount route escalates to ct.privileged when the body asks for a host bind
// (HandleAddMount), and the recreate route also requires ct.manage on the target
// host (allowRecreate).
func (p *LXCPlugin) RegisterRoutes(reg plugin.Registry) error {
	routes := []struct {
		method string
		path   string
		// public marks the two agent transfer routes. They carry no session —
		// the caller is a remote agent redeeming a one-shot grant McHarbor minted
		// seconds earlier — so a permission key would refuse every legitimate
		// call. Everything else is Protected and declared in plugin.yaml.
		public  bool
		handler func(*Handler, http.ResponseWriter, *http.Request)
	}{
		// The host reboot's drain: a literal path, registered before /{id}.
		{http.MethodGet, "/api/system-containers/reboot-drain", false, (*Handler).HandleRebootDrainRunning},
		{http.MethodPost, "/api/system-containers/reboot-drain", false, (*Handler).HandleRebootDrainStop},
		{http.MethodGet, "/api/system-containers", false, (*Handler).HandleList},
		{http.MethodPost, "/api/system-containers", false, (*Handler).HandleCreate},
		{http.MethodGet, "/api/system-containers/{id}", false, (*Handler).HandleGet},
		{http.MethodPut, "/api/system-containers/{id}", false, (*Handler).HandleUpdate},
		{http.MethodDelete, "/api/system-containers/{id}", false, (*Handler).HandleDelete},
		{http.MethodGet, "/api/system-containers/{id}/stats", false, (*Handler).HandleStats},
		{http.MethodGet, "/api/system-containers/{id}/processes", false, (*Handler).HandleProcesses},

		{http.MethodPost, "/api/system-containers/{id}/start", false, (*Handler).HandleStart},
		{http.MethodPost, "/api/system-containers/{id}/shutdown", false, (*Handler).HandleShutdown},
		{http.MethodPost, "/api/system-containers/{id}/stop", false, (*Handler).HandleStop},
		{http.MethodPost, "/api/system-containers/{id}/restart", false, (*Handler).HandleRestart},
		{http.MethodPost, "/api/system-containers/{id}/freeze", false, (*Handler).HandleFreeze},
		{http.MethodPost, "/api/system-containers/{id}/unfreeze", false, (*Handler).HandleUnfreeze},

		{http.MethodPut, "/api/system-containers/{id}/limits", false, (*Handler).HandleUpdateLimits},

		{http.MethodGet, "/api/system-containers/{id}/nics", false, (*Handler).HandleListNICs},
		{http.MethodPost, "/api/system-containers/{id}/nics", false, (*Handler).HandleAddNIC},
		{http.MethodDelete, "/api/system-containers/{id}/nics/{nicId}", false, (*Handler).HandleDeleteNIC},

		{http.MethodGet, "/api/system-containers/{id}/mounts", false, (*Handler).HandleListMounts},
		{http.MethodPost, "/api/system-containers/{id}/mounts", false, (*Handler).HandleAddMount},
		{http.MethodDelete, "/api/system-containers/{id}/mounts/{mountId}", false, (*Handler).HandleDeleteMount},

		{http.MethodGet, "/api/system-containers/{id}/snapshots", false, (*Handler).HandleListSnapshots},
		{http.MethodPost, "/api/system-containers/{id}/snapshots", false, (*Handler).HandleCreateSnapshot},
		{http.MethodPost, "/api/system-containers/{id}/snapshots/{snapId}/revert", false, (*Handler).HandleRevertSnapshot},
		{http.MethodDelete, "/api/system-containers/{id}/snapshots/{snapId}", false, (*Handler).HandleDeleteSnapshot},

		{http.MethodPost, "/api/system-containers/{id}/clone", false, (*Handler).HandleClone},
		{http.MethodPost, "/api/system-containers/{id}/template", false, (*Handler).HandleTemplate},

		{http.MethodPost, "/api/system-containers/{id}/diagnose", false, (*Handler).HandleDiagnose},

		{http.MethodGet, "/api/system-containers/{id}/backup-plans", false, (*Handler).HandleListBackupPlans},
		{http.MethodPost, "/api/system-containers/{id}/backup-plans", false, (*Handler).HandleCreateBackupPlan},
		{http.MethodPut, "/api/system-containers/backup-plans/{planId}", false, (*Handler).HandleUpdateBackupPlan},
		{http.MethodDelete, "/api/system-containers/backup-plans/{planId}", false, (*Handler).HandleDeleteBackupPlan},
		{http.MethodGet, "/api/system-containers/{id}/backups", false, (*Handler).HandleListBackupRuns},
		{http.MethodPost, "/api/system-containers/{id}/backups", false, (*Handler).HandleRunBackup},
		{http.MethodGet, "/api/system-containers/backups/{runId}", false, (*Handler).HandleBackupRun},
		{http.MethodDelete, "/api/system-containers/backups/{runId}", false, (*Handler).HandleDeleteBackupRun},
		{http.MethodPost, "/api/system-containers/backups/{runId}/restore", false, (*Handler).HandleRestoreBackup},
		{http.MethodGet, "/api/system-containers/orphaned-backups", false, (*Handler).HandleOrphanedBackups},
		{http.MethodGet, "/api/system-containers/backups/{runId}/manifest", false, (*Handler).HandleBackupManifest},
		{http.MethodPost, "/api/system-containers/backups/{runId}/recreate", false, (*Handler).HandleRecreateFromBackup},

		{http.MethodPut, "/api/system-containers/backups/internal/{grantId}", true, (*Handler).HandleBackupUpload},
		{http.MethodGet, "/api/system-containers/backups/internal/download/{grantId}", true, (*Handler).HandleBackupDownload},
	}

	for _, route := range routes {
		// Resolved per request rather than bound here.
		//
		// The loader calls RegisterRoutes *before* Init, so `p.handler` is
		// still nil at this point. Writing `p.handler.HandleList` would take a
		// method value bound to that nil receiver and keep it forever — the
		// route registers cleanly, the plugin loads cleanly, and every request
		// panics on the first field access. Taking a method expression and
		// supplying the receiver at request time is what makes the handler the
		// one Init actually built.
		fn := route.handler
		serve := func(w http.ResponseWriter, r *http.Request) {
			handler := p.handler
			if handler == nil {
				// Reachable only if a request arrives between RegisterRoutes
				// and Init. Answered honestly rather than panicked.
				writeJSON(w, http.StatusServiceUnavailable,
					failEnvelope(r, codeNotReady, "", nil))
				return
			}
			fn(handler, w, r)
		}
		if route.public {
			reg.Public(route.method, route.path, serve)
			continue
		}
		reg.Protected(route.method, route.path, serve)
	}
	return nil
}

// RegisterMenu adds this plugin's sidebar entries.
//
// The manifest's `contributes.menu` block is declarative metadata; the sidebar
// is built from what is registered here, so a plugin that skips this call
// installs correctly and contributes no navigation at all.
//
// All three compute plugins share the parent section "compute", which is what
// puts virtual machines and system containers under one heading beside the
// pools and networks they run on. The section label resolves as
// `<LabelNS>:nav.section.compute` from whichever plugin's entry the sidebar
// sees first, so every one of the three defines that key with the same text.
//
// Each entry names the permission it needs and the host hides one the user
// cannot use. Navigation is the documented exception to disabled-not-hidden,
// because a dead-end page teaches nobody anything.
func (p *LXCPlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	items := []plugin.MenuItem{
		{
			Parent: "compute", LabelNS: "ct", LabelKey: "nav.containers",
			Path: "/plugins/addon-lxc", Icon: "IconBox", Required: "ct.view",
			Priority: 85,
		},
		{
			Parent: "compute", LabelNS: "ct", LabelKey: "nav.templates",
			Path: "/plugins/addon-lxc/templates", Icon: "IconTemplate", Required: "ct.view",
			Priority: 45,
		},
	}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return fmt.Errorf("lxc: register menu %s: %w", item.LabelKey, err)
		}
	}
	return nil
}

// RegisterWidgets contributes the dashboard tiles.
func (p *LXCPlugin) RegisterWidgets(reg plugin.WidgetRegistry) error {
	for _, widget := range []struct {
		key, camel, icon string
		width, height    int
		minW, minH       int
	}{
		{key: "ct-summary", camel: "ctSummary", icon: "IconBox",
			width: 4, height: 3, minW: 3, minH: 2},
		// Density rather than plain allocation: the interesting number for
		// system containers is how much more the host is carrying than its
		// memory would suggest, because they share page cache and a kernel.
		// Showing that is what makes the case for using them at all.
		{key: "ct-density", camel: "ctDensity", icon: "IconStack2",
			width: 4, height: 3, minW: 3, minH: 2},
	} {
		if err := reg.Add(widget.key, plugin.WidgetDescriptor{
			Title:  "ct:widgets." + widget.camel + ".label",
			Path:   "/widgets/" + widget.key,
			Width:  widget.width,
			Height: widget.height,
			Settings: map[string]any{
				"description": "ct:widgets." + widget.camel + ".description",
				"icon":        widget.icon,
				"min_width":   widget.minW,
				"min_height":  widget.minH,
			},
		}); err != nil {
			return fmt.Errorf("lxc: register widget %s: %w", widget.key, err)
		}
	}
	return nil
}

// RegisterNodes contributes the container workflow nodes. See nodes.go.
func (p *LXCPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	return p.registerNodes(reg)
}

// RegisterAdapters contributes this plugin's runtime adapters.
//
// Optional half of the plugin contract: the loader calls it when the symbol
// implements it and skips it otherwise. Neither family costs anything when
// unused — an operator with no container alert rules is never asked to evaluate
// one, and the report types are invisible until the Reporting plugin is
// installed.
func (p *LXCPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Reporting().Register(lxcReportSpec(), &reportProvider{plugin: p}); err != nil {
		return fmt.Errorf("lxc: register report provider: %w", err)
	}
	if err := reg.Alerting().Register(lxcAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("lxc: register alert source: %w", err)
	}
	if err := reg.Backup().Register(lxcBackupSpec(), &backupProvider{plugin: p}); err != nil {
		return fmt.Errorf("lxc: register backup provider: %w", err)
	}
	return nil
}

// RegisterPermissions declares the plugin's RBAC keys.
//
// Routes are gated from the manifest regardless; registering is what puts the
// keys within reach of an operator. A plugin that skips this call is one only a
// wildcard role can use — the keys exist but appear in no role editor.
func (p *LXCPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("lxc: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}
