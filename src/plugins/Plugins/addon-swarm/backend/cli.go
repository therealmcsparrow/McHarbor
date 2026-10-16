// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// cliRoot is the command group this plugin owns. It cannot be `stack`, which is
// what an operator would reach for first: core already owns `stack` and `stacks`
// for Compose, so Swarm's own stacks live under `swarm stack`.
const cliRoot = "swarm"

// logFlags are the shared parameters of a service or task log tail. The tail is
// bounded server-side at 5000 lines, and this is a one-shot read rather than a
// follow: no Swarm route streams.
func logFlags() []plugin.CLIFlag {
	return []plugin.CLIFlag{
		{Name: "tail", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "tail", Default: "200", Help: "lines from the end, 1-5000"},
		{Name: "since", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "since", Help: "RFC3339 timestamp or a Go duration, e.g. 10m"},
		{Name: "stdout", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "stdout", Default: "true", Help: "include stdout"},
		{Name: "stderr", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "stderr", Default: "true", Help: "include stderr"},
	}
}

// serviceColumns is the service listing's rendering. Running against desired is
// the pair that answers "is this converged", so both are columns rather than one
// derived state.
func serviceColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "MODE", Path: "mode"},
		{Header: "REPLICAS", Path: "replicas"},
		{Header: "RUNNING", Path: "runningTasks"},
		{Header: "DESIRED", Path: "desiredTasks"},
		{Header: "IMAGE", Path: "image"},
		{Header: "STACK", Path: "stackName"},
		{Header: "UPDATE", Path: "updateState"},
	}
}

// payloadColumns is the shared rendering of a secret and a config listing. A
// secret's payload is absent by construction — Docker's API is write-only for it
// — so there is no field here that could print one.
func payloadColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "STACK", Path: "stackName"},
		{Header: "IN USE BY", Path: "inUseBy"},
		{Header: "AGE", Path: "createdAt", Format: "age"},
	}
}

// RegisterCLI declares the `swarm` command group. Every Route is the string
// RegisterRoutes hands the host, so renaming a route without the command is a
// compile error here rather than a 404 an operator discovers.
//
// Every command is EnvScoped: all forty-six handlers resolve their daemon
// through response.ParseEnvID, so a command without --env would silently address
// whichever environment the context happens to hold.
func (p *SwarmPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:    cliRoot,
		Short:   "Manage a Docker Swarm cluster, its services, nodes, stacks and secrets",
		Aliases: []string{"swarms"},
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
// Forty-six commands for the plugin's forty-six routes: full parity, because
// every Swarm route is a plain request/response an operator could want in a
// runbook.
func cliCommands() []plugin.CLICommand {
	out := make([]plugin.CLICommand, 0, 46)
	out = append(out, swarmClusterCommands()...)
	out = append(out, swarmServiceCommands()...)
	out = append(out, swarmTaskCommands()...)
	out = append(out, swarmNodeCommands()...)
	out = append(out, swarmPayloadCommands()...)
	out = append(out, swarmStackCommands()...)
	return out
}

// swarmClusterCommands covers the cluster itself and its two credentials: twelve
// commands.
func swarmClusterCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"cluster", "status"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm",
			Needs:     "swarm.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show the cluster's own configuration",
			// The join tokens are deliberately absent from this response:
			// possession of a manager token is control of the cluster, so they
			// travel only on their own route.
			Long:    "Carries no join token. Possession of a manager token is control of the cluster, so tokens are served only by `swarm join-token get`.",
			Example: "mcharbor swarm cluster status --env env_2f81",
		},
		{
			Path:      []string{"cluster", "overview"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/overview",
			Needs:     "swarm.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show node, service, task and stack counts in one response",
			Example:   "mcharbor swarm cluster overview --env env_2f81",
		},
		{
			// Needs swarm.view, not nothing. The source spec recorded this route
			// as having no permission; declaring it empty short-circuited the
			// plugin permission middleware to a pass-through, which made the
			// probe reachable by any authenticated user whatever their role. The
			// manifest now declares swarm.view and this mirrors it, because the
			// host refuses a command that disagrees.
			Path:      []string{"cluster", "capability"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/capability",
			Needs:     "swarm.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Report whether this host runs a Swarm, and its local role",
			Long:      "Needs swarm.view like every other read. It returns a boolean and the local node's role and leaks nothing about workloads.",
			Example:   "mcharbor swarm cluster capability --env env_2f81",
		},
		{
			Path:      []string{"cluster", "update"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPut,
			Route:     "/api/swarm",
			Body:      plugin.BodyFlags,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "task-history-limit", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "taskHistoryLimit", Help: "task history entries kept per slot"},
				{Name: "autolock", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "autoLockManagers", Help: "require an unlock key after a manager restarts"},
				{Name: "cert-expiry-hours", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "certExpiryHours", Help: "node certificate lifetime in hours"},
				{Name: "snapshot-interval", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "snapshotInterval", Help: "raft log entries between snapshots"},
			},
			Needs: "swarm.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Change the cluster's configuration",
			// Disruptive: turning autolock on means the next manager restart
			// needs a key the operator must already have saved.
			Long:    "An omitted flag is left alone. Turning --autolock on means the next manager restart will not come back without the unlock key — read it first with `swarm unlock-key get`.",
			Example: "mcharbor swarm cluster update --env env_2f81 --task-history-limit 10 --yes",
		},
		{
			Path:      []string{"cluster", "init"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/init",
			Body:      plugin.BodyBoth,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "listen-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "listenAddr", Help: "address to listen on, e.g. 0.0.0.0:2377"},
				{Name: "advertise-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "advertiseAddr", Help: "address other nodes reach this one on"},
				{Name: "data-path-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "dataPathAddr", Help: "address for overlay data traffic"},
				{Name: "force-new-cluster", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "forceNewCluster", Help: "recover a lost quorum from this node's raft log"},
				{Name: "autolock", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "autoLockManagers", Help: "require an unlock key after a manager restarts"},
				{Name: "default-addr-pool", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "defaultAddrPool", Help: "CIDR pools overlay networks are carved from"},
				{Name: "subnet-size", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "subnetSize", Help: "prefix length of each carved subnet"},
			},
			Needs: "swarm.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Turn this host into a single-node Swarm manager",
			// Disruptive rather than safe: --force-new-cluster rewrites the raft
			// log and discards every other manager's view of the cluster.
			Long:    "--force-new-cluster recovers a cluster that has lost quorum by discarding every other manager's raft state. Use it only when the others are gone for good.",
			Example: "mcharbor swarm cluster init --env env_2f81 --advertise-addr 10.0.0.11 --yes",
		},
		{
			Path:      []string{"cluster", "join"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/join",
			Body:      plugin.BodyBoth,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "remote-addr", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "remoteAddrs", Help: "manager addresses to join through"},
				{Name: "listen-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "listenAddr", Help: "address to listen on"},
				{Name: "advertise-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "advertiseAddr", Help: "address other nodes reach this one on"},
				{Name: "data-path-addr", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "dataPathAddr", Help: "address for overlay data traffic"},
			},
			Needs: "swarm.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Join this host to an existing Swarm",
			// The token is not a flag. It is equivalent to control of the
			// cluster and a flag lands in the shell history.
			Long:    "Pass the join token through --data <file>, never as a flag: a manager token is equivalent to control of the cluster and a flag lands in the shell history.",
			Example: "mcharbor swarm cluster join --env env_2f81 --remote-addr 10.0.0.11:2377 --data ./join.json --yes",
		},
		{
			// Irreversible: a manager that leaves takes its raft state with it,
			// and the last manager leaving destroys the cluster and every
			// service, secret and config definition in it.
			Path:      []string{"cluster", "leave"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/leave",
			Body:      plugin.BodyFlags,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "force", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "force", Help: "leave even as the last manager, destroying the cluster"},
			},
			Needs:   "swarm.manage",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove this host from the Swarm",
			Long:    "--force on the last manager destroys the cluster: every service, secret, config and stack definition goes with it, and McHarbor holds no copy.",
			Example: "mcharbor swarm cluster leave --env env_2f81 --confirm env_2f81",
		},
		{
			Path:      []string{"cluster", "unlock"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/unlock",
			Body:      plugin.BodyBoth,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "unlock-key", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "unlockKey", Help: "the autolock key; prefer --data <file>"},
			},
			Needs: "swarm.manage",
			Risk:  plugin.RiskSafe,
			Short: "Unlock a manager that restarted with autolock on",
			// Safe: it restores a manager that is already down. Nothing is
			// interrupted and nothing is destroyed by trying.
			Long:    "Safe by consequence: the manager is already locked, and a wrong key changes nothing. Prefer --data <file> so the key stays out of the shell history.",
			Example: "mcharbor swarm cluster unlock --env env_2f81 --data ./unlock.json",
		},
		{
			Path:      []string{"unlock-key", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/unlock-key",
			Needs:     "swarm.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Read the cluster's autolock key",
			// A GET on swarm.manage rather than swarm.view: this is a credential
			// read, and the key is what brings a locked manager back.
			Long:    "Needs swarm.manage although it is a read: the key is the credential that brings a locked manager back. Save it somewhere a restart cannot take with it.",
			Example: "mcharbor swarm unlock-key get --env env_2f81",
		},
		{
			Path:      []string{"unlock-key", "rotate"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/unlock-key/rotate",
			Needs:     "swarm.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Rotate the cluster's autolock key",
			// Disruptive: every saved copy of the old key stops working, and a
			// manager that restarts before the new one is recorded stays locked.
			Long:    "The old key stops working immediately. Read the new one with `swarm unlock-key get` and store it before any manager restarts.",
			Example: "mcharbor swarm unlock-key rotate --env env_2f81 --yes",
		},
		{
			// get, not list: the route returns one JoinTokens object with a
			// worker and a manager token, not an array of anything.
			Path:      []string{"join-token", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/join-tokens",
			Needs:     "swarm.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Read the worker and manager join tokens",
			Long:      "Needs swarm.manage although it is a read: a manager token is equivalent to control of the cluster.",
			Example:   "mcharbor swarm join-token get --env env_2f81",
		},
		{
			Path:      []string{"join-token", "rotate"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/join-tokens/{kind}/rotate",
			Args:      []string{"kind"},
			Needs:     "swarm.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Rotate the worker or the manager join token",
			// Disruptive: any automation holding the old token — an autoscaler,
			// a provisioning script — stops being able to add nodes.
			Long:    "Takes `worker` or `manager`. Anything holding the old token, including provisioning automation, can no longer add nodes.",
			Example: "mcharbor swarm join-token rotate worker --env env_2f81 --yes",
		},
	}
}

// swarmServiceCommands covers services: nine commands.
func swarmServiceCommands() []plugin.CLICommand {
	writeFlags := []plugin.CLIFlag{
		{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "service name"},
		{Name: "image", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "image", Help: "image reference"},
		{Name: "mode", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "mode", Help: "replicated or global"},
		{Name: "replicas", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "replicas", Help: "replica count, for a replicated service"},
		{Name: "env-var", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "env", Help: "environment entries, KEY=value; not --env, which selects the McHarbor environment"},
		{Name: "constraint", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "constraints", Help: "placement constraints"},
		{Name: "network", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "networks", Help: "overlay networks to attach"},
		{Name: "limit-memory-bytes", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "limitMemoryBytes", Help: "memory limit in bytes"},
		{Name: "limit-nano-cpus", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "limitNanoCpus", Help: "CPU limit in nanocpus; 1e9 is one core"},
	}
	return []plugin.CLICommand{
		{
			Path:      []string{"service", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/services",
			Needs:     "swarm_services.view",
			Risk:      plugin.RiskSafe,
			Columns:   serviceColumns(),
			EnvScoped: true,
			Short:     "List services",
			Example:   "mcharbor swarm service list --env env_2f81",
		},
		{
			Path:      []string{"service", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/services/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_services.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one service",
			Example:   "mcharbor swarm service get web --env env_2f81",
		},
		{
			Path:      []string{"service", "env"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/services/{id}/env",
			Args:      []string{"id"},
			Needs:     "swarm_services.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show a service's environment values in the clear (audited)",
			Long:      "`service get` masks credential-looking values. This reads them, needs swarm_services.manage and writes an audit entry; McHarbor's own keys stay masked.",
			Example:   "mcharbor swarm service env web --env env_2f81",
		},
		{
			// text: the tail is the point of the command, so the body goes to
			// stdout rather than through the object renderer.
			Path:      []string{"service", "logs"},
			Kind:      plugin.CLIText,
			Method:    http.MethodGet,
			Route:     "/api/swarm/services/{id}/logs",
			Args:      []string{"id"},
			Flags:     logFlags(),
			Needs:     "swarm_services.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Print a service's aggregated log tail",
			Long:      "A bounded one-shot read, not a follow: no Swarm route streams. --tail is capped at 5000 lines.",
			Example:   "mcharbor swarm service logs web --env env_2f81 --tail 500",
		},
		{
			Path:      []string{"service", "create"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/services",
			Body:      plugin.BodyBoth,
			Flags:     writeFlags,
			Needs:     "swarm_services.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Create a service",
			Long:      "The flags cover a small subset of the spec on purpose. Anything they cannot express — mounts, secrets, healthchecks — goes through `swarm stack deploy` or --data <file>.",
			Example:   "mcharbor swarm service create --env env_2f81 --name web --image nginx:1 --replicas 3",
		},
		{
			Path:      []string{"service", "update"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPut,
			Route:     "/api/swarm/services/{id}",
			Args:      []string{"id"},
			Body:      plugin.BodyBoth,
			Flags:     writeFlags,
			Needs:     "swarm_services.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Change a service",
			// Disruptive: an update is a rollout, so every task is replaced and
			// every connection an old task held ends.
			Long:    "An update is a rollout: every task is replaced. Nothing is destroyed, so `swarm service rollback` can put the previous spec back.",
			Example: "mcharbor swarm service update web --env env_2f81 --image nginx:2 --yes",
		},
		{
			Path:      []string{"service", "restart"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/services/{id}/restart",
			Args:      []string{"id"},
			Needs:     "swarm_services.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Force a service's tasks to be replaced",
			Long:      "Bumps the spec so Swarm reschedules every task. The rollout is gradual, but every connection an old task held ends.",
			Example:   "mcharbor swarm service restart web --env env_2f81 --yes",
		},
		{
			Path:      []string{"service", "rollback"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/services/{id}/rollback",
			Args:      []string{"id"},
			Needs:     "swarm_services.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Roll a service back to its previous spec",
			// Disruptive rather than safe: it is a rollout like any other, and
			// there is only one previous spec — rolling back twice does not
			// reach the one before that.
			Long:    "Another rollout, in the other direction. Swarm keeps one previous spec, so this is not an undo stack.",
			Example: "mcharbor swarm service rollback web --env env_2f81 --yes",
		},
		{
			Path:      []string{"service", "scale"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/services/{id}/scale",
			Args:      []string{"id"},
			Body:      plugin.BodyFlags,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "replicas", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "replicas", Required: true, Help: "desired replica count"},
			},
			// Its own key, not swarm_services.manage: scaling is the one service
			// change a site may want to delegate without handing over the spec.
			Needs:   "swarm_services.scale",
			Risk:    plugin.RiskDisruptive,
			Short:   "Set a service's replica count",
			Long:    "Needs swarm_services.scale, a key of its own. --replicas 0 takes the service offline without deleting it.",
			Example: "mcharbor swarm service scale web --env env_2f81 --replicas 0 --yes",
		},
		{
			// Irreversible: the service spec is gone from the cluster, and
			// McHarbor holds no copy of it. A stack-managed service can be
			// redeployed from its Compose file; a hand-made one cannot.
			Path:      []string{"service", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/swarm/services/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_services.delete",
			Risk:      plugin.RiskIrreversible,
			EnvScoped: true,
			Short:     "Remove a service",
			Long:      "The spec leaves the cluster and McHarbor keeps no copy. Recreating a hand-made service means having its definition elsewhere.",
			Example:   "mcharbor swarm service delete web --env env_2f81 --confirm web",
		},
	}
}

// swarmTaskCommands covers tasks, which are read-only throughout: three
// commands. A task is not something an operator acts on — the service it belongs
// to is.
func swarmTaskCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"task", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/tasks",
			Needs:     "swarm_tasks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "service", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "service", Help: "narrow to one service id"},
				{Name: "node", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "node", Help: "narrow to one node id"},
				{Name: "state", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "state", Help: "narrow to one desired state, e.g. running"},
			},
			Short:   "List tasks",
			Example: "mcharbor swarm task list --env env_2f81 --state running",
			Columns: []plugin.CLIColumn{
				{Header: "ID", Path: "id"},
				{Header: "NAME", Path: "name"},
				{Header: "SERVICE", Path: "serviceName"},
				{Header: "NODE", Path: "nodeName"},
				{Header: "SLOT", Path: "slot"},
				{Header: "DESIRED", Path: "desiredState"},
				{Header: "STATE", Path: "state"},
				{Header: "ERROR", Path: "error"},
				{Header: "AGE", Path: "createdAt", Format: "age"},
			},
		},
		{
			Path:      []string{"task", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/tasks/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_tasks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one task",
			Example:   "mcharbor swarm task get kq81m2 --env env_2f81",
		},
		{
			Path:      []string{"task", "logs"},
			Kind:      plugin.CLIText,
			Method:    http.MethodGet,
			Route:     "/api/swarm/tasks/{id}/logs",
			Args:      []string{"id"},
			Flags:     logFlags(),
			Needs:     "swarm_tasks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Print one task's log tail",
			Long:      "The single-replica view of `swarm service logs`: this is the one task, not the service's aggregate.",
			Example:   "mcharbor swarm task logs kq81m2 --env env_2f81",
		},
	}
}

// swarmNodeCommands covers cluster members: six commands.
func swarmNodeCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"node", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/nodes",
			Needs:     "swarm_nodes.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "List cluster nodes",
			Example:   "mcharbor swarm node list --env env_2f81",
			Columns: []plugin.CLIColumn{
				{Header: "ID", Path: "id"},
				{Header: "HOSTNAME", Path: "hostname"},
				{Header: "ROLE", Path: "role"},
				{Header: "AVAILABILITY", Path: "availability"},
				{Header: "STATE", Path: "state"},
				{Header: "LEADER", Path: "isLeader", Format: "bool"},
				{Header: "TASKS", Path: "taskCount"},
				{Header: "ENGINE", Path: "engineVersion"},
			},
		},
		{
			Path:      []string{"node", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/nodes/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_nodes.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one node",
			Example:   "mcharbor swarm node get n1a2b3 --env env_2f81",
		},
		{
			Path:      []string{"node", "update"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPut,
			Route:     "/api/swarm/nodes/{id}",
			Args:      []string{"id"},
			Body:      plugin.BodyFile,
			Needs:     "swarm_nodes.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Replace a node's labels",
			// The body is a whole label map, so it comes from a file rather than
			// from flags: a partial flag set would silently drop the labels it
			// did not name.
			Long:    "Takes the whole label map through --data <file>, because it replaces rather than merges. Placement constraints read these labels, so a dropped label can strand a service.",
			Example: "mcharbor swarm node update n1a2b3 --env env_2f81 --data ./labels.json --yes",
		},
		{
			Path:      []string{"node", "availability"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/nodes/{id}/availability",
			Args:      []string{"id"},
			Body:      plugin.BodyFlags,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "to", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "availability", Required: true, Help: "active, pause or drain"},
			},
			Needs: "swarm_nodes.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Set a node's availability",
			// Disruptive: `drain` evacuates every task on the node, which is the
			// whole point and is still an interruption of everything running on
			// it.
			Long:    "`drain` evacuates every task on the node onto the others; `pause` only stops new ones being scheduled. Draining a node the cluster has no spare capacity for leaves tasks pending.",
			Example: "mcharbor swarm node availability n1a2b3 --env env_2f81 --to drain --yes",
		},
		{
			Path:      []string{"node", "role"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/nodes/{id}/role",
			Args:      []string{"id"},
			Body:      plugin.BodyFlags,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "to", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "role", Required: true, Help: "manager or worker"},
			},
			Needs: "swarm_nodes.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Promote a node to manager, or demote it to worker",
			// Disruptive: demoting a manager changes the raft quorum, and
			// demoting enough of them loses it.
			Long:    "Demoting a manager changes the raft quorum. Demoting below a majority of the remaining managers loses quorum, and recovery then needs `cluster init --force-new-cluster`.",
			Example: "mcharbor swarm node role n1a2b3 --env env_2f81 --to manager --yes",
		},
		{
			// Irreversible: the node leaves the cluster and its certificate is
			// revoked, so it can only come back by joining again with a token.
			Path:      []string{"node", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/swarm/nodes/{id}",
			Args:      []string{"id"},
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "force", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "force", Help: "remove a node that is still reachable"},
			},
			Needs:   "swarm_nodes.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove a node from the cluster",
			Long:    "The node's certificate is revoked; it can only return by joining again with a token. --force removes one that is still up, and its tasks are not drained first.",
			Example: "mcharbor swarm node delete n1a2b3 --env env_2f81 --confirm n1a2b3",
		},
	}
}

// swarmPayloadCommands covers secrets and configs: ten commands over two objects
// that differ in exactly one way — Docker returns a config's data and never a
// secret's.
func swarmPayloadCommands() []plugin.CLICommand {
	createFlags := []plugin.CLIFlag{
		{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Required: true, Help: "name, unique in the cluster"},
		{Name: "data", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "data", Help: "base64 payload; prefer --data <file> for anything real"},
	}
	return []plugin.CLICommand{
		{
			Path:      []string{"secret", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/secrets",
			Needs:     "swarm_secrets.view",
			Risk:      plugin.RiskSafe,
			Columns:   payloadColumns(),
			EnvScoped: true,
			Short:     "List secrets",
			Long:      "Metadata only. Docker's API is write-only for a secret's payload, so no route in the product can return one.",
			Example:   "mcharbor swarm secret list --env env_2f81",
		},
		{
			Path:      []string{"secret", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/secrets/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_secrets.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one secret's metadata",
			Example:   "mcharbor swarm secret get db-password --env env_2f81",
		},
		{
			Path:      []string{"secret", "create"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/secrets",
			Body:      plugin.BodyBoth,
			Flags:     createFlags,
			Needs:     "swarm_secrets.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Create a secret",
			Long:      "The payload is base64 in the body. Pass it through --data <file>: a secret on a command line lands in the shell history, and it cannot be read back afterwards to check.",
			Example:   "mcharbor swarm secret create --env env_2f81 --data ./secret.json",
		},
		{
			Path:      []string{"secret", "update"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPut,
			Route:     "/api/swarm/secrets/{id}",
			Args:      []string{"id"},
			Body:      plugin.BodyFile,
			Needs:     "swarm_secrets.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Replace a secret's labels",
			// Docker refuses a SecretUpdate that changes anything but the
			// labels, so this is a label write and nothing else.
			Long:    "Labels only — Docker rejects a secret update that changes the payload. Rotating a secret means creating a new one and updating the services that mount it.",
			Example: "mcharbor swarm secret update db-password --env env_2f81 --data ./labels.json --yes",
		},
		{
			// Irreversible: the payload is unreadable through the API, so
			// deleting a secret destroys the only copy the cluster had.
			Path:      []string{"secret", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/swarm/secrets/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_secrets.delete",
			Risk:      plugin.RiskIrreversible,
			EnvScoped: true,
			Short:     "Remove a secret",
			Long:      "Nothing can read a secret's payload back, so this destroys the cluster's only copy. Docker refuses to remove one a service still mounts.",
			Example:   "mcharbor swarm secret delete db-password --env env_2f81 --confirm db-password",
		},
		{
			Path:      []string{"config", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/configs",
			Needs:     "swarm_configs.view",
			Risk:      plugin.RiskSafe,
			Columns:   payloadColumns(),
			EnvScoped: true,
			Short:     "List configs",
			Example:   "mcharbor swarm config list --env env_2f81",
		},
		{
			Path:      []string{"config", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/configs/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_configs.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one config, with its payload",
			// The one asymmetry with secrets: ConfigInspect does return the
			// data, so the single read carries it and the listing does not.
			Long:    "Unlike a secret, a config's base64 payload is returned here — and only here; the listing never carries it.",
			Example: "mcharbor swarm config get nginx-conf --env env_2f81",
		},
		{
			Path:      []string{"config", "create"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/configs",
			Body:      plugin.BodyBoth,
			Flags:     createFlags,
			Needs:     "swarm_configs.manage",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Create a config",
			Example:   "mcharbor swarm config create --env env_2f81 --data ./config.json",
		},
		{
			Path:      []string{"config", "update"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPut,
			Route:     "/api/swarm/configs/{id}",
			Args:      []string{"id"},
			Body:      plugin.BodyFile,
			Needs:     "swarm_configs.manage",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Replace a config's labels",
			Long:      "Labels only — Docker rejects a config update that changes the payload. Changing content means a new config and a service update.",
			Example:   "mcharbor swarm config update nginx-conf --env env_2f81 --data ./labels.json --yes",
		},
		{
			// Irreversible: a config's payload is readable, but only from the
			// cluster. Once removed McHarbor holds no copy of it.
			Path:      []string{"config", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/swarm/configs/{id}",
			Args:      []string{"id"},
			Needs:     "swarm_configs.delete",
			Risk:      plugin.RiskIrreversible,
			EnvScoped: true,
			Short:     "Remove a config",
			Long:      "McHarbor keeps no copy of the payload. Read it with `swarm config get` first if you might need it. Docker refuses to remove one a service still mounts.",
			Example:   "mcharbor swarm config delete nginx-conf --env env_2f81 --confirm nginx-conf",
		},
	}
}

// swarmStackCommands covers stacks and the overlay networks they create: five
// commands.
func swarmStackCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"stack", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/stacks",
			Needs:     "swarm_stacks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "List stacks",
			Long:      "A stack is a set of services sharing a com.docker.stack.namespace label; nothing in the cluster stores it as an object.",
			Example:   "mcharbor swarm stack list --env env_2f81",
			Columns: []plugin.CLIColumn{
				{Header: "NAME", Path: "name"},
				{Header: "SERVICES", Path: "services"},
				{Header: "RUNNING", Path: "runningTasks"},
				{Header: "DESIRED", Path: "desiredTasks"},
				{Header: "NETWORKS", Path: "networks"},
				{Header: "AGE", Path: "createdAt", Format: "age"},
			},
		},
		{
			Path:      []string{"stack", "get"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/swarm/stacks/{name}",
			Args:      []string{"name"},
			Needs:     "swarm_stacks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show one stack and its member services",
			Example:   "mcharbor swarm stack get shop --env env_2f81",
		},
		{
			Path:      []string{"stack", "deploy"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodPost,
			Route:     "/api/swarm/stacks",
			Body:      plugin.BodyBoth,
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Required: true, Help: "stack name"},
				{Name: "compose", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "compose", Help: "inline Compose document; prefer --data <file>"},
				{Name: "prune", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "prune", Help: "remove services the document no longer declares"},
			},
			Needs: "swarm_stacks.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Deploy or redeploy a stack from a Compose document",
			// Disruptive rather than safe: a redeploy is a rollout across every
			// service in the stack, and --prune removes the ones the document
			// dropped.
			Long:    "A redeploy rolls every changed service. --prune removes services the document no longer declares, so a truncated file with --prune deletes the rest of the stack. The result reports per-service success, so a partial deploy is visible rather than one opaque error.",
			Example: "mcharbor swarm stack deploy --env env_2f81 --name shop --data ./stack.json --yes",
		},
		{
			// Irreversible: every service, and with it every task, in the
			// namespace goes. The Compose document is the operator's, not
			// McHarbor's, so nothing here can put it back.
			Path:      []string{"stack", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/swarm/stacks/{name}",
			Args:      []string{"name"},
			Needs:     "swarm_stacks.delete",
			Risk:      plugin.RiskIrreversible,
			EnvScoped: true,
			Short:     "Remove a stack and every service in it",
			Long:      "Every service in the namespace is removed. McHarbor does not keep the Compose document, so redeploying means still having the file.",
			Example:   "mcharbor swarm stack delete shop --env env_2f81 --confirm shop",
		},
		{
			Path:      []string{"network", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/swarm/networks",
			Needs:     "swarm_networks.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "List swarm-scoped overlay networks",
			// Read-only, and deliberately so: the plugin registers no create or
			// delete route for an overlay network, because a stack deploy is how
			// one comes into existence.
			Long:    "Read-only: overlay networks are created and removed by a stack deploy, and the plugin exposes no route to do it directly.",
			Example: "mcharbor swarm network list --env env_2f81",
			Columns: []plugin.CLIColumn{
				{Header: "ID", Path: "id"},
				{Header: "NAME", Path: "name"},
				{Header: "DRIVER", Path: "driver"},
				{Header: "SCOPE", Path: "scope"},
				{Header: "SUBNETS", Path: "subnets"},
				{Header: "ATTACHABLE", Path: "attachable", Format: "bool"},
				{Header: "INGRESS", Path: "ingress", Format: "bool"},
				{Header: "STACK", Path: "stackName"},
			},
		},
	}
}
