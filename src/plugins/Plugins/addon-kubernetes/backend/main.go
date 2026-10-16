// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor Kubernetes.
//
// Cluster management — workloads, networking, storage, config and the cluster
// itself — as an installable add-on rather than part of the host.
//
// This used to be core: `src/backend/modules/kubernetes`, `src/backend/core/
// kubernetes`, ten RBAC keys in the host catalog, a six-entry sidebar section,
// five encrypted columns on the `environments` table, and `k8s.io/client-go`
// in the host binary. All of it lives here now. An installation that does not
// run Kubernetes no longer carries any of it — not the dependency, not the
// schema, not the permissions, not the navigation.
//
// The one thing that had to move with the feature is the cluster connection
// itself. Clusters were Docker "environments" with an orchestrator type, which
// is what kept client-go in the host; they are now
// `plugin_kubernetes_clusters`, encrypted through the host's own encryption
// service so the credentials the host wrote are the credentials this plugin
// reads. Host migration 147 performs that hand-off, because a plugin migration
// runs at install — long after the host has dropped the columns it would need
// to read.
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

// KubernetesPlugin is the plugin instance the host loads.
type KubernetesPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	store    *clusterStore
	pool     *ClientPool
	svc      *Service
	// watcher feeds the kubernetes-event trigger; nil until Init and
	// after Shutdown, or when the host wired no workflow bridge.
	watcher *k8sWatcher
}

// Plugin is the symbol the host looks up after dlopen.
var Plugin plugin.Plugin = &KubernetesPlugin{manifest: manifest}

var _ plugin.Plugin = (*KubernetesPlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-kubernetes",
	Name:     "Kubernetes",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Manage Kubernetes clusters from McHarbor: pods, deployments and other workloads, " +
		"services and networking, storage, config, namespaces, nodes and events — with pod " +
		"logs, deployment scaling and rolling restarts.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks this
	// slice, so the two lists cannot drift apart.
	//
	// Four keys replace the ten the host used to carry (`pods.*`,
	// `deployments.*`, `k8s_services.*`, `namespaces.view`). Splitting read
	// access three ways was never enforceable: every one of those resources is
	// served by one `/summary` call, so a role with `pods.view` and without
	// `k8s_services.view` still received the services. One honest `view` key
	// says what the product can actually gate.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key:      "kubernetes.view",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows browsing a cluster: workloads, networking, storage, config, " +
				"namespaces, nodes and events. Reading pod logs needs kubernetes.logs.",
		},
		{
			// Logs get their own key. A pod's stdout routinely carries
			// connection strings, tokens and customer data, so "may browse
			// the cluster" and "may read what the cluster is printing" are
			// different grants. Unlike the per-resource read split this
			// replaces, this one is enforceable as written: logs already have
			// their own endpoint.
			Key:      "kubernetes.logs",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows reading pod logs. Pod output frequently contains secrets, " +
				"so this is separate from kubernetes.view.",
		},
		{
			Key:      "kubernetes.manage",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows scaling a deployment and triggering a rolling restart. " +
				"Deleting workloads needs kubernetes.delete.",
		},
		{
			Key:         "kubernetes.delete",
			Scope:       "env",
			Category:    "Plugins",
			Description: "Allows deleting pods and deployments.",
		},
		{
			Key:      "kubernetes.clusters.manage",
			Scope:    "global",
			Category: "Plugins",
			Description: "Allows adding, editing and removing cluster connections, including " +
				"their kubeconfig and bearer-token credentials.",
		},
	},
}

func (p *KubernetesPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *KubernetesPlugin) RegisterRoutes(reg plugin.Registry) error {
	// Cluster inventory. The list is `kubernetes.view` rather than
	// `kubernetes.clusters.manage` because every page's cluster picker reads
	// it; the response carries names and ids, never a credential.
	reg.Protected(http.MethodGet, "/api/kubernetes/clusters", p.handleListClusters)
	reg.Protected(http.MethodPost, "/api/kubernetes/clusters", p.handleCreateCluster)
	reg.Protected(http.MethodGet, "/api/kubernetes/clusters/{id}", p.handleGetCluster)
	reg.Protected(http.MethodPut, "/api/kubernetes/clusters/{id}", p.handleUpdateCluster)
	reg.Protected(http.MethodDelete, "/api/kubernetes/clusters/{id}", p.handleDeleteCluster)

	reg.Protected(http.MethodGet, "/api/kubernetes/summary", p.handleSummary)

	reg.Protected(http.MethodGet, "/api/kubernetes/pods/{namespace}/{name}", p.handlePod)
	reg.Protected(http.MethodGet, "/api/kubernetes/pods/{namespace}/{name}/logs", p.handlePodLogs)
	reg.Protected(http.MethodDelete, "/api/kubernetes/pods/{namespace}/{name}", p.handleDeletePod)

	reg.Protected(http.MethodGet, "/api/kubernetes/deployments/{namespace}/{name}", p.handleDeployment)
	reg.Protected(http.MethodPost, "/api/kubernetes/deployments/{namespace}/{name}/scale", p.handleScaleDeployment)
	reg.Protected(http.MethodPost, "/api/kubernetes/deployments/{namespace}/{name}/restart", p.handleRestartDeployment)
	reg.Protected(http.MethodDelete, "/api/kubernetes/deployments/{namespace}/{name}", p.handleDeleteDeployment)
	return nil
}

// RegisterMenu contributes the sidebar section.
//
// Every entry shares the parent "kubernetes", which `pluginMenuToSections`
// turns into a section of its own labelled from this plugin's own bundle
// (`kubernetes:nav.section.kubernetes`) rather than six loose entries under
// Plugins. The six groups are the ones the host section used, and for the same
// reason: the split the Kubernetes Dashboard, Lens and Rancher agree on —
// group by the job a resource does for an application, not by API group.
//
// Icon is a Tabler *component name*, not an asset filename: `resolvePluginIcon`
// looks the string up in a registry of imported components and falls back to
// IconPuzzle for anything it does not know.
func (p *KubernetesPlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	items := []plugin.MenuItem{
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.overview",
			Path: "/plugins/addon-kubernetes", Icon: "IconTopologyStar3", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.workloads",
			Path: "/plugins/addon-kubernetes/pods", Icon: "IconBoxMultiple", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.network",
			Path: "/plugins/addon-kubernetes/services", Icon: "IconRoute", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.storage",
			Path: "/plugins/addon-kubernetes/storage", Icon: "IconDeviceFloppy", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.config",
			Path: "/plugins/addon-kubernetes/config", Icon: "IconAdjustments", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.cluster",
			Path: "/plugins/addon-kubernetes/nodes", Icon: "IconServer2", Required: "kubernetes.view",
		},
		{
			Parent: "kubernetes", LabelNS: "kubernetes", LabelKey: "nav.clusters",
			Path: "/plugins/addon-kubernetes/clusters", Icon: "IconCloudNetwork", Required: "kubernetes.clusters.manage",
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
func (p *KubernetesPlugin) RegisterWidgets(r plugin.WidgetRegistry) error {
	for _, w := range []struct {
		key, icon     string
		width, height int
		minW, minH    int
	}{
		{key: "cluster-health", icon: "IconHexagons", width: 6, height: 3, minW: 4, minH: 3},
		{key: "workloads", icon: "IconStack2", width: 4, height: 3, minW: 3, minH: 2},
		{key: "events", icon: "IconBell", width: 4, height: 3, minW: 3, minH: 2},
	} {
		camel := widgetI18nKey(w.key)
		if err := r.Add(w.key, plugin.WidgetDescriptor{
			Title:  "kubernetes:widgets." + camel + ".label",
			Path:   "/widgets/" + w.key,
			Width:  w.width,
			Height: w.height,
			Settings: map[string]any{
				"description": "kubernetes:widgets." + camel + ".description",
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

// widgetI18nKey turns `cluster-health` into `clusterHealth`, the casing the
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

// RegisterNodes contributes the Kubernetes workflow nodes (see nodes.go).
// The loader calls this before Init, so the executors resolve the
// service lazily on each call.
func (p *KubernetesPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	for _, n := range p.kubernetesNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("kubernetes: register node %s: %w", n.key, err)
		}
	}
	for _, n := range p.kubernetesTriggerNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("kubernetes: register trigger %s: %w", n.key, err)
		}
	}
	return nil
}

// RegisterPermissions declares the four permissions this plugin owns.
//
// Routes are gated from the manifest regardless; registering is what puts the
// keys within reach of an operator. A plugin that skips this call is one only
// a wildcard role can use — the keys exist but appear in no role editor.
// RegisterAdapters contributes this plugin's runtime adapters.
//
// Optional half of the plugin contract (plugin.AdapterContributor): the
// loader calls it when the symbol implements it and skips it otherwise,
// so implementing it changed nothing about how the rest of the plugin
// loads.
//
// Two families:
//
//   - Reporting — five Kubernetes report types, reachable only once the
//     Reporting plugin is installed.
//   - Alerting — the conditions in alerts.go. The host's alert engine
//     polls them on its own leader-elected ticker; the plugin exposes
//     no route for them and owns no thresholds, schedules or delivery.
//
// Neither costs anything when unused: an operator with no alert rules
// naming source_type "kubernetes" is never asked to evaluate one.
func (p *KubernetesPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Reporting().Register(kubernetesReportSpec(), &reportProvider{plugin: p}); err != nil {
		return fmt.Errorf("kubernetes: register report provider: %w", err)
	}
	if err := reg.Alerting().Register(kubernetesAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("kubernetes: register alert source: %w", err)
	}
	return nil
}

func (p *KubernetesPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("kubernetes: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}

func (p *KubernetesPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host

	p.store = &clusterStore{db: host.DB, enc: host.Encryption}
	// Self-heal the schema before any query path becomes reachable. The host
	// re-applies migrations/*.sql only on a fresh install, so an existing
	// install upgrading into a build with a new migration file would otherwise
	// load fine and then fail every request on a missing table.
	if err := p.store.ensureSchema(ctx); err != nil {
		return err
	}
	// Credentials stored before the store encrypted on write are sealed once here. A
	// failure is logged, not returned: the clusters still work from their stored values.
	encrypted, backfillErr := p.store.backfillEncryption(ctx)
	if host.Logger != nil {
		if backfillErr != nil {
			host.Logger.Error("kubernetes: encrypting stored cluster credentials failed", "error", backfillErr)
		}
		host.Logger.Info("kubernetes: cluster credential backfill", "table", "plugin_kubernetes_clusters",
			"kubeconfig", encrypted["kubeconfig"], "bearer_token", encrypted["bearer_token"], "ca_cert", encrypted["ca_cert"])
	}

	p.pool = NewClientPool(p.store)
	p.svc = NewService(p.pool, host.Logger)
	p.startWatcher()

	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.kubernetes.init", map[string]any{
			"slug":    p.manifest.Slug,
			"version": p.manifest.Version,
		})
	}
	if host.Logger != nil {
		host.Logger.Info("kubernetes: ready")
	}
	return nil
}

func (p *KubernetesPlugin) Shutdown(_ context.Context) error {
	p.stopWatcher()
	// Drop the cached clientsets first: each one holds an HTTP transport with
	// live keep-alive connections to an API server, and leaving those open
	// after the host has torn the plugin down leaks a socket per cluster.
	if p.pool != nil {
		p.pool.Close()
	}
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.kubernetes.shutdown", nil)
	}
	return nil
}
