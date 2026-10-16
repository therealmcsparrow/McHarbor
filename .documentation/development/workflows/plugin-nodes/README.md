# Plugin Nodes

A plugin can contribute workflow nodes. They appear in the palette beside the built-ins, under
a **Plugins** category, and they execute through the plugin runtime rather than through the
host dispatcher.

## The key shape

A plugin node reaches the canvas as:

```
plugin:<slug>:<key>
```

So the Kubernetes add-on's scale node is `plugin:addon-kubernetes:kubernetes-scale-deployment`.
Both halves use the same lowercase, hyphenated alphabet as a built-in key.

That composite key is what the Store toggles availability under, and what a saved canvas
stores. A workflow referencing a plugin node keeps its wiring while the plugin is
uninstalled, and starts working again when it returns.

## Declaring them

Two things, and both are required:

1. **The manifest.** `contributes.nodes` lists the node keys, and the loader mounts only what
   is declared. A node registered in code but missing from `plugin.yaml` is unreachable.
2. **`RegisterNodes`.** The plugin registers each descriptor with the node registry at boot.

```yaml
contributes:
  nodes:
    - "kubernetes-scale-deployment"
    - "kubernetes-deployment-action"
    - "kubernetes-pod-logs"
```

The host exposes them to the frontend from `GET /api/plugins/loader/nodes`, and the editor
merges them into the palette.

## What they get

A plugin node executes inside the host process with the plugin's own `HostContext`: its
database handle, its encryption handle, the Docker and agent pools, audit, RBAC, and the AI
router. That is the difference from a [custom node](../custom-nodes/README.md), which has no
host bridge at all.

A manual run carries the id of the account that pressed Run, so a plugin node that must act as
a user (collecting a report over the API, for instance) can act as the one who started it.
Automatic triggers carry no user.

## What ships today

Every bundled feature plugin contributes nodes:

| Plugin | Nodes |
|---|---|
| Kubernetes | scale deployment, deployment action, delete pod, pod logs, summary, event |
| Docker Swarm | service scale, service action, stack deploy, stack remove, node availability, overview, event |
| Reporting | report generate, export, schedule run, generated, schedule ran |
| Compute Foundation | host capability, pool capacity, ensure image |
| Virtual Machines | lifecycle, snapshot, clone, state, VM trigger |
| System Containers | lifecycle, limits, snapshot, clone, state |

Their fields and ports are documented with the plugin: each plugin's folder under
the plugin section carries them.

## Triggers from a plugin

A plugin can bind its own events to workflow triggers, so a plugin-contributed event starts a
run without the host knowing anything about that event. The binding is resolved by action key
at fire time.

A trigger node registers with no `Inputs` and fires through `HostContext.Workflows.Fire(ctx, key,
payload, match)`: `key` is the plugin-local node key, `payload` becomes `msg.payload`, `topic` is
set to the key, and `match` decides per placement from the node's saved config. `key` is
always the plugin's own: the host fires `plugin:<slug>:<key>` and refuses any other action key,
on the node that fired and again on the leader. Runs start only
on the cluster's workflow-trigger leader. On any other node the host runs `match` there, then
forwards the matched placements over the signed node-to-node hop to the leader, which starts
each once and returns the count; if no leader answers within three seconds `Fire` returns `0`
and the host retries in the background. `match` therefore runs on the node that fired, never on
the leader, and must not depend on state only the leader has. The payload crosses as JSON, so
numbers arrive in `msg.payload` as floating point, and a payload over 1 MiB cannot be forwarded
and returns an error. See
[Active/active → Plugin workflow triggers](../../../settings/system/high-availability/active-active.md#plugin-workflow-triggers).
The KVM plugin's **VM Trigger** fires this way, from the same place it writes the guest's
timeline row.

## Pick lists from the plugin

A field of type `plugin-select` offers the plugin's own objects as a pick list. It names a
`picker-source` capability the plugin declares, and the host renders it with one generic picker
that never names the plugin's route. See
[Picker sources](../../contracts/capabilities.md#picker-sources).

```go
{"key": "vm", "label": "Virtual Machine", "type": "plugin-select", "allowCustom": true,
    "source": map[string]any{"plugin": "addon-kvm", "target": "vm"}}
```

## A node that used to ship with the host

When a node moves from the host into a plugin, saved canvases still carry its bare key. The
plugin keeps the key as its node key and declares a `workflow-node-alias` for it, and the host
resolves the bare action to the plugin node wherever it runs, binds or renders it. No stored
workflow is rewritten. See [Workflow node aliases](../../contracts/capabilities.md#workflow-node-aliases).
`vm-trigger` is the one node alias shipped today. A former host node that was folded into a
generic host node rather than moved into a plugin is claimed with `channel_type` instead of
`node`: the eight comm plugins with a former `send-<provider>` key each claim it for their own
channel type, and the host runs it as `send-communication` on that type. See
[Channel-type aliases](../../contracts/capabilities.md#channel-type-aliases).

## Availability and uninstalling

Plugin node keys are toggled in the Store's **Nodes** tab like any other, under the composite
key.

Uninstalling a plugin withdraws its nodes from the palette. Workflows that used them keep the
nodes on the canvas, but they cannot execute. The wiring is
still what you designed, and reinstalling restores it rather than asking you to rebuild. A
composite-key node reports `plugin node unavailable` on its error port. An aliased bare key
(`vm-trigger`) is an unknown action while the plugin is away: it never fires, and a manual run
passes the message through.

## Related

- [Core nodes](../core-nodes/README.md): adding a node to McHarbor itself
- [Custom nodes](../custom-nodes/README.md): JavaScript, no rebuild
- [Plugins](../../../plugins/README.md): the plugin system, the manifest and the loader
- [Node catalog](../../../workflows/nodes/README.md): the built-in 90
