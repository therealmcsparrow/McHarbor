// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// KubernetesPlugin contributes CLI commands. Asserted here rather than left to
// the loader's type switch, so a signature drift is a compile error.
var _ plugin.CLIProvider = (*KubernetesPlugin)(nil)

// cliRoot is the command group this plugin owns. `k8s` rather than `kubernetes`
// because the CLI's own `cluster` command already exists and `k8s` is what an
// operator types; both names are refused by the host if a core command takes them.
const cliRoot = "k8s"

// clusterFlag is the `--cluster` query flag every read and write carries.
//
// The plugin scopes by cluster id, not by environment: every handler resolves its
// target through `clusterID(r)` reading `?cluster=`, and an empty value means "the
// default cluster". So no command in this set is EnvScoped — passing `?env=` would
// be ignored, which is worse than not offering it.
func clusterFlag() plugin.CLIFlag {
	return plugin.CLIFlag{
		Name:   "cluster",
		Type:   plugin.FlagString,
		Target: plugin.TargetQuery,
		Field:  "cluster",
		Help:   "cluster id; empty targets the default cluster",
	}
}

// clusterColumnSet is the cluster listing's rendering. Credentials are absent from
// the response by construction — `Cluster` carries `hasKubeconfig` / `hasToken`
// booleans instead of the kubeconfig and the bearer token — so the table can name
// the connection style without ever printing one.
func clusterColumnSet() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "SERVER", Path: "serverUrl"},
		{Header: "NAMESPACE", Path: "namespace"},
		{Header: "DEFAULT", Path: "isDefault"},
		{Header: "ACTIVE", Path: "isActive"},
		{Header: "KUBECONFIG", Path: "hasKubeconfig"},
		{Header: "TOKEN", Path: "hasToken"},
		{Header: "AGE", Path: "createdAt", Format: "age"},
	}
}

// RegisterCLI declares the `k8s` command group. Every Route is the string
// RegisterRoutes hands the host, so renaming a route without the command is a
// compile error here rather than a 404 an operator discovers.
func (p *KubernetesPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:    cliRoot,
		Short:   "Manage Kubernetes clusters, pods and deployments",
		Aliases: []string{"kube"},
	}); err != nil {
		return err
	}
	for _, cmd := range cliCommands() {
		if err := reg.Add(cmd); err != nil {
			return err
		}
	}
	return nil
}

// cliCommands is the whole command table, kept as data so the test can walk it
// against the manifest rather than against a second copy of the same list.
//
// Thirteen commands for thirteen routes: there is deliberately no `k8s pod list`
// or `k8s deployment list`, because the plugin exposes no such route — the
// interface finds a pod through the one `/summary` response, and so does `k8s
// summary`.
func cliCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:    []string{"cluster", "list"},
			Kind:    plugin.CLIList,
			Method:  http.MethodGet,
			Route:   "/api/kubernetes/clusters",
			Needs:   "kubernetes.view",
			Risk:    plugin.RiskSafe,
			Columns: clusterColumnSet(),
			Short:   "List the configured cluster connections",
			Example: "mcharbor k8s cluster list",
		},
		{
			// Reading one cluster needs `kubernetes.clusters.manage`, not
			// `kubernetes.view` — the manifest's choice, mirrored here rather
			// than softened, because the host refuses a weaker key than the
			// route enforces.
			Path:    []string{"cluster", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/kubernetes/clusters/{id}",
			Args:    []string{"id"},
			Needs:   "kubernetes.clusters.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Show one cluster connection",
			Long:    "Requires kubernetes.clusters.manage, the key that also edits clusters: the route is declared that way in the plugin manifest.",
			Example: "mcharbor k8s cluster get c8f2k1",
		},
		{
			Path:   []string{"cluster", "add"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/kubernetes/clusters",
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "display name"},
				{Name: "namespace", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "namespace", Help: "default namespace"},
				{Name: "server-url", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "serverUrl", Help: "API server URL, for a bearer-token connection"},
				{Name: "default", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isDefault", Help: "make this the default cluster"},
				{Name: "active", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isActive", Help: "enable the connection"},
			},
			Needs: "kubernetes.clusters.manage",
			Risk:  plugin.RiskSafe,
			Short: "Add a cluster connection",
			// The credentials are deliberately not flags. A kubeconfig is a
			// multi-line document and a bearer token on a command line lands in
			// the shell history, so both arrive through --data <file>.
			Long:    "Pass the kubeconfig or bearerToken and caCert through --data <file>; they are not flags, because a credential on a command line lands in the shell history.",
			Example: "mcharbor k8s cluster add --name prod --data ./cluster.json",
		},
		{
			// Disruptive rather than safe: the handler evicts the cached
			// clientset, so every in-flight page against this cluster
			// reconnects against whatever the new connection details say.
			Path:   []string{"cluster", "update"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/kubernetes/clusters/{id}",
			Args:   []string{"id"},
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "display name"},
				{Name: "namespace", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "namespace", Help: "default namespace"},
				{Name: "server-url", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "serverUrl", Help: "API server URL"},
				{Name: "default", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isDefault", Help: "make this the default cluster"},
				{Name: "active", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isActive", Help: "enable the connection"},
			},
			Needs:   "kubernetes.clusters.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Change a cluster connection",
			Long:    "An absent field is left alone. Updating drops the cached clientset, so open sessions against this cluster reconnect.",
			Example: "mcharbor k8s cluster update c8f2k1 --namespace default",
		},
		{
			// Irreversible: the row and its encrypted kubeconfig and bearer
			// token are deleted, and nothing in the product can reconstruct a
			// credential the operator no longer holds.
			Path:    []string{"cluster", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/kubernetes/clusters/{id}",
			Args:    []string{"id"},
			Needs:   "kubernetes.clusters.manage",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove a cluster connection",
			Long:    "Deletes the stored connection and its encrypted kubeconfig and bearer token. The credential cannot be recovered from McHarbor afterwards.",
			Example: "mcharbor k8s cluster delete c8f2k1 --confirm c8f2k1",
		},
		{
			Path:   []string{"summary"},
			Kind:   plugin.CLIGet,
			Method: http.MethodGet,
			Route:  "/api/kubernetes/summary",
			Flags: []plugin.CLIFlag{
				clusterFlag(),
				{Name: "namespace", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "namespace", Help: "narrow the inventory to one namespace"},
			},
			Needs: "kubernetes.view",
			Risk:  plugin.RiskSafe,
			Short: "Show the whole cluster inventory",
			// This is the only route that enumerates pods, deployments,
			// services, nodes and events, so it is also how an operator finds a
			// name to pass to the commands below.
			Long:    "One response carries workloads, networking, storage, config, namespaces, nodes and events. Use -o json to find a pod or deployment name; there is no list route for either.",
			Example: "mcharbor k8s summary --namespace kube-system -o json",
		},
		{
			Path:    []string{"pod", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/kubernetes/pods/{namespace}/{name}",
			Args:    []string{"namespace", "name"},
			Flags:   []plugin.CLIFlag{clusterFlag()},
			Needs:   "kubernetes.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show one pod",
			Example: "mcharbor k8s pod get default web-7d9f-abcde",
		},
		{
			// text, not get: the handler answers `text/plain` and copies the
			// API server's stream through, so parsing it as JSON would fail.
			Path:   []string{"pod", "logs"},
			Kind:   plugin.CLIText,
			Method: http.MethodGet,
			Route:  "/api/kubernetes/pods/{namespace}/{name}/logs",
			Args:   []string{"namespace", "name"},
			Flags: []plugin.CLIFlag{
				clusterFlag(),
				{Name: "container", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "container", Help: "container name; empty picks the pod's first"},
				{Name: "tail", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "tail", Default: "500", Help: "lines from the end, 1-10000"},
				{Name: "previous", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "previous", Help: "read the previous container instance's log"},
			},
			Needs: "kubernetes.logs",
			Risk:  plugin.RiskSafe,
			Short: "Print a pod container's log",
			// Its own key, not kubernetes.view: pod stdout routinely carries
			// connection strings and tokens.
			Long:    "Needs kubernetes.logs rather than kubernetes.view, because a pod's output frequently contains secrets. The tail is bounded; this is not a follow.",
			Example: "mcharbor k8s pod logs default web-7d9f-abcde --tail 200",
		},
		{
			// Irreversible: a pod is not recoverable. A deployment-managed pod
			// is replaced, but the one that was running, and whatever was in
			// its ephemeral storage, is gone.
			Path:    []string{"pod", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/kubernetes/pods/{namespace}/{name}",
			Args:    []string{"namespace", "name"},
			Flags:   []plugin.CLIFlag{clusterFlag()},
			Needs:   "kubernetes.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Delete a pod",
			Long:    "The pod and its ephemeral storage are gone. A controller-managed pod is replaced by a new one; a bare pod is not.",
			Example: "mcharbor k8s pod delete default web-7d9f-abcde --confirm web-7d9f-abcde",
		},
		{
			Path:    []string{"deployment", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/kubernetes/deployments/{namespace}/{name}",
			Args:    []string{"namespace", "name"},
			Flags:   []plugin.CLIFlag{clusterFlag()},
			Needs:   "kubernetes.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show one deployment",
			Example: "mcharbor k8s deployment get default web",
		},
		{
			// Disruptive: scaling down terminates running pods, and scaling to
			// zero takes the workload offline. Nothing is destroyed, so it is
			// not irreversible.
			Path:   []string{"deployment", "scale"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/kubernetes/deployments/{namespace}/{name}/scale",
			Args:   []string{"namespace", "name"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				clusterFlag(),
				{Name: "replicas", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "replicas", Required: true, Help: "desired replica count, 0-1000"},
			},
			Needs:   "kubernetes.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Set a deployment's replica count",
			Long:    "Scaling down terminates running pods, and --replicas 0 takes the workload offline. The handler rejects a count above 1000.",
			Example: "mcharbor k8s deployment scale default web --replicas 3 --yes",
		},
		{
			// Disruptive: a rolling restart replaces every pod. The rollout is
			// gradual, but every connection an old pod was serving ends.
			Path:    []string{"deployment", "restart"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/kubernetes/deployments/{namespace}/{name}/restart",
			Args:    []string{"namespace", "name"},
			Flags:   []plugin.CLIFlag{clusterFlag()},
			Needs:   "kubernetes.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Roll a deployment's pods",
			Long:    "Annotates the pod template so Kubernetes replaces every pod. The rollout is gradual, but every connection an old pod held ends.",
			Example: "mcharbor k8s deployment restart default web --yes",
		},
		{
			// Irreversible: the deployment object and every pod it owns are
			// deleted, and the spec is not recoverable from McHarbor.
			Path:    []string{"deployment", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/kubernetes/deployments/{namespace}/{name}",
			Args:    []string{"namespace", "name"},
			Flags:   []plugin.CLIFlag{clusterFlag()},
			Needs:   "kubernetes.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Delete a deployment",
			Long:    "Deletes the deployment and every pod it owns. McHarbor holds no copy of the spec, so recreating it means having the manifest elsewhere.",
			Example: "mcharbor k8s deployment delete default web --confirm web",
		},
	}
}
