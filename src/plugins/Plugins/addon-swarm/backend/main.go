// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor Docker Swarm.
//
// SwarmKit — services, tasks, nodes, secrets, configs, swarm stacks and
// cluster lifecycle — as an installable add-on rather than part of the host.
//
// This used to be core: `src/backend/modules/swarm` (5,600 lines),
// `src/frontend/modules/swarm` (5,200 lines), twenty RBAC keys in the host
// catalog, sixty-nine message codes in `core/i18n`, ten routes in the host
// router and a four-group sidebar section. An installation that runs a single
// Docker host now carries none of it.
//
// Unlike the Kubernetes add-on, no dependency leaves the host binary: Swarm is
// the same Docker SDK, talking to the same daemon, over the same client pool.
// That is what makes this migration a clean one — there is no schema to hand
// over, no credential to re-encrypt and no environment row to rewrite. Swarm
// was never a column; it is a question the plugin asks a daemon the host has
// already connected to (`capability.go`).
//
// The twenty permission keys keep their exact strings. A plugin cannot shadow
// a host key, so host migration 094 deletes the catalog rows before these can
// land — but the *grants* in `roles.permissions`, `role_presets.permissions`
// and `user_permissions` are deliberately left alone. They name the same keys
// this plugin re-registers, so an operator who had granted `swarm_nodes.manage`
// before the upgrade still has it after. (The Kubernetes migration dropped its
// grants because it also renamed ten keys down to four. Nothing is renamed
// here, so nothing needs dropping.)
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginLogger is the host logger's shape. Declared here rather than taken as
// *slog.Logger — which is what the core module used — because the plugin
// contract hands over an interface, not the concrete logger.
type pluginLogger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// dockerPool is the slice of the host's client pool this plugin needs: one
// method, called from one place (`Service.getClient`).
//
// `Get` returns `any` rather than `*client.Client` because the plugin SDK
// deliberately does not import the Docker SDK — see
// `core/plugin/services.DockerPoolAccessor`. The assertion happens in
// getClient.
type dockerPool interface {
	Get(envID string) (any, error)
}

// auditSink is the host's audit log, narrowed to the one method the handlers
// use. Matches plugin.AuditHandle; declared separately so the handler file
// does not have to import the SDK for a single interface.
type auditSink interface {
	Emit(event string, fields map[string]any) error
}

// SwarmPlugin is the plugin instance the host loads.
type SwarmPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	handler  *Handler
	// watcher feeds the swarm-event trigger; nil until Init and after
	// Shutdown, or when the host wired no workflow bridge.
	watcher *swarmWatcher
}

// Plugin is the symbol the host looks up after dlopen.
//
// The handler is built here rather than in Init because the loader calls
// RegisterRoutes *before* Init: a handler created later would be a second
// object, and the method values the routes captured would still point at the
// first one — dependency-less and permanently broken. So the handler is a
// stable object from the start and Init attaches the host to it.
var Plugin plugin.Plugin = &SwarmPlugin{manifest: manifest, handler: &Handler{}}

var _ plugin.Plugin = (*SwarmPlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-swarm",
	Name:     "Docker Swarm",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Manage Docker Swarm from McHarbor: services, tasks, nodes, secrets, configs, " +
		"swarm stacks and overlay networks — with cluster init, join, leave, join-token " +
		"and unlock-key lifecycle, service scaling, rolling restarts and rollback.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks this
	// slice, so the two lists cannot drift apart.
	//
	// The twenty keys are the host's, unchanged. Splitting them further, or
	// collapsing them the way the Kubernetes add-on collapsed its ten into
	// four, would have been a second decision riding along on a migration —
	// and unlike Kubernetes, this split is already enforceable: each resource
	// has its own list endpoint, so a role with `swarm_nodes.view` and without
	// `swarm_secrets.view` genuinely receives nodes and no secrets.
	//
	// Scopes are the host catalog's, carried across unchanged. `env` for
	// almost everything — Swarm is a property of one Docker environment's
	// daemon, so a grant that did not name an environment would be a grant
	// over every daemon the installation can reach. The two exceptions are
	// the host's and are kept rather than tidied: `swarm.manage` is `global`
	// because init/join/leave reconfigures the host's cluster membership
	// rather than one environment's workloads, and `swarm_stacks.manage` is
	// `stack` so a grant can name the stacks it covers.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key: "swarm.view", Scope: "env", Category: "Plugins",
			Description: "Allows viewing the cluster: its state, the local node's role, and the overview.",
		},
		{
			// The heaviest key in the set. Init, join and leave change what
			// the daemon *is*; join tokens and the unlock key are credentials
			// that let a stranger's host into the cluster. They share a key
			// because they share a blast radius.
			Key: "swarm.manage", Scope: "global", Category: "Plugins",
			Description: "Allows cluster lifecycle: initialise, join, leave, update settings, and read or rotate join tokens and the unlock key.",
		},
		{
			Key: "swarm_services.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm services, including their logs.",
		},
		{
			Key: "swarm_services.manage", Scope: "env", Category: "Plugins",
			Description: "Allows creating and updating Swarm services, and triggering a restart or rollback.",
		},
		{
			// Scale is its own key, not part of manage: a replica bump is the
			// routine operation an on-call engineer needs at 3am, and it must
			// not require the ability to replace the service's image.
			Key: "swarm_services.scale", Scope: "env", Category: "Plugins",
			Description: "Allows changing a service's replica count. Separate from manage so scaling does not imply the ability to replace the image.",
		},
		{
			Key: "swarm_services.delete", Scope: "env", Category: "Plugins",
			Description: "Allows removing Swarm services.",
		},
		{
			Key: "swarm_tasks.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm tasks, including their logs.",
		},
		{
			Key: "swarm_nodes.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm nodes.",
		},
		{
			Key: "swarm_nodes.manage", Scope: "env", Category: "Plugins",
			Description: "Allows editing node labels, availability (active, pause, drain) and role (manager, worker).",
		},
		{
			Key: "swarm_nodes.delete", Scope: "env", Category: "Plugins",
			Description: "Allows removing a node from the cluster.",
		},
		{
			// A Swarm secret's *payload* is write-only in the Docker API, so
			// `view` here is metadata: names, labels, which services mount it.
			Key: "swarm_secrets.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm secrets. Docker never returns a secret's payload, so this is metadata only.",
		},
		{
			Key: "swarm_secrets.manage", Scope: "env", Category: "Plugins",
			Description: "Allows creating Swarm secrets and updating their labels.",
		},
		{
			Key: "swarm_secrets.delete", Scope: "env", Category: "Plugins",
			Description: "Allows removing Swarm secrets.",
		},
		{
			// Configs differ from secrets in exactly one way that matters
			// here: Docker *does* return a config's payload, so `view` on a
			// config is a genuine read of its contents.
			Key: "swarm_configs.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm configs, including their contents.",
		},
		{
			Key: "swarm_configs.manage", Scope: "env", Category: "Plugins",
			Description: "Allows creating Swarm configs and updating their labels.",
		},
		{
			Key: "swarm_configs.delete", Scope: "env", Category: "Plugins",
			Description: "Allows removing Swarm configs.",
		},
		{
			Key: "swarm_stacks.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing and inspecting Swarm stacks.",
		},
		{
			Key: "swarm_stacks.manage", Scope: "stack", Category: "Plugins",
			Description: "Allows deploying a Compose file as a Swarm stack.",
		},
		{
			Key: "swarm_stacks.delete", Scope: "env", Category: "Plugins",
			Description: "Allows removing a Swarm stack and everything it deployed.",
		},
		{
			Key: "swarm_networks.view", Scope: "env", Category: "Plugins",
			Description: "Allows listing the cluster's overlay networks.",
		},
	},
}

func (p *SwarmPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes mounts the forty-six routes the host module carried.
//
// Paths are unchanged — `/api/swarm/...`, exactly as before — which is why the
// frontend port did not have to touch a single request. The permission gate
// moved, though: it was `rbac.RequirePermission(...)` wrapped around each
// route in the module's own `routes.go`, and it is now the
// `requires_permission` field on the matching entry in plugin.yaml. The host
// applies it before the request reaches this process, so a handler here can
// still assume it has been authorised — the same assumption the module made.
//
// `GET /api/swarm/capability` is the one route with no permission. It returns
// a boolean and the local node's role, leaks nothing about workloads, and the
// UI needs it before it can know whether to show a cluster or the "initialise"
// empty state. It is `Protected` rather than `Auth` so it still requires a
// session — "is this a Swarm?" is not a question for the logged-out.
func (p *SwarmPlugin) RegisterRoutes(reg plugin.Registry) error {
	h := p.handler

	// --- cluster ---------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/capability", h.HandleCapability)
	reg.Protected(http.MethodGet, "/api/swarm", h.HandleInspect)
	reg.Protected(http.MethodGet, "/api/swarm/overview", h.HandleOverview)
	reg.Protected(http.MethodPut, "/api/swarm", h.HandleUpdate)
	reg.Protected(http.MethodPost, "/api/swarm/init", h.HandleInit)
	reg.Protected(http.MethodPost, "/api/swarm/join", h.HandleJoin)
	reg.Protected(http.MethodPost, "/api/swarm/leave", h.HandleLeave)
	reg.Protected(http.MethodGet, "/api/swarm/join-tokens", h.HandleJoinTokens)
	reg.Protected(http.MethodPost, "/api/swarm/join-tokens/{kind}/rotate", h.HandleRotateJoinToken)
	reg.Protected(http.MethodGet, "/api/swarm/unlock-key", h.HandleUnlockKey)
	reg.Protected(http.MethodPost, "/api/swarm/unlock-key/rotate", h.HandleRotateUnlockKey)
	reg.Protected(http.MethodPost, "/api/swarm/unlock", h.HandleUnlock)

	// --- services --------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/services", h.HandleServiceList)
	reg.Protected(http.MethodPost, "/api/swarm/services", h.HandleServiceCreate)
	reg.Protected(http.MethodGet, "/api/swarm/services/{id}", h.HandleServiceInspect)
	reg.Protected(http.MethodGet, "/api/swarm/services/{id}/logs", h.HandleServiceLogs)
	reg.Protected(http.MethodGet, "/api/swarm/services/{id}/env", h.HandleServiceEnv)
	reg.Protected(http.MethodPut, "/api/swarm/services/{id}", h.HandleServiceUpdate)
	reg.Protected(http.MethodPost, "/api/swarm/services/{id}/restart", h.HandleServiceRestart)
	reg.Protected(http.MethodPost, "/api/swarm/services/{id}/rollback", h.HandleServiceRollback)
	reg.Protected(http.MethodPost, "/api/swarm/services/{id}/scale", h.HandleServiceScale)
	reg.Protected(http.MethodDelete, "/api/swarm/services/{id}", h.HandleServiceRemove)

	// --- tasks -----------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/tasks", h.HandleTaskList)
	reg.Protected(http.MethodGet, "/api/swarm/tasks/{id}", h.HandleTaskInspect)
	reg.Protected(http.MethodGet, "/api/swarm/tasks/{id}/logs", h.HandleTaskLogs)

	// --- nodes -----------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/nodes", h.HandleNodeList)
	reg.Protected(http.MethodGet, "/api/swarm/nodes/{id}", h.HandleNodeInspect)
	reg.Protected(http.MethodPut, "/api/swarm/nodes/{id}", h.HandleNodeLabels)
	reg.Protected(http.MethodPost, "/api/swarm/nodes/{id}/availability", h.HandleNodeAvailability)
	reg.Protected(http.MethodPost, "/api/swarm/nodes/{id}/role", h.HandleNodeRole)
	reg.Protected(http.MethodDelete, "/api/swarm/nodes/{id}", h.HandleNodeRemove)

	// --- secrets ---------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/secrets", h.HandleSecretList)
	reg.Protected(http.MethodPost, "/api/swarm/secrets", h.HandleSecretCreate)
	reg.Protected(http.MethodGet, "/api/swarm/secrets/{id}", h.HandleSecretInspect)
	reg.Protected(http.MethodPut, "/api/swarm/secrets/{id}", h.HandleSecretUpdate)
	reg.Protected(http.MethodDelete, "/api/swarm/secrets/{id}", h.HandleSecretRemove)

	// --- configs ---------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/configs", h.HandleConfigList)
	reg.Protected(http.MethodPost, "/api/swarm/configs", h.HandleConfigCreate)
	reg.Protected(http.MethodGet, "/api/swarm/configs/{id}", h.HandleConfigInspect)
	reg.Protected(http.MethodPut, "/api/swarm/configs/{id}", h.HandleConfigUpdate)
	reg.Protected(http.MethodDelete, "/api/swarm/configs/{id}", h.HandleConfigRemove)

	// --- stacks ----------------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/stacks", h.HandleStackList)
	reg.Protected(http.MethodPost, "/api/swarm/stacks", h.HandleStackDeploy)
	reg.Protected(http.MethodGet, "/api/swarm/stacks/{name}", h.HandleStackInspect)
	reg.Protected(http.MethodDelete, "/api/swarm/stacks/{name}", h.HandleStackRemove)

	// --- overlay networks ------------------------------------------------
	reg.Protected(http.MethodGet, "/api/swarm/networks", h.HandleNetworkList)
	return nil
}

// RegisterMenu contributes the sidebar section.
//
// Every entry shares the parent "swarm", which `pluginMenuToSections` turns
// into a section of its own labelled from this plugin's own bundle
// (`swarm:nav.section.swarm`) rather than four loose entries under Plugins.
//
// The four groups are the ones the host section used, and their labels and
// paths come straight across: a group's entry points at its first tab, and the
// plugin's router lights it for every tab below it. What is gone is the host
// section's conditional shape — it collapsed to the overview alone when the
// current environment was not a Swarm. That test cost a `/swarm/capability`
// request on every page load in the product, including for the majority of
// installations that never ran a Swarm at all. Installing the add-on is the
// answer to the same question now, and the overview still renders the
// "initialise a cluster" empty state for an environment that is not one yet.
//
// Icon is a Tabler *component name*, not an asset filename: `resolvePluginIcon`
// looks the string up in a registry of imported components and falls back to
// IconPuzzle for anything it does not know.
func (p *SwarmPlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	items := []plugin.MenuItem{
		{
			Parent: "swarm", LabelNS: "swarm", LabelKey: "nav.swarm.overview",
			Path: "/plugins/addon-swarm", Icon: "IconTopologyStar3", Required: "swarm.view",
		},
		{
			Parent: "swarm", LabelNS: "swarm", LabelKey: "nav.swarm.workloads",
			Path: "/plugins/addon-swarm/services", Icon: "IconStack2", Required: "swarm_services.view",
		},
		{
			Parent: "swarm", LabelNS: "swarm", LabelKey: "nav.swarm.cluster",
			Path: "/plugins/addon-swarm/nodes", Icon: "IconServer2", Required: "swarm_nodes.view",
		},
		{
			Parent: "swarm", LabelNS: "swarm", LabelKey: "nav.swarm.config",
			Path: "/plugins/addon-swarm/secrets", Icon: "IconKey", Required: "swarm_secrets.view",
		},
	}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return err
		}
	}
	return nil
}

// RegisterWidgets declares the dashboard tiles the frontend bundle exports
// under `widgets[key]`. Title and description are i18n keys in the plugin's
// own namespace; the host resolves them in the widget picker.
func (p *SwarmPlugin) RegisterWidgets(r plugin.WidgetRegistry) error {
	for _, w := range []struct {
		key, icon     string
		width, height int
		minW, minH    int
	}{
		{key: "swarm-overview", icon: "IconServerBolt", width: 6, height: 2, minW: 4, minH: 2},
		{key: "swarm-services", icon: "IconStack2", width: 4, height: 3, minW: 3, minH: 2},
		{key: "swarm-nodes", icon: "IconServer2", width: 4, height: 3, minW: 3, minH: 2},
	} {
		camel := widgetI18nKey(w.key)
		if err := r.Add(w.key, plugin.WidgetDescriptor{
			Title:  "swarm:widgets." + camel + ".label",
			Path:   "/widgets/" + w.key,
			Width:  w.width,
			Height: w.height,
			Settings: map[string]any{
				"description": "swarm:widgets." + camel + ".description",
				"icon":        w.icon,
				"min_width":   w.minW,
				"min_height":  w.minH,
			},
		}); err != nil {
			return fmt.Errorf("register widget %s: %w", w.key, err)
		}
	}
	return nil
}

// widgetI18nKey turns `swarm-overview` into `swarmOverview`, the casing the
// i18n bundle uses.
func widgetI18nKey(key string) string {
	parts := strings.Split(key, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// RegisterNodes contributes the Swarm workflow nodes (see nodes.go). The
// loader calls this before Init, so the executors resolve the service
// lazily on each call.
func (p *SwarmPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	for _, n := range p.swarmNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("swarm: register node %s: %w", n.key, err)
		}
	}
	for _, n := range p.swarmTriggerNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("swarm: register trigger %s: %w", n.key, err)
		}
	}
	return nil
}

// RegisterAdapters contributes this plugin's runtime adapters.
//
// Optional half of the plugin contract (plugin.AdapterContributor): the loader
// calls it when the symbol implements it and skips it otherwise, so
// implementing it changes nothing about how the rest of the plugin loads.
//
// One family: Reporting — the capacity-and-placement report type and the
// platform facets (inventory, capacity, health, exposure, cost) in reports.go
// and reports_facets.go. Reporting keeps the cover, the period, the annex and
// the renderers; the Swarm questions live here so an installation that does
// not run a cluster sees no Swarm shelf.
//
// It costs nothing when unused: the provider registers whether or not
// Reporting is installed, and is only asked once it is.
func (p *SwarmPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Reporting().Register(swarmReportSpec(), &reportProvider{plugin: p}); err != nil {
		return fmt.Errorf("swarm: register report provider: %w", err)
	}
	return nil
}

// RegisterPermissions declares the twenty permissions this plugin owns.
//
// Routes are gated from the manifest regardless; registering is what puts the
// keys within reach of an operator. A plugin that skips this call is one only
// a wildcard role can use — the keys exist but appear in no role editor.
func (p *SwarmPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("swarm: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}

func (p *SwarmPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host

	// The host hands the pool over as `any` behind a `ForKind` lookup so the
	// plugin SDK never imports the Docker SDK. A nil here is not fatal: the
	// plugin still loads, its routes still answer, and every one of them
	// reports "docker connection failed" — which is the honest description of
	// a host that wired no pool, and a better outcome than refusing to boot.
	var pool dockerPool
	if host.DockerPool != nil {
		if accessor, ok := host.DockerPool.ForKind("docker").(plugin.DockerPoolAccessor); ok {
			pool = accessor
		}
	}
	if pool == nil && host.Logger != nil {
		host.Logger.Warn("swarm: no docker pool from host; every request will fail to connect")
	}

	p.handler.attach(pool, host.Logger, host.Audit)
	p.startWatcher()

	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.swarm.init", map[string]any{
			"slug":    p.manifest.Slug,
			"version": p.manifest.Version,
		})
	}
	if host.Logger != nil {
		host.Logger.Info("swarm: ready")
	}
	return nil
}

func (p *SwarmPlugin) Shutdown(_ context.Context) error {
	p.stopWatcher()
	// Nothing to tear down. The Docker clients belong to the host's pool, not
	// to this plugin, and the only state it holds is the capability cache —
	// which goes with the plugin's own memory.
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.swarm.shutdown", nil)
	}
	return nil
}
