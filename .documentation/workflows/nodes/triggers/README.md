# Trigger Nodes

A trigger is where a run starts. Every workflow needs at least one, and a workflow with
no runnable entry node fails immediately with "no runnable entry nodes".

10 node types.

Triggers have an output port and no input port. Activating a workflow is what arms the
automatic ones; an inactive workflow can still be run by hand from the editor.

| Node | Key | Ports | What it does |
|---|---|---|---|
| **Container Trigger** | `container-trigger` | in — · out `output` | Start a workflow when a container changes state or health |
| **Docker Event Trigger** | `docker-event-trigger` | in — · out `output` | Start a workflow from Docker events filtered by type, action, name, or label |
| **File Watch Trigger** | `file-watch-trigger` | in — · out `output` | Start a workflow when a watched file path changes |
| **Link In** | `link-in` | in — · out `output` | Receive the latest message from a link output in another workflow |
| **Manual Trigger** | `manual-trigger` | in — · out `output` | Start a workflow manually from the editor or run panel |
| **Metric Trigger** | `metric-trigger` | in — · out `output` | Start a workflow when container metrics are checked |
| **Registry Tag Trigger** | `registry-tag-trigger` | in — · out `output` | Start a workflow when a watched image tag digest changes |
| **Ticket Status** | `itsm-status-trigger` | in — · out `output` | Start when an ITSM reports that a linked ticket changed state |
| **Schedule Trigger** | `schedule-trigger` | in — · out `output` | Run a workflow on a cron-based schedule |
| **Webhook Trigger** | `webhook-trigger` | in — · out `output` | Start a workflow from an incoming webhook request |

## Configuration, node by node

### Container Trigger (`container-trigger`)

Start a workflow when a container changes state or health

- **Ports**: in: none; out: `output`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Watch (`watch`): select, required, default `status`, one of `status`, `health`
  - Status Event (`status`): select, default `any`, one of `any`, `start`, `stop`, `die`, `health_status`; shown when `{"watch":"status"}`
  - Health Status (`health`): select, default `any`, one of `any`, `healthy`, `unhealthy`, `starting`, `none`; shown when `{"watch":"health"}`

### Docker Event Trigger (`docker-event-trigger`)

Start a workflow from Docker events filtered by type, action, name, or label

- **Ports**: in: none; out: `output`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Event Type (`type`): select, default `any`, one of `any`, `container`, `image`, `volume`, `network`
  - Action (`action`): multiselect, one of `create`, `start`, `stop`, `restart`, `die`, `kill`, `destroy`, `pause`, `unpause`, `oom`, `health_status`, `rename`, `update`, `exec_create`, `exec_start`, `exec_die`, `pull`, `push`, `delete`, `tag`, `untag`, `prune`, `mount`, `unmount`, `connect`, `disconnect`
  - Name or ID (`name`): text
  - Label Filter (`label`): text

### File Watch Trigger (`file-watch-trigger`)

Start a workflow when a watched file path changes

- **Ports**: in: none; out: `output`
- **Fields**
  - Storage Location (`storage_location_id`): storage-location-select
  - Path (`path`): text, required
  - Event Types (`event_types`): multiselect, one of `create`, `write`, `remove`

### Link In (`link-in`)

Receive the latest message from a link output in another workflow

- **Ports**: in: none; out: `output`
- **Fields**
  - Source Output (`source`): link-output-select, required

### Manual Trigger (`manual-trigger`)

Start a workflow manually from the editor or run panel

- **Ports**: in: none; out: `output`
- **Fields**: none

### Metric Trigger (`metric-trigger`)

Start a workflow when container metrics are checked

- **Ports**: in: none; out: `output`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Metric Conditions (`conditions`): metric-conditions
  - Condition Logic (`logic`): select, default `and`, one of `and`, `or`
  - Interval (seconds) (`interval`): number, default `10`
  - Cooldown (seconds) (`cooldown`): number, default `60`

### Registry Tag Trigger (`registry-tag-trigger`)

Start a workflow when a watched image tag digest changes

- **Ports**: in: none; out: `output`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Image Tag (`image`): text, required
  - Registry (`registry_mode`): select, required, default `custom`, one of `configured`, `custom`
  - Registry (`registry_id`): registry-select; shown when `{"registry_mode":"configured"}`
  - Interval (seconds) (`interval`): number, default `300`

### Ticket Status (`itsm-status-trigger`)

Start when an ITSM reports that a linked ticket changed state

- **Ports**: in: none; out: `output`
- **Fields**
  - Ticketing Channel (`channel_id`): communication-channel-select, ticketing channels only; empty accepts every channel
  - States (`states`): multiselect of `open`, `acknowledged`, `resolved`, `closed`, `cancelled`; empty accepts all

`msg.payload` is `{ channelId, channelType, externalId, number, url, dedupKey, state, assignee, note }`.
Only statuses for tickets McHarbor opened arrive, and a status older than the last one applied
is dropped before it gets here.

### Schedule Trigger (`schedule-trigger`)

Run a workflow on a cron-based schedule

- **Ports**: in: none; out: `output`
- **Fields**
  - Cron Expression (`cron`): cron, required
  - Timezone (`timezone`): timezone-select, default `UTC`

### Webhook Trigger (`webhook-trigger`)

Start a workflow from an incoming webhook request

- **Ports**: in: none; out: `output`
- **Fields**
  - Method (`method`): select, default `POST`, one of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
  - Path (`path`): text
  - Secret (`secret`): text

## Related

- [Node catalog](../README.md): all 90 node types, and how to read these pages
- [Execution model](../../execution/README.md): ports, emissions and error routing
- [The message contract](../../messages/README.md): what flows between these nodes
