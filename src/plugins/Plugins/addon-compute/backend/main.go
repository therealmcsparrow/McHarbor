// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor Compute Foundation.
//
// The shared half of running guests on your own hosts: capability probing,
// storage pools, guest networks and the image catalog. It manages no guests
// itself — Virtual Machines (addon-kvm) and System Containers (addon-lxc) do
// that, and both need exactly these four things.
//
// # Why this is a plugin at all
//
// The compute roadmap argued the whole capability had to be core, on the
// grounds that a plugin cannot extend `src/agent/protocol.go` or the agent's
// message dispatch. That is true, and it is why the node executor — QEMU,
// libcontainer, netlink, the storage drivers — is core.
//
// It does not follow for anything above that line. Kubernetes made the same
// journey in the other direction: ten RBAC keys, a six-entry sidebar section,
// five encrypted columns and `client-go` all left the host, and an installation
// that does not run Kubernetes now carries none of it. Compute follows that
// precedent. An installation that runs no guests carries no compute schema, no
// compute permissions and no compute navigation.
//
// What crosses the line is the host's agent-rpc bridge: this plugin declares the
// agent messages it sends and observes under `agent-rpc` in plugin.yaml, the host
// carries them inside its own reviewed ceiling, and everything above the wire —
// the model, the conversions, relocation progress — lives here (agent_bridge.go).
//
// # Why it installs itself
//
// addon-kvm and addon-lxc both need the same pools, the same bridges and the
// same image cache. Duplicating that would give an operator with both installed
// two pool lists over one set of disks. So this ships as a dependency: the host
// installs it alongside either dependant and removes it when the last one goes,
// and it is never something to find in the store.
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginLogger is the host logger's shape. Declared here rather than taken as
// *slog.Logger because the plugin contract hands over an interface.
type pluginLogger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// ComputePlugin is the plugin instance the host loads.
type ComputePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	svc      *Service
	handler  *Handler
	logger   pluginLogger
	// rpc is nil when the host wired no agent bridge.
	rpc *agentBridge
}

// Plugin is the symbol the host looks up after dlopen.
var Plugin plugin.Plugin = &ComputePlugin{manifest: manifest}

var _ plugin.Plugin = (*ComputePlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-compute",
	Name:     "Compute Foundation",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Node capability probing, storage pools, guest networks and the image catalog " +
		"shared by Virtual Machines and System Containers.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks this
	// slice, so the two lists cannot drift apart.
	//
	// Four keys. The roadmap's view / manage / delete split covers pools,
	// networks and catalog images, which are all read by the same overview call
	// and written by an operator doing the same job. Upload is the exception
	// that earned its own key: it is the only way to put bytes of the
	// operator's choosing on the host, and a site that is happy to let someone
	// download Debian is not necessarily happy to let them upload anything.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key:      "compute.view",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows seeing a host's compute capability, its storage pools, " +
				"its guest networks and the image catalog.",
		},
		{
			Key:      "compute.manage",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows enabling compute on a host, creating and editing storage " +
				"pools and guest networks, and downloading images. Removing them needs " +
				"compute.delete.",
		},
		{
			// Upload is separate from manage for the reason manage is separate
			// from view: it is a different authority, not a bigger one. A
			// catalog download fetches a named, checksummed image from a
			// distribution; an upload puts whatever the operator chose onto the
			// host's disk. Ships in no preset.
			Key:      "compute.images.upload",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows uploading an image file from the browser onto a host. " +
				"Downloading from the curated catalog only needs compute.manage.",
		},
		{
			// Browsing a pool shows the host's own filesystem under the pool
			// root, which compute.view does not otherwise expose: a pool's
			// row says how full it is, not what is in it. Ships in no preset
			// for the same reason upload does not.
			Key:      "compute.pools.browse",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows listing and reading the files inside a storage pool. " +
				"Writing to them needs compute.manage and removing them needs " +
				"compute.delete.",
		},
		{
			// Delete is separate because it is the key that can take a pool
			// with guest volumes still on it. That is a data-loss action and
			// does not belong in the same grant as "create a pool".
			Key:      "compute.delete",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows removing storage pools and guest networks and evicting " +
				"cached images, including forcing removal of a pool that still holds " +
				"guest volumes.",
		},
	},
}

// Manifest returns the plugin's manifest.
func (p *ComputePlugin) Manifest() plugin.Manifest { return p.manifest }

// Init wires the plugin to the host.
func (p *ComputePlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host
	p.logger = host.Logger

	// The bridge is nil when no agent transport is wired, or when the host
	// predates agent-rpc. That is not fatal: the pages still render and explain
	// that no host can run guests yet, which is more useful than the section
	// refusing to load.
	p.rpc = newAgentBridge(host.AgentRPC)
	if p.rpc == nil {
		p.logger.Warn("compute: the host wired no agent bridge; " +
			"pools, networks and images will be read-only")
	}

	p.svc = NewService(host.DB, p.rpc, host.Audit, host.Lifecycle, host.Workflows, host.Catalogs, host.WorkDir, p.logger)
	p.handler = NewHandler(p.svc, host.Audit, p.logger)

	// Columns added after the first release, applied here because a plugin's
	// migrations run only at install and would therefore never reach an
	// instance that already has this plugin.
	if err := p.svc.EnsureColumns(ctx); err != nil {
		return fmt.Errorf("compute: updating the schema: %w", err)
	}

	// A download is driven by a goroutine in this process, so nothing survives
	// a restart. Any row still claiming `downloading` is stranded, and it hides
	// the Download button behind a bar that will never move.
	p.svc.ResetInterruptedDownloads(context.Background())

	// The catalog directory is the operator's to edit, and it is read on every
	// boot rather than seeded once: a plugin's migrations run only at install,
	// so a migration could never add an image to an instance already running.
	if result, err := p.svc.ReconcileCatalog(ctx); err != nil {
		p.logger.Warn("compute: reading the image catalog failed", "error", err)
	} else if result.Loaded > 0 || result.Removed > 0 || len(result.Skipped) > 0 {
		p.logger.Info("compute: image catalog loaded", "dir", result.Dir,
			"loaded", result.Loaded, "removed", result.Removed,
			"skipped", len(result.Skipped), "kept", len(result.Kept))
		for _, reason := range result.Skipped {
			p.logger.Warn("compute: catalog entry skipped", "reason", reason)
		}
	}

	// Unsolicited guest events. DHCP leases are recorded here so the lease
	// viewer reflects what the network actually handed out, rather than what
	// McHarbor intended.
	// Relocation progress arrives the same way and is kept by the bridge.
	if p.rpc != nil {
		p.rpc.Subscribe(func(event GuestEvent) {
			// The host delivers from this plugin's own queue, so a slow upsert
			// delays only this plugin's frames; one upsert per lease keeps it short.
			p.svc.RecordLease(context.Background(), event)
		})
	}

	p.logger.Info("compute foundation ready")
	return nil
}

// Shutdown releases the plugin.
func (p *ComputePlugin) Shutdown(_ context.Context) error {
	// The service holds no goroutines of its own. The subscription is dropped
	// here as well as by the host's teardown, so no frame reaches a plugin that
	// has stopped serving.
	if p.rpc != nil {
		p.rpc.Unsubscribe()
	}
	return nil
}

// RegisterRoutes registers the plugin's HTTP surface.
//
// Every route is Protected, and the permission each one requires is declared in
// plugin.yaml — the host's plugin route registry wraps them in its permission
// middleware, so declaring correctly is enforcing.
func (p *ComputePlugin) RegisterRoutes(reg plugin.Registry) error {
	routes := []struct {
		method string
		path   string
		// public routes mount outside the session middleware. Exactly one does,
		// and it authenticates itself — see HandleLibraryPull. The manifest's
		// `public: true` records the intent, but the mount group is chosen
		// here: the loader reads the manifest flag only for the route table it
		// reports, so a route declared public and registered with Protected is
		// listed as public and still answers 401.
		public  bool
		handler func(*Handler, http.ResponseWriter, *http.Request)
	}{
		{http.MethodGet, "/api/compute/overview", false, (*Handler).HandleOverview},

		// Moving a pool's data, and repairing what an earlier move left
		// pointing at the old path.
		{http.MethodPost, "/api/compute/pools/{poolId}/move/plan", false, (*Handler).HandlePlanPoolMove},
		{http.MethodPost, "/api/compute/pools/{poolId}/move", false, (*Handler).HandleStartPoolMove},
		{http.MethodGet, "/api/compute/pools/{poolId}/move", false, (*Handler).HandlePoolMoveStatus},
		{http.MethodPost, "/api/compute/pools/{poolId}/repair", false, (*Handler).HandleRepairPool},

		// Where a host keeps VM state, rootfs trees and cached images. The
		// manifest has declared these six since the feature's first attempt;
		// they are registered here in the order it declares them.
		{http.MethodGet, "/api/compute/locations", false, (*Handler).HandleListLocations},
		{http.MethodPut, "/api/compute/locations", false, (*Handler).HandleSaveGlobalLocations},
		{http.MethodGet, "/api/compute/locations/{envId}", false, (*Handler).HandleGetLocations},
		{http.MethodPut, "/api/compute/locations/{envId}", false, (*Handler).HandleSaveLocations},
		{http.MethodDelete, "/api/compute/locations/{envId}", false, (*Handler).HandleRemoveLocations},
		{http.MethodPost, "/api/compute/locations/{envId}/apply", false, (*Handler).HandleApplyLocations},

		{http.MethodGet, "/api/compute/environments", false, (*Handler).HandleListEnvironments},
		{http.MethodGet, "/api/compute/environments/{envId}/capability", false, (*Handler).HandleGetCapability},
		{http.MethodPost, "/api/compute/environments/{envId}/probe", false, (*Handler).HandleProbe},
		{http.MethodPost, "/api/compute/environments/{envId}/capability/install", false, (*Handler).HandleInstallCapability},
		{http.MethodPut, "/api/compute/environments/{envId}", false, (*Handler).HandleUpdateEnvironment},

		// The detect routes come before the {id} routes so chi does not read
		// "detect" as an id.
		{http.MethodGet, "/api/compute/pools/detect", false, (*Handler).HandleDetectPools},
		{http.MethodGet, "/api/compute/pools", false, (*Handler).HandleListPools},
		{http.MethodPost, "/api/compute/pools", false, (*Handler).HandleCreatePool},
		{http.MethodPost, "/api/compute/pools/default", false, (*Handler).HandleEnsureDefaultPool},
		{http.MethodPut, "/api/compute/pools/{id}", false, (*Handler).HandleRefreshPool},
		{http.MethodDelete, "/api/compute/pools/{id}", false, (*Handler).HandleDeletePool},

		// The pool file browser. "files" cannot collide with a pool id
		// because the segment after {id} is fixed, so these need no ordering
		// against the routes above.
		{http.MethodGet, "/api/compute/pools/{id}/files", false, (*Handler).HandleBrowsePool},
		{http.MethodGet, "/api/compute/pools/{id}/files/content", false, (*Handler).HandleReadPoolFile},
		{http.MethodPut, "/api/compute/pools/{id}/files/content", false, (*Handler).HandleWritePoolFile},
		{http.MethodPost, "/api/compute/pools/{id}/files/mkdir", false, (*Handler).HandleCreatePoolDir},
		{http.MethodPost, "/api/compute/pools/{id}/files/rename", false, (*Handler).HandleRenamePoolFile},
		{http.MethodDelete, "/api/compute/pools/{id}/files", false, (*Handler).HandleDeletePoolFile},

		{http.MethodGet, "/api/compute/networks/detect", false, (*Handler).HandleDetectLinks},
		{http.MethodGet, "/api/compute/networks", false, (*Handler).HandleListNetworks},
		{http.MethodPost, "/api/compute/networks", false, (*Handler).HandleCreateNetwork},
		{http.MethodPut, "/api/compute/networks/{id}", false, (*Handler).HandleUpdateNetwork},
		{http.MethodDelete, "/api/compute/networks/{id}", false, (*Handler).HandleDeleteNetwork},
		{http.MethodGet, "/api/compute/networks/{id}/leases", false, (*Handler).HandleListLeases},

		// Before the {id} routes, so chi does not read "library" or "upload"
		// as an image id.
		{http.MethodGet, "/api/compute/images/library", false, (*Handler).HandleGetLibrary},
		{http.MethodPost, "/api/compute/images/upload", false, (*Handler).HandleUploadImage},
		{http.MethodGet, "/api/compute/internal/library/{grantId}", true, (*Handler).HandleLibraryPull},
		{http.MethodGet, "/api/compute/images", false, (*Handler).HandleListImages},
		{http.MethodPost, "/api/compute/images", false, (*Handler).HandleCreateImage},
		{http.MethodPost, "/api/compute/images/catalog/reload", false, (*Handler).HandleReloadCatalog},
		{http.MethodPost, "/api/compute/images/{id}/fetch", false, (*Handler).HandleFetchImage},
		{http.MethodPost, "/api/compute/images/{id}/fetch-all", false, (*Handler).HandleFetchImageEverywhere},
		{http.MethodDelete, "/api/compute/images/{id}/cache", false, (*Handler).HandleEvictImage},
		{http.MethodDelete, "/api/compute/images/{id}", false, (*Handler).HandleDeleteImage},
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
func (p *ComputePlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	items := []plugin.MenuItem{
		{
			Parent: "compute", LabelNS: "compute", LabelKey: "nav.overview",
			Path: "/plugins/addon-compute", Icon: "IconServerBolt", Required: "compute.view",
			Priority: 100,
		},
		{
			Parent: "compute", LabelNS: "compute", LabelKey: "nav.images",
			Path: "/plugins/addon-compute/images", Icon: "IconPhoto", Required: "compute.view",
			Priority: 80,
		},
		{
			Parent: "compute", LabelNS: "compute", LabelKey: "nav.pools",
			Path: "/plugins/addon-compute/pools", Icon: "IconDatabase", Required: "compute.view",
			Priority: 70,
		},
		{
			Parent: "compute", LabelNS: "compute", LabelKey: "nav.networks",
			Path: "/plugins/addon-compute/networks", Icon: "IconNetwork", Required: "compute.view",
			Priority: 60,
		},
	}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return fmt.Errorf("compute: register menu %s: %w", item.LabelKey, err)
		}
	}
	return nil
}

// RegisterTabs mounts this plugin's settings surfaces on two host pages.
//
// Locations belong on Settings and on an environment, not on a page of their
// own: the McHarbor-wide defaults are a setting like any other, and a host's
// override belongs beside everything else about that host. Both tabs are the
// same component in two scopes, which is what stops them drifting apart.
func (p *ComputePlugin) RegisterTabs(reg plugin.TabRegistry) error {
	// No tabs. The Locations tabs lived here until a pool turned out to carry
	// its own target, which made a second place to name the same path a way to
	// disagree with it.
	items := []plugin.TabItem{}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return fmt.Errorf("compute: register tab %s: %w", item.Key, err)
		}
	}
	return nil
}

// RegisterWidgets contributes the dashboard widgets.
//
// Title and description are i18n keys in this plugin's own namespace; the host
// resolves them in the widget picker. Path names the export the frontend UMD
// bundle provides under `widgets[key]`.
func (p *ComputePlugin) RegisterWidgets(reg plugin.WidgetRegistry) error {
	for _, widget := range []struct {
		key, camel, icon string
		width, height    int
		minW, minH       int
	}{
		// Pool capacity is the number that predicts a problem: a thin pool that
		// fills pauses every guest on it, and the warning has to arrive before
		// that rather than as an incident.
		{key: "compute-capacity", camel: "computeCapacity", icon: "IconDatabase",
			width: 4, height: 3, minW: 3, minH: 2},
		{key: "compute-hosts", camel: "computeHosts", icon: "IconServerBolt",
			width: 4, height: 3, minW: 3, minH: 2},
		// Images are the one compute resource that grows without anybody
		// deciding to grow it, so the footprint gets a tile of its own.
		{key: "compute-images", camel: "computeImages", icon: "IconPhoto",
			width: 4, height: 3, minW: 3, minH: 2},
		{key: "compute-networks", camel: "computeNetworks", icon: "IconNetwork",
			width: 4, height: 3, minW: 3, minH: 2},
	} {
		if err := reg.Add(widget.key, plugin.WidgetDescriptor{
			Title:  "compute:widgets." + widget.camel + ".label",
			Path:   "/widgets/" + widget.key,
			Width:  widget.width,
			Height: widget.height,
			Settings: map[string]any{
				"description": "compute:widgets." + widget.camel + ".description",
				"icon":        widget.icon,
				"min_width":   widget.minW,
				"min_height":  widget.minH,
			},
		}); err != nil {
			return fmt.Errorf("register widget %s: %w", widget.key, err)
		}
	}
	return nil
}

// RegisterNodes contributes the infrastructure workflow nodes.
//
// Guest lifecycle nodes belong to addon-kvm and addon-lxc, which own those
// objects. What lives here is the three things a compute workflow needs to
// decide with: can this host run it, is there room, and is the image present.
// See nodes.go.
func (p *ComputePlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	return p.registerNodes(reg)
}

// RegisterAdapters contributes this plugin's runtime adapters.
//
// Optional half of the plugin contract (plugin.AdapterContributor): the loader
// calls it when the symbol implements it and skips it otherwise.
//
// Two families, neither of which costs anything when unused:
//
//   - Reporting — two compute report types, plus the inventory, capacity and
//     health facets of the shared reports, reachable only once the
//     Reporting plugin is installed.
//   - Alerting — the conditions in alerts.go. The host's engine owns the
//     thresholds, the schedule, the firing state and the delivery; this plugin
//     only answers whether a condition holds when asked.
func (p *ComputePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Reporting().Register(computeReportSpec(), &reportProvider{plugin: p}); err != nil {
		return fmt.Errorf("compute: register report provider: %w", err)
	}
	if err := reg.Alerting().Register(computeAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("compute: register alert source: %w", err)
	}
	return nil
}

// RegisterPermissions declares the plugin's RBAC keys.
func (p *ComputePlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return err
		}
	}
	return nil
}
