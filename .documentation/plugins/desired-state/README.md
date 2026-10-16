# Desired state for plugins

An optional contract that lets the [reconciler](../../screens/reconciler/README.md) watch
and correct what a plugin manages (a virtual machine's power and size, a Swarm service's
replicas and image, a Kubernetes Deployment's scale) with the same observe, propose and
enforce modes, limits, plans and audit trail as containers.

The contract lives in `core/plugin/specs/desiredstate`. It is a separate package, so adding
it changed no already-built plugin, and a plugin that does not implement it is unaffected.

## Which plugins implement it

| Plugin | Kind | Properties that can be pinned |
|---|---|---|
| Virtual Machines (`addon-kvm`) | `virtual_machine` | `power` (running/stopped), `vcpus`, `memory_mib`, `cpu_model`, `autostart` |
| System Containers (`addon-lxc`) | `system_container` | `power` (running/stopped), `cpu_max` (cgroup `cpu.max`, e.g. `150000 100000` = 1.5 CPUs), `memory_max` (MiB), `autostart` |
| Swarm (`addon-swarm`) | `swarm_service` | `replicas` (replicated services), `image`, `update_state` |
| Swarm (`addon-swarm`) | `swarm_node` | `availability` (active/pause/drain) |
| Kubernetes (`addon-kubernetes`) | `k8s_deployment` | `replicas`, `image` (first container), `paused` |
| Kubernetes (`addon-kubernetes`) | `k8s_node` | `schedulable` (cordon/uncordon) |

## The contract

| Method | Used for |
|---|---|
| `DesiredKinds()` | The kinds and their properties, as the form shows them |
| `DesiredSubjects(kind, scope)` | What can be picked: VMs, services, Deployments, nodes |
| `DesiredOptions(kind, field, scope)` | Choices for a property whose options depend on the scope (Swarm images) |
| `CurrentSpec(kind, scope, subject)` | **Use current values** in the form |
| `DetectDrift(kind, scope, subject, spec)` | About once a minute: one difference per pinned property that departs |
| `PlanDrift(...)` | The steps that would restore the spec, each automatic or manual, and marked disruptive when it interrupts service |
| `ApplyStep(...)` | Runs one approved step through the plugin's own service methods, with the reconciler as the actor |

A plugin whose subjects live outside McHarbor environments also implements `ScopeProvider`
(`DesiredScopes`), and the form lists its scopes (Kubernetes clusters) instead of
environments.

## Rules the host applies

- A property left empty is **not managed** and never compared.
- A subject, host or cluster the plugin cannot reach is **unknown**, never drift.
- **Enforce** applies only plans whose steps are all automatic, within the per-pass budget,
  and **never a disruptive step**: a stop, an image rollout, a scale-down, a drain or a
  cordon is written as a plan under **Plans** for a person to approve.
- Git sources declare Docker containers and stacks only; plugin kinds are created in the
  interface or through `POST /api/reconciler`.

## What goes wrong

| Symptom | Cause |
|---|---|
| A plugin's kinds are missing from **What to watch** | The plugin is not installed, or its build predates the contract. Update it from the Plugin Store |
| A watch stays *Unknown* | The plugin, host or cluster is unreachable, or the plugin is not loaded |
| Enforce writes a plan instead of acting | A step is manual or disruptive, or the pass budget is used up |
