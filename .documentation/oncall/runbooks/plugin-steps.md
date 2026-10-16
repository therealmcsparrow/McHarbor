# Plugin steps

A runbook step can act on objects that belong to a plugin: a virtual machine, a system
container, a storage pool, a swarm service or node, a Kubernetes pod or deployment. The host
does not import any plugin to do it. Each plugin step names one route the plugin already
declares in its `plugin.yaml`, and the runbook engine calls that route **in-process, as the
person following the runbook** (or, for an unattended run, as its service principal), exactly
as their browser would.

## How a plugin step is authorised

A plugin step passes two gates, and both are the plugin's own key:

1. **The runbook gate.** Before anything is called, the engine checks the step's key with
   `HasPermission` against whoever is running it, for the run's environment. A refusal answers
   `403` with `err.runbook.step_forbidden` and the key, and **no request reaches the plugin**.
2. **The plugin's gate.** The request then goes through the plugin router's own permission
   middleware (the same one every browser call to that route meets) with the operator as
   the request's user. A policy bound to the object, or a key held in a different environment,
   can still refuse it there. That refusal is recorded on the step as
   *&lt;plugin&gt; refused the request (403)* and, in an unattended run, stops the run.

The request is dispatched into the **plugin router**, not the host router. That matters: the
System Containers plugin declares `/api/containers/{id}/start`, which is also the Docker
route. Through the public `/api` tree that path reaches Docker; a runbook step always reaches
the plugin that declared it.

An unattended run carries the **service principal** as the request's user, so the plugin's
middleware evaluates the principal's roles, never a person's, and never a bypass. If an MFA
enrolment policy covers the principal, the plugin route answers `403` and the step fails with
that reason.

## When the plugin is not there

- The authoring picker lists a plugin's steps and attach targets **only while the plugin is
  loaded** (installed, enabled, and serving routes). `GET /api/runbooks/catalog` answers with
  the loaded ones and `targets`, each loaded selector kind with the label its plugin gave it.
- A runbook written while the plugin was installed keeps its steps. If the plugin is removed
  or disabled later, the step fails when pressed with *the &lt;slug&gt; plugin is not installed
  or not loaded, so &lt;key&gt; cannot run*, and nothing is called.

## Parameters

Each entry declares typed parameters. The editor renders them as selectors, number fields
and switches; the server refuses a parameter the entry does not declare, an option outside
the declared set, a number outside its range, and an empty required value
(`err.invalid_body`).

| Type | Editor control | Examples |
|---|---|---|
| target | a selector listing the objects from the plugin, with **Enter manually…** | `vm`, `ct`, `pool`, `service`, `node`, `cluster`, `pod`, `deployment` |
| enum | a selector | `kind` (`full`, `incremental`) |
| int | a number field | `replicas` (0 to 1000), `graceSeconds` (0 to 3600) |
| string | a text field | `name` |
| bool | a switch | `withMemory` |

A target parameter left empty is the run's own subject: a runbook attached to a virtual
machine, or started from an alert about one, acts on that machine. A pod or deployment is
written `namespace/name`. Every value is path-escaped, so a parameter can never add a path
segment and reach a route the step does not name.

The selectors read `GET /api/runbooks/targets?kind=<kind>&env=<id>[&cluster=<id>]`
(`runbooks.view`), which asks the owning plugin's list route as the operator, so the list
shows only what the operator's own view key reaches.

## The steps

### Virtual machines: `addon-kvm`

| Step | Kind | Calls | Needs |
|---|---|---|---|
| `vm.state` | Diagnostic | `GET /api/vms/{id}` | `vm.view` |
| `vm.snapshots` | Diagnostic | `GET /api/vms/{id}/snapshots` | `vm.view` |
| `vm.start` | Action | `POST /api/vms/{id}/start` | `vm.manage` |
| `vm.shutdown` | Action | `POST /api/vms/{id}/shutdown` (`graceSeconds`) | `vm.manage` |
| `vm.reset` | Action | `POST /api/vms/{id}/reset`, a hard reset; the plugin has no soft reboot | `vm.manage` |
| `vm.force_stop` | Action | `POST /api/vms/{id}/stop` | `vm.manage` |
| `vm.snapshot.create` | Action | `POST /api/vms/{id}/snapshots` (`name`, `withMemory`) | `vm.manage` |

### System containers: `addon-lxc`

| Step | Kind | Calls | Needs |
|---|---|---|---|
| `ct.state` | Diagnostic | `GET /api/containers/{id}` | `ct.view` |
| `ct.backups` | Diagnostic | `GET /api/containers/{id}/backups?limit=20` | `ct.backup` |
| `ct.start` | Action | `POST /api/containers/{id}/start` | `ct.manage` |
| `ct.stop` | Action | `POST /api/containers/{id}/stop` | `ct.manage` |
| `ct.restart` | Action | `POST /api/containers/{id}/restart` | `ct.manage` |
| `ct.freeze` | Action | `POST /api/containers/{id}/freeze` | `ct.manage` |
| `ct.unfreeze` | Action | `POST /api/containers/{id}/unfreeze` | `ct.manage` |
| `ct.backup.run` | Action | `POST /api/containers/{id}/backups` (`kind`) | `ct.backup` |

### Compute: `addon-compute`

| Step | Kind | Calls | Needs |
|---|---|---|---|
| `compute.pool.state` | Diagnostic | `GET /api/compute/pools?env=`, the chosen pool | `compute.view` |
| `compute.capability` | Diagnostic | `GET /api/compute/environments/{envId}/capability` for the run's environment | `compute.view` |
| `compute.capability.probe` | Action | `POST /api/compute/environments/{envId}/probe` | `compute.view` |

### Swarm: `addon-swarm`

| Step | Kind | Calls | Needs |
|---|---|---|---|
| `swarm.service.state` | Diagnostic | `GET /api/swarm/services/{id}?env=` | `swarm_services.view` |
| `swarm.service.tasks` | Diagnostic | `GET /api/swarm/tasks?env=&service=` | `swarm_tasks.view` |
| `swarm.service.logs` | Diagnostic | `GET /api/swarm/services/{id}/logs?env=&tail=150` | `swarm_services.view` |
| `swarm.node.state` | Diagnostic | `GET /api/swarm/nodes/{id}?env=` | `swarm_nodes.view` |
| `swarm.service.scale` | Action | `POST /api/swarm/services/{id}/scale?env=` (`replicas`) | `swarm_services.scale` |
| `swarm.service.restart` | Action | `POST /api/swarm/services/{id}/restart?env=`, a forced update | `swarm_services.manage` |

### Kubernetes: `addon-kubernetes`

| Step | Kind | Calls | Needs |
|---|---|---|---|
| `k8s.pod.state` | Diagnostic | `GET /api/kubernetes/pods/{namespace}/{name}?cluster=` | `kubernetes.view` |
| `k8s.pod.logs` | Diagnostic | `GET /api/kubernetes/pods/{namespace}/{name}/logs?cluster=&tail=150` | `kubernetes.logs` |
| `k8s.deployment.state` | Diagnostic | `GET /api/kubernetes/deployments/{namespace}/{name}?cluster=` | `kubernetes.view` |
| `k8s.deployment.restart` | Action | `POST /api/kubernetes/deployments/{namespace}/{name}/restart?cluster=`, a rollout restart | `kubernetes.manage` |
| `k8s.deployment.scale` | Action | `POST /api/kubernetes/deployments/{namespace}/{name}/scale?cluster=` (`replicas`) | `kubernetes.manage` |
| `k8s.pod.delete` | Action | `DELETE /api/kubernetes/pods/{namespace}/{name}?cluster=` | `kubernetes.delete` |

An empty `cluster` is the plugin's default cluster.

Every action is behind the same confirmation as a container action, executed as you, and
audited twice: once by the runbook (`runbook.step.act` on the run) and once by the plugin's
own route audit.

Diagnostic output is the plugin's answer, pretty-printed, redacted for socket paths and
endpoints and capped at 16 KiB with `[truncated]`, like any other diagnostic.

## Attaching a runbook to a plugin object

**Attach to** offers the four host kinds (alert type, container, stack, environment) and then
every attach kind a loaded plugin declares. The host names none of the plugin kinds: each plugin
declares an `attach` block on the runbook-provider target it owns (see
[Runbook providers](../../development/contracts/capabilities.md#runbook-providers)), and the
editor, the list, the cards and the run page read the catalog.

| Attach kind | Declared by | Attached by |
|---|---|---|
| **Virtual machine** (`vm`) | `addon-kvm`, on the `vm` target | the VM's id |
| **System container** (`ct`) | `addon-lxc`, on the `ct` target | the system container's id |
| **Swarm service** (`swarm_service`) | `addon-swarm`, on the `swarm_service` target | the service's id |
| **Kubernetes deployment** (`k8s_workload`) | `addon-kubernetes`, on the `k8s_deployment` target | `namespace/name` |

Each is offered only while its plugin is loaded. The Kubernetes kind lets a run for a pod or a
deployment act on the steps of either, because the plugin lists `k8s_pod` and `k8s_deployment`
as its subject targets. `GET /api/runbooks/for?kind=<attach kind>&id=<id>` answers "is there a
runbook for this object?", with any run already under way, for the same kinds. The plugins' own
pages do not call it yet: a plugin page cannot import host components, and the host has no slot
on a plugin page to put a link in.

### When the provider is not installed

A runbook stored with a plugin attach kind keeps it after the plugin is removed or disabled. The
kind is then unknown to the catalog, so every surface shows **Provider not installed: &lt;kind&gt;**
in place of the kind's label, and the runbook stays readable. Saving a runbook with a kind no
loaded plugin declares is refused with `err.plugin.not_installed`, and the editor says the plugin
that provides the attach kind is not installed and asks for another attachment. Installing or
enabling the plugin again brings the label back with no change to the runbook.

## Related

- [Runbooks](./README.md): step kinds, how a step is authorised, authoring
- [Unattended runs](./unattended.md): service principals and their roles
- [Command-line client](../../cli/README.md): `runbook catalog` lists every entry with its key
