// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor Virtual Machines.
//
// KVM guests on your own hosts: create from a cloud image and reach a login
// prompt in the browser in under two minutes, or install from an ISO. Disks,
// interfaces, snapshots, clones and templates, with a VNC and serial console.
//
// # What this plugin does not contain
//
// No libvirt, no Proxmox, no third-party hypervisor UI. McHarbor drives
// `qemu-system-*` directly over QMP and supervises it with systemd transient
// units. The reasoning, in short: libvirt is a daemon rather than a library, it
// keeps its own domain definitions and storage pools that then drift from
// McHarbor's, and everything needed from a running guest is a QMP command
// anyway — so talking to QMP directly removes a translation layer, not a safety
// layer.
//
// # Where the work actually happens
//
// Not here. The McHarbor process cannot host guests: a `docker compose up -d`
// would kill every running VM, taps created in its network namespace would sit
// behind Docker's NAT rather than on the LAN, and the privileges required
// amount to `privileged: true` plus host networking. So the executor lives in
// the agent, and this plugin reaches it through the host agent-rpc bridge —
// a generic Call and Subscribe pair it asserts locally, so no host type is linked.
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
// guest not start?" — and nothing here should be able to do more with the
// router than that.
type aiCompleter interface {
	Complete(ctx context.Context, req plugin.AIRequest) (plugin.AIResponse, error)
	Available() bool
}

// KVMPlugin is the plugin instance the host loads.
type KVMPlugin struct {
	manifest  plugin.Manifest
	host      plugin.HostContext
	svc       *Service
	handler   *Handler
	logger    pluginLogger
	events    *guestEventWorker
	recon     *reconciler
	health    *healthPoller
	metrics   *metricsSampler
	scheduler *snapshotScheduler
	backups   *backupScheduler
}

// Plugin is the symbol the host looks up after dlopen.
var Plugin plugin.Plugin = &KVMPlugin{manifest: manifest}

var _ plugin.Plugin = (*KVMPlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-kvm",
	Name:     "Virtual Machines",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Run KVM virtual machines on your own hosts, with disks, networking, " +
		"snapshots, clones, templates and a browser console.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks this
	// slice, so the two lists cannot drift apart.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key:      "vm.view",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows seeing virtual machines, their hardware, their snapshots " +
				"and their resource usage.",
		},
		{
			Key:      "vm.manage",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows creating, editing, starting, stopping, resizing, " +
				"snapshotting and cloning virtual machines. Destroying one needs vm.delete.",
		},
		{
			Key:      "vm.delete",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows destroying a virtual machine and its disks, and deleting " +
				"its snapshots.",
		},
		{
			// Separate from manage because reaching a console is effectively
			// root inside the guest, and plenty of operators want to grant
			// start/stop without it — the same reasoning that separates
			// containers.manage from exec.
			Key:      "vm.console",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows opening the VNC or serial console of a virtual machine. " +
				"This is effectively root access inside the guest, which is why it is " +
				"separate from vm.manage.",
		},
		{
			// Separate from manage because a migration touches two hosts and
			// moves a guest's disks and memory across the network; it is
			// checked on both the guest's host and the target.
			Key:      "vm.migrate",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows moving a virtual machine to another host, stopped or live. " +
				"Needed on both the host it leaves and the host it moves to.",
		},
		{
			// Import is separate from manage for the reason upload is separate
			// from download in the compute plugin: it is a different
			// authority, not a bigger one. Creating a guest from the catalog
			// uses images McHarbor fetched and verified; an import puts a disk
			// of the operator's choosing on the host and boots it.
			Key:      "vm.import",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows uploading a disk or an OVA from another hypervisor and " +
				"converting it into a virtual machine. Creating a guest from the image " +
				"catalog only needs vm.manage.",
		},
		{
			Key:      "vm.backup.view",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows seeing backup plans, backup history and where each " +
				"backup was written.",
		},
		{
			Key:      "vm.backup.manage",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows creating and editing backup schedules and starting a " +
				"backup. Removing one needs vm.backup.delete, and restoring needs " +
				"vm.restore.run.",
		},
		{
			Key:      "vm.backup.delete",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows removing a backup and the copies it wrote to its " +
				"destinations.",
		},
		{
			// Separate from backup.manage the way restore.run is separate from
			// backups.manage for containers: restoring replaces a machine's
			// disks, which is destructive in a way taking a backup is not.
			Key:      "vm.restore.run",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows rebuilding a machine's disks from a backup, replacing " +
				"whatever is on them now. The machine has to be stopped.",
		},
		{
			// Never in a role preset. Each of these can compromise the host
			// itself rather than only the guest.
			Key:      "vm.privileged",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows PCI and USB passthrough and attaching raw host block " +
				"devices. These can compromise the host itself, not just the guest, so " +
				"this is never granted by a role preset.",
		},
	},
}

// Manifest returns the plugin's manifest.
func (p *KVMPlugin) Manifest() plugin.Manifest { return p.manifest }

// Init wires the plugin to the host.
func (p *KVMPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host
	p.logger = host.Logger

	if newAgentBridge(host.AgentRPC) == nil {
		// Not fatal: the list still renders and explains that no host can run
		// guests, which is more useful than the section refusing to load.
		p.logger.Warn("virtual machines: the host wired no agent bridge; " +
			"guests will be read-only")
	}

	p.svc = NewService(host.DB, host.AgentRPC, host.Audit, host.Lifecycle,
		host.BackupLog, host.Environments, host.Storage, host.WorkDir, p.logger)
	p.svc.setWorkflowTrigger(host.Workflows)
	p.svc.setVault(host.Secrets)

	// Created here rather than left to the migration: a plugin's migrations run
	// once, at install, so 0002 and 0003 never reach an installation that
	// already has this plugin. Every statement is idempotent and names this
	// plugin's own prefix.
	if err := p.svc.ensureScheduleSchema(ctx); err != nil {
		return fmt.Errorf("kvm: preparing the snapshot schedule tables: %w", err)
	}
	if err := p.svc.ensureImportSchema(ctx); err != nil {
		return fmt.Errorf("kvm: preparing the import table: %w", err)
	}
	if err := p.svc.ensureNICSchema(ctx); err != nil {
		return fmt.Errorf("kvm: preparing the interface columns: %w", err)
	}
	// After the schedule tables, because it adds a column to one of them.
	if err := p.svc.ensureSnapshotTreeSchema(ctx); err != nil {
		return fmt.Errorf("kvm: preparing the snapshot tree columns: %w", err)
	}
	// Fatal: every guest read selects display_profile_id.
	if err := p.svc.ensureDisplayProfileSchema(ctx); err != nil {
		return fmt.Errorf("kvm: preparing the display profiles: %w", err)
	}
	// Not fatal: without these tables the plugin still manages guests, and the
	// incident scan stays silent about stops rather than calling every one unexpected.
	if err := p.svc.ensureIncidentSchema(ctx); err != nil {
		p.logger.Warn("kvm: preparing the incident history tables failed", "error", err)
	}
	if err := p.svc.ensureGuestHealthSchema(ctx); err != nil {
		p.logger.Warn("kvm: preparing the guest health table failed", "error", err)
	}
	// Not fatal either: without the table the detail charts start empty, as
	// they did before history was stored, and everything else still works.
	if err := p.svc.ensureVMMetricsSchema(ctx); err != nil {
		p.logger.Warn("kvm: preparing the guest metrics table failed", "error", err)
	}

	// An inspect or convert lives in memory, so a restart ends the run without
	// ending the row. Reconciling here is what stops an interrupted import
	// from saying "inspecting" for the rest of the install's life.
	if err := p.svc.reconcileImports(ctx); err != nil {
		p.logger.Warn("kvm: reconciling interrupted imports failed", "error", err)
	}

	// Same reasoning for a backup: its progress lives in a goroutine, so a
	// reload leaves the row saying "running" with nothing behind it.
	if err := p.svc.reapOrphanedBackupRuns(ctx); err != nil {
		p.logger.Warn("kvm: reaping interrupted backup runs failed", "error", err)
	}

	// The AI handle is optional. A nil one means the diagnose endpoint answers
	// "not available" rather than failing.
	var ai aiCompleter
	if host.AI != nil {
		ai = host.AI
	}
	p.handler = NewHandler(p.svc, host.Audit, ai, p.logger)
	p.handler.rbac = host.RBAC

	// Give every declared key a label from this plugin's i18n namespace.
	// Without this the role editor lists the raw key, which tells an
	// administrator granting `vm.privileged` nothing about what they are about
	// to hand over. The host removes these again on Shutdown.
	if host.RBAC != nil {
		for _, decl := range p.manifest.AddsPerms {
			host.RBAC.RegisterLabel(decl.Key, "vm", "permissions."+decl.Key+".label")
		}
	}

	// Live state. The agent already pushes every QMP lifecycle event and the
	// host already routes them; nothing subscribed, so they were discarded and
	// a guest's state only ever moved when somebody clicked something. That is
	// what left a finished shutdown reading "shutting down" indefinitely.
	if p.svc.rpc != nil {
		p.events = newGuestEventWorker(p.svc, p.logger)
		p.events.Start()
		p.svc.rpc.Subscribe(p.events.Enqueue)

		// The backstop behind those events. Service.Reconcile existed and was
		// never called, so nothing ever corrected state an event could not
		// carry — an agent restarting mid-transition, say.
		p.recon = newReconciler(p.svc, p.logger)
		p.recon.Start()

		// Guest health through qemu-ga, feeding the incident signals. Its own
		// loop so a guest agent that stalls a query cannot delay reconciling.
		p.health = newHealthPoller(p.svc, p.logger)
		p.health.Start()

		// Guest metrics history, so a detail page opens on the last hour
		// instead of a chart that only fills while the page stays open.
		p.metrics = newMetricsSampler(p.svc, p.logger)
		p.metrics.Start()

		// Scheduled snapshots. Behind the agent-bridge check with the rest:
		// without a bridge every firing would fail, and a schedule that logs a
		// failure a minute forever is worse than one that does not run.
		p.scheduler = newSnapshotScheduler(p.svc, p.logger)
		p.scheduler.Start()

		// Its own loop rather than folded into the snapshot scheduler: the
		// two have different retention rules and different failure modes, and
		// sharing a loop would let a stuck backup hold up every snapshot.
		p.backups = newBackupScheduler(p.svc, p.logger)
		go p.backups.Start(context.WithoutCancel(ctx))
	}

	p.logger.Info("virtual machines ready")
	return nil
}

// Shutdown releases the plugin.
func (p *KVMPlugin) Shutdown(_ context.Context) error {
	if p.svc != nil && p.svc.rpc != nil {
		p.svc.rpc.Unsubscribe()
	}
	if p.health != nil {
		p.health.Stop()
	}
	if p.metrics != nil {
		p.metrics.Stop()
	}
	if p.scheduler != nil {
		p.scheduler.Stop()
	}
	if p.backups != nil {
		p.backups.Stop()
	}
	if p.recon != nil {
		p.recon.Stop()
	}
	if p.events != nil {
		p.events.Stop()
	}
	return nil
}

// RegisterRoutes registers the plugin's HTTP surface.
//
// Every route is Protected and its permission is declared in plugin.yaml — the
// host's plugin route registry wraps them in its permission middleware, so
// declaring correctly is enforcing.
func (p *KVMPlugin) RegisterRoutes(reg plugin.Registry) error {
	routes := []struct {
		method string
		path   string
		// public routes mount outside the session middleware. Exactly one
		// does, and it authenticates itself — see HandleImportPull.
		public  bool
		handler func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{http.MethodGet, "/api/vms/capability", false, (*Handler).HandleQEMUCapability},
		// The host reboot's drain: literal paths, registered before /{id} like capability.
		{http.MethodGet, "/api/vms/reboot-drain", false, (*Handler).HandleRebootDrainRunning},
		{http.MethodPost, "/api/vms/reboot-drain", false, (*Handler).HandleRebootDrainStop},

		{http.MethodGet, "/api/vms/display-profiles", false, (*Handler).HandleListDisplayProfiles},
		{http.MethodPost, "/api/vms/display-profiles", false, (*Handler).HandleCreateDisplayProfile},
		{http.MethodGet, "/api/vms/display-profiles/{profileId}", false, (*Handler).HandleGetDisplayProfile},
		{http.MethodPut, "/api/vms/display-profiles/{profileId}", false, (*Handler).HandleUpdateDisplayProfile},
		{http.MethodPost, "/api/vms/display-profiles/{profileId}/duplicate", false, (*Handler).HandleDuplicateDisplayProfile},
		{http.MethodDelete, "/api/vms/display-profiles/{profileId}", false, (*Handler).HandleDeleteDisplayProfile},

		{http.MethodGet, "/api/vms/{id}/backup-plans", false, (*Handler).HandleListBackupPlans},
		{http.MethodPost, "/api/vms/{id}/backup-plans", false, (*Handler).HandleCreateBackupPlan},
		{http.MethodPut, "/api/vms/backup-plans/{planId}", false, (*Handler).HandleUpdateBackupPlan},
		{http.MethodDelete, "/api/vms/backup-plans/{planId}", false, (*Handler).HandleDeleteBackupPlan},
		{http.MethodGet, "/api/vms/{id}/backups", false, (*Handler).HandleListBackupRuns},
		{http.MethodPost, "/api/vms/{id}/backups", false, (*Handler).HandleRunBackup},
		{http.MethodGet, "/api/vms/backups/{runId}", false, (*Handler).HandleBackupRun},
		{http.MethodDelete, "/api/vms/backups/{runId}", false, (*Handler).HandleDeleteBackupRun},
		{http.MethodPost, "/api/vms/backups/{runId}/restore", false, (*Handler).HandleRestoreBackup},
		{http.MethodGet, "/api/vms/{id}/block-jobs", false, (*Handler).HandleBlockJobs},
		{http.MethodPost, "/api/vms/{id}/guest-info", false, (*Handler).HandleRefreshGuest},

		{http.MethodPost, "/api/vms/{id}/memory", false, (*Handler).HandleResizeMemory},
		{http.MethodPost, "/api/vms/{id}/vcpus", false, (*Handler).HandleSetVCPUs},
		{http.MethodPut, "/api/vms/{id}/disks/{diskId}/throttle", false, (*Handler).HandleSetThrottle},
		{http.MethodPost, "/api/vms/{id}/disks/{diskId}/inspect", false, (*Handler).HandleInspectImage},

		{http.MethodGet, "/api/vms/share-roots", false, (*Handler).HandleGetShareRoots},
		{http.MethodPut, "/api/vms/share-roots", false, (*Handler).HandleSetShareRoots},
		{http.MethodGet, "/api/vms/{id}/shares", false, (*Handler).HandleListShares},
		{http.MethodPost, "/api/vms/{id}/shares", false, (*Handler).HandleAddShare},
		{http.MethodDelete, "/api/vms/{id}/shares/{shareId}", false, (*Handler).HandleRemoveShare},
		{http.MethodGet, "/api/vms", false, (*Handler).HandleList},
		{http.MethodPost, "/api/vms", false, (*Handler).HandleCreate},
		{http.MethodGet, "/api/vms/{id}", false, (*Handler).HandleGet},
		{http.MethodGet, "/api/vms/{id}/cloud-init", false, (*Handler).HandleCloudInit},
		{http.MethodPut, "/api/vms/{id}", false, (*Handler).HandleUpdate},
		{http.MethodDelete, "/api/vms/{id}", false, (*Handler).HandleDelete},
		{http.MethodGet, "/api/vms/{id}/stats", false, (*Handler).HandleStats},
		{http.MethodGet, "/api/vms/{id}/stats/history", false, (*Handler).HandleStatsHistory},
		{http.MethodGet, "/api/vms/{id}/logs", false, (*Handler).HandleGuestLog},

		{http.MethodPost, "/api/vms/{id}/start", false, (*Handler).HandleStart},
		{http.MethodPost, "/api/vms/{id}/shutdown", false, (*Handler).HandleShutdown},
		{http.MethodPost, "/api/vms/{id}/stop", false, (*Handler).HandleStop},
		{http.MethodPost, "/api/vms/{id}/reset", false, (*Handler).HandleReset},
		{http.MethodPost, "/api/vms/{id}/pause", false, (*Handler).HandlePause},
		{http.MethodPost, "/api/vms/{id}/resume", false, (*Handler).HandleResume},

		{http.MethodGet, "/api/vms/{id}/disks", false, (*Handler).HandleListDisks},
		{http.MethodPost, "/api/vms/{id}/disks", false, (*Handler).HandleAddDisk},
		{http.MethodPatch, "/api/vms/{id}/disks/{diskId}", false, (*Handler).HandleResizeDisk},
		{http.MethodDelete, "/api/vms/{id}/disks/{diskId}", false, (*Handler).HandleDeleteDisk},

		{http.MethodPost, "/api/vms/{id}/media", false, (*Handler).HandleAttachMedia},
		{http.MethodPost, "/api/vms/{id}/guest-agent-disc", false, (*Handler).HandleAttachGuestAgentDisc},
		{http.MethodDelete, "/api/vms/{id}/media/{diskId}", false, (*Handler).HandleEjectMedia},

		{http.MethodGet, "/api/vms/{id}/nics", false, (*Handler).HandleListNICs},
		{http.MethodPost, "/api/vms/{id}/nics", false, (*Handler).HandleAddNIC},
		{http.MethodPatch, "/api/vms/{id}/nics/{nicId}", false, (*Handler).HandleUpdateNIC},
		{http.MethodDelete, "/api/vms/{id}/nics/{nicId}", false, (*Handler).HandleDeleteNIC},

		// Registered before /{id}/snapshots so the literal path is not eaten by
		// the id parameter.
		{http.MethodGet, "/api/vms/snapshots/summary", false, (*Handler).HandleSnapshotSummary},
		{http.MethodGet, "/api/vms/{id}/snapshots", false, (*Handler).HandleListSnapshots},
		{http.MethodPost, "/api/vms/{id}/snapshots", false, (*Handler).HandleCreateSnapshot},
		{http.MethodPost, "/api/vms/{id}/snapshots/{snapId}/revert", false, (*Handler).HandleRevertSnapshot},
		{http.MethodDelete, "/api/vms/{id}/snapshots/{snapId}", false, (*Handler).HandleDeleteSnapshot},

		{http.MethodGet, "/api/vms/{id}/snapshot-schedules", false, (*Handler).HandleListSchedules},
		{http.MethodPost, "/api/vms/{id}/snapshot-schedules", false, (*Handler).HandleCreateSchedule},
		{http.MethodPut, "/api/vms/{id}/snapshot-schedules/{scheduleId}", false, (*Handler).HandleUpdateSchedule},
		{http.MethodPost, "/api/vms/{id}/snapshot-schedules/{scheduleId}/run", false, (*Handler).HandleRunSchedule},
		{http.MethodDelete, "/api/vms/{id}/snapshot-schedules/{scheduleId}", false, (*Handler).HandleDeleteSchedule},

		{http.MethodPost, "/api/vms/{id}/clone", false, (*Handler).HandleClone},
		{http.MethodPost, "/api/vms/{id}/template", false, (*Handler).HandleTemplate},

		// Migration between hosts, and its relay. The relay routes are public
		// because the callers are agents with no session; each redeems a
		// one-time token for one direction of one disk.
		{http.MethodPost, "/api/vms/{id}/migrate", false, (*Handler).HandleMigrate},
		{http.MethodGet, "/api/vms/{id}/migration", false, (*Handler).HandleMigrationStatus},
		{http.MethodPost, "/api/vms/{id}/migration/cancel", false, (*Handler).HandleCancelMigration},
		{http.MethodPut, "/api/vms/migrations/internal/{grantId}", true, (*Handler).HandleMigrationUpload},
		{http.MethodGet, "/api/vms/migrations/internal/{grantId}", true, (*Handler).HandleMigrationDownload},

		// The remote-viewer file for a SPICE console.
		{http.MethodGet, "/api/vms/{id}/console.vv", false, (*Handler).HandleSpiceFile},

		{http.MethodGet, "/api/vms/pool-volumes", false, (*Handler).HandlePoolVolumes},

		{http.MethodGet, "/api/vms/host-devices", false, (*Handler).HandleHostDevices},
		{http.MethodPost, "/api/vms/{id}/devices", false, (*Handler).HandleAddDevice},
		{http.MethodDelete, "/api/vms/{id}/devices/{devId}", false, (*Handler).HandleDeleteDevice},

		{http.MethodPost, "/api/vms/{id}/diagnose", false, (*Handler).HandleDiagnose},

		// Imports. The literal segments come before "/api/vms/{id}" would
		// match them, so chi does not read "imports" as a guest id.
		{http.MethodGet, "/api/vms/imports", false, (*Handler).HandleListImports},
		{http.MethodPost, "/api/vms/imports", false, (*Handler).HandleUploadImport},
		{http.MethodGet, "/api/vms/imports/internal/{grantId}", true, (*Handler).HandleImportPull},
		// Public for the same reason the import pull is: the caller is an
		// agent with no session, and each redeems a one-shot grant McHarbor
		// minted moments earlier.
		{http.MethodPut, "/api/vms/backups/internal/{grantId}", true, (*Handler).HandleBackupUpload},
		{http.MethodGet, "/api/vms/backups/internal/download/{grantId}", true, (*Handler).HandleBackupDownload},
		{http.MethodPost, "/api/vms/imports/{importId}/inspect", false, (*Handler).HandleInspectImport},
		{http.MethodPost, "/api/vms/imports/{importId}/convert", false, (*Handler).HandleConvertImport},
		{http.MethodPost, "/api/vms/imports/{importId}/cancel", false, (*Handler).HandleCancelImport},
		{http.MethodDelete, "/api/vms/imports/{importId}", false, (*Handler).HandleDeleteImport},
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
		mount := reg.Protected
		if route.public {
			mount = reg.Public
		}
		mount(route.method, route.path, func(w http.ResponseWriter, r *http.Request) {
			handler := p.handler
			if handler == nil {
				// Reachable only if a request arrives between RegisterRoutes
				// and Init. Answered honestly rather than panicked.
				writeJSON(w, http.StatusServiceUnavailable,
					failEnvelope(r, codeNotReady, "", nil))
				return
			}
			fn(handler, w, r)
		})
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
func (p *KVMPlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	items := []plugin.MenuItem{
		// The Compute section is ordered explicitly across all three plugins:
		// Overview 100, machines 90, containers 85, images 80, pools 70,
		// networks 60, then the template pages. Without a priority the order
		// inside a section is alphabetical by plugin slug.
		{
			Parent: "compute", LabelNS: "vm", LabelKey: "nav.machines",
			Path: "/plugins/addon-kvm", Icon: "IconDeviceDesktop", Required: "vm.view",
			Priority: 90,
		},
		{
			Parent: "compute", LabelNS: "vm", LabelKey: "nav.templates",
			Path: "/plugins/addon-kvm/templates", Icon: "IconTemplate", Required: "vm.view",
			Priority: 50,
		},
	}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return fmt.Errorf("kvm: register menu %s: %w", item.LabelKey, err)
		}
	}
	return nil
}

// RegisterWidgets contributes the dashboard tiles.
func (p *KVMPlugin) RegisterWidgets(reg plugin.WidgetRegistry) error {
	for _, widget := range []struct {
		key, camel, icon string
		width, height    int
		minW, minH       int
	}{
		{key: "vm-summary", camel: "vmSummary", icon: "IconDeviceDesktop",
			width: 4, height: 3, minW: 3, minH: 2},
		// Allocation against host capacity, because overcommitting vCPUs and
		// memory across guests is normal and the point at which it stops being
		// fine is worth seeing before a guest fails to start.
		{key: "vm-resources", camel: "vmResources", icon: "IconCpu",
			width: 4, height: 3, minW: 3, minH: 2},
		// Snapshot coverage rather than snapshot count: a guest with none is
		// the one that costs a rebuild, and nothing else on the dashboard
		// says which guests those are.
		{key: "vm-protection", camel: "vmProtection", icon: "IconShieldOff",
			width: 4, height: 3, minW: 3, minH: 2},
		{key: "vm-imports", camel: "vmImports", icon: "IconFileImport",
			width: 4, height: 3, minW: 3, minH: 2},
	} {
		if err := reg.Add(widget.key, plugin.WidgetDescriptor{
			Title:  "vm:widgets." + widget.camel + ".label",
			Path:   "/widgets/" + widget.key,
			Width:  widget.width,
			Height: widget.height,
			Settings: map[string]any{
				"description": "vm:widgets." + widget.camel + ".description",
				"icon":        widget.icon,
				"min_width":   widget.minW,
				"min_height":  widget.minH,
			},
		}); err != nil {
			return fmt.Errorf("kvm: register widget %s: %w", widget.key, err)
		}
	}
	return nil
}

// RegisterNodes contributes the guest lifecycle workflow nodes. See nodes.go.
func (p *KVMPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	return p.registerNodes(reg)
}

// RegisterAdapters contributes this plugin's runtime adapters.
//
// Optional half of the plugin contract: the loader calls it when the symbol
// implements it and skips it otherwise. Neither family costs anything when
// unused — an operator with no VM alert rules is never asked to evaluate one,
// and the report types are invisible until the Reporting plugin is installed.
func (p *KVMPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Reporting().Register(kvmReportSpec(), &reportProvider{plugin: p}); err != nil {
		return fmt.Errorf("kvm: register report provider: %w", err)
	}
	if err := reg.Alerting().Register(kvmAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("kvm: register alert source: %w", err)
	}
	if err := reg.Backup().Register(kvmBackupSpec(), &backupProvider{plugin: p}); err != nil {
		return fmt.Errorf("kvm: register backup provider: %w", err)
	}
	return nil
}

// RegisterPermissions declares the plugin's RBAC keys.
//
// Routes are gated from the manifest regardless; registering is what puts the
// keys within reach of an operator. A plugin that skips this call is one only a
// wildcard role can use — the keys exist but appear in no role editor.
func (p *KVMPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("kvm: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}
