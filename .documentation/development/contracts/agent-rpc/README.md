# Agent RPC Contract

How a plugin sends a message to a remote agent and receives the agent's replies and unsolicited
frames. The host carries no compute meaning here: it checks that the plugin declared the message,
applies a time budget and a version floor, and moves JSON. What a message means is the plugin's
business.

This replaces the typed compute handle. `HostContext.ComputeRPC`, `plugin.ComputeRPCHandle`,
`PoolFileHandle`, `ConsoleStream` and the `specs/compute` types no longer exist.

## What a plugin needs

Both of these, or the host hands the plugin nothing:

1. The permission token `agent.rpc` in `permissions:` of `plugin.yaml`. It was called
   `compute.rpc` before. The loader sets `HostContext.AgentRPC` only when the manifest has the
   token **and** an `agent-rpc` capability. Otherwise the field is `nil`.
2. An `agent-rpc` capability under `contributes.capabilities`:

```yaml
permissions:
  - agent.rpc
contributes:
  capabilities:
    - kind: agent-rpc
      send: [vm_define, vm_start, vm_shutdown, vm_query]
      observe: [vm_event]
```

| Field | Required | Meaning |
|---|---|---|
| `send` | yes | Message types the plugin may send. Each must be in the host ceiling and listed once. |
| `observe` | no | Unsolicited frame types the plugin may receive. Each must be observable and listed once. |

A declaration can only narrow the ceiling. A type outside it is refused when the manifest is
validated (`core/plugincaps`, mirrored by `mcharbor plugin dev validate`) and again on every
call. A declaration that fails validation is dropped whole, and the plugin loads without a
bridge.

## The local interface

`HostContext.AgentRPC` is typed `any`. The plugin declares this interface in its own code, in
builtin types only, and asserts the field against it. It imports nothing from the host for this.

```go
type agentRPC interface {
	Available(ctx context.Context, envID string) (bool, string)
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	Subscribe(sink func(envID, msgType string, frame []byte))
}

rpc, ok := host.AgentRPC.(agentRPC)
if !ok {
	// No token, no declaration, or a host without the bridge: say so, offer nothing.
}
```

| Method | Behaviour |
|---|---|
| `Available` | `true` when the environment takes agent calls. Otherwise `false` and one of `not_agent_environment`, `agent_offline`, `agent_too_old`. |
| `Call` | Sends one declared message and returns the reply. `onProgress` receives the progress frames the message type declares, and may be `nil`. |
| `Subscribe` | Installs the plugin's sink for the frame types it declared under `observe`. `nil` removes it. A second call replaces the first. |

`Call` fails fast when the agent is offline. It never buffers a message and never replays one,
because a replayed "start this guest" is a hazard. A plugin that wants a retry makes the new call
itself.

`Subscribe` frames reach the sink from the plugin's own buffered queue of 256. When the queue is
full, frames are dropped rather than slowing the agent link, so a sink must be quick and a plugin
must treat events as hints and reconcile from a `Call`.

## Payload and reply

The payload is the JSON of the **one envelope slot** the message uses: `vm`, `ct`, `net`, `pool`,
`compute_image` or `compute`. The host decodes it into its own agent struct for that slot, drops
unknown fields, overwrites `envId`, and applies the budget and the version floor. The reply is that
slot's JSON. The plugin cannot fill a second slot or choose a timeout.

## Errors

Errors from `Call` expose a code. Branch on it, never on the text:

```go
var coded interface{ AgentRPCCode() string }
if errors.As(err, &coded) {
	switch coded.AgentRPCCode() {
	case "agent_offline", "not_agent_env":
		// report the environment as unreachable
	}
}
```

| Code | Means |
|---|---|
| `not_agent_env` | The environment is not reached through an agent. |
| `agent_offline` | No agent is connected, or it left during the call. |
| `agent_outdated` | The agent is below the message's floor. The message contains `needs agent`. |
| `uplink_busy` | The host refused the adapter a network was to be built on. |
| `refused` | The type is undeclared or not routable, or the agent failed the request. |

## The ceiling

The host owns the list of what a plugin may send, in `src/backend/core/agent/pluginroutes.go`.
Each entry names the message type, its envelope slot, its budget class, its version floor and any
progress types. The golden contract is `src/contracts/plugincaps/agent_rpc_ceiling.yaml`.

- **Sendable (69 types):** `vm_*`, `ct_*`, `net_ensure`, `net_remove`, `net_detect`, `pool_*`,
  `compute_probe_request`, `compute_config_request`, `compute_migrate_request`,
  `compute_image_fetch`, `compute_image_list`, `compute_image_remove` and `host_device_list`.
- **Observable (5 types):** `vm_event`, `vm_import_progress`, `ct_backup_progress`,
  `net_lease_event`, `compute_migrate_progress`.
- **Never routable:** `exec_*`, `host_terminal_*`, `agent_update_*`, `compose_*`, `http_*`,
  file transfer, backup key messages, `net_adopt_confirm` and `compute_image_cancel`.

| Budget | Timeout |
|---|---|
| `quick` | 60 s |
| `long` | 30 min |
| `transfer` | 6 h |
| `relocate` | 12 h for an apply; a plan or repair takes `quick` |

Version floors are host-side too, and an agent below one fails with `agent_outdated` instead of
waiting out the budget. A plugin cannot raise or lower either.

### Adding a message type

A new type is a reviewed addition to the ceiling in `core/agent/pluginroutes.go`, with its slot,
budget and floor, and a line in `agent_rpc_ceiling.yaml`. After that a plugin lists it under
`send`. The agent wire protocol (`src/agent/protocol.go`) is a separate change and needs an agent
release.

## Wire structs and goldens

A plugin keeps the agent's JSON shapes as local structs in `backend/wire_agent.go`, holding only
the fields it reads. These files are a deliberate duplication family, `addon-agent-wire`, in
`scripts/sanity/duplication/accepted.json`: plugins share no code. They are held to the protocol by
the golden files in `src/contracts/agentwire/`:

| Folder | Holds |
|---|---|
| `slots/*.json` | The JSON shape of each envelope slot, generated from the `core/agent` structs. |
| `requests/*.json` | Frozen requests the retired host bridge produced. |
| `converted/*.json` | Frozen replies and frames, including `vm_events.json`, `ct_events.json` and `import_progress.json`. |

A plugin test decodes the goldens through its local structs, so a protocol change that renames a
field fails the plugin's test instead of a running host.

## Console and reboot

The host keeps the console gateway, `modules/compute_console`. It calls the bridge (`core/agentrpc`)
directly, so no console type is in the plugin ABI. A plugin opts guest kinds in with
[`console-subject`](../capabilities.md#console-subjects). A host reboot reaches guests through
[`reboot-drain`](../reboot-drain/README.md), not through the agent.

## What goes wrong

| Symptom | Cause |
|---|---|
| `AgentRPC` is `nil` | The manifest lacks `agent.rpc` or an `agent-rpc` capability, or the capability was refused. Check the loader log for `plugin capabilities refused`. |
| The manifest is refused for `compute.rpc` | The token is now `agent.rpc`. |
| `refused` for a type you listed | The type is outside the ceiling, or it is not routable. |
| A built `.so` will not load | The plugin ABI changed. Rebuild and update every plugin. |
| Progress stops arriving | The sink queue was full and frames were dropped. Reconcile with a `Call`. |

## Minimal example

```go
func (p *Plugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	return nil
}

func (p *Plugin) probe(ctx context.Context, envID string) ([]byte, error) {
	rpc, ok := p.host.AgentRPC.(agentRPC)
	if !ok {
		return nil, errBridgeMissing
	}
	if up, why := rpc.Available(ctx, envID); !up {
		return nil, fmt.Errorf("agent unavailable: %s", why)
	}
	return rpc.Call(ctx, envID, "compute_probe_request", []byte(`{}`), nil)
}
```

## Related

[Host services](../host-services.md), [Capability declarations](../capabilities.md),
[Manifest](../../../how-to/plugin/manifest.md), [Plugin development](../../plugins/developing.md).
