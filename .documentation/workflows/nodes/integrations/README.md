# Integration Nodes

Integration nodes talk to something outside McHarbor: a mail server, a chat channel, a
remote host, a DNS resolver, an HTTP or GraphQL endpoint.

12 node types.

Nodes that deliver through a configured channel or email server show as **Unavailable**
until one exists, and say which: "This node only works when at least one enabled email server
is configured in Settings."

| Node | Key | Ports | What it does |
|---|---|---|---|
| **DNS Lookup** | `dns-lookup` | in `input` · out `output`, `error` | Resolve DNS records for a hostname |
| **FTP Upload** | `ftp-upload` | in `input` · out `output`, `error` | Upload workflow data or a saved file with FTP or SFTP |
| **GraphQL Request** | `graphql-request` | in `input` · out `output`, `error` | Send a GraphQL request and store the parsed response |
| **Open Ticket** | `itsm-open` | in `input` · out `output`, `error` | Open or re-notify a ticket in an ITSM through an incident-capable channel |
| **Update Ticket** | `itsm-update` | in `input` · out `output`, `error` | Add a note to the ticket a key opened, without changing its state |
| **Resolve Ticket** | `itsm-resolve` | in `input` · out `output`, `error` | Resolve the ticket a key opened |
| **Ping** | `ping` | in `input` · out `output`, `error` | Check whether a host is reachable over TCP |
| **Send Message** | `send-communication` | in `input` · out `output`, `error` | Send a message through a configured communication channel |
| **Send Email** | `send-email` | in `input` · out `output`, `error` | Send an email through a configured email server |
| **Send Internal Notification** | `send-notification` | in `input` · out `output`, `error` | Send an in-app notification inside McHarbor |
| **Send Webhook** | `send-webhook` | in `input` · out `output`, `error` | Send an outbound webhook with message data |
| **SSH Exec** | `ssh-exec` | in `input` · out `output`, `error` | Run a remote command over SSH with password or key authentication |

## Configuration, node by node

### DNS Lookup (`dns-lookup`)

Resolve DNS records for a hostname

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Hostname (`hostname`): text, required
  - Record Type (`record_type`): select, default `A`, one of `A`, `AAAA`, `MX`, `TXT`, `CNAME`, `NS`

### FTP Upload (`ftp-upload`)

Upload workflow data or a saved file with FTP or SFTP

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Host (`host`): text, required
  - Port (`port`): number
  - Username (`username`): text
  - Password (`password`): text
  - Protocol (`protocol`): select, default `ftp`, one of `ftp`, `sftp`
  - Remote Path (`remote_path`): text, required
  - Source Mode (`source_mode`): select, default `payload`, one of `payload`, `file`
  - Property (`property`): expression, default `payload`; shown when `{"source_mode":"payload"}`
  - Local Path (`local_path`): text; shown when `{"source_mode":"file"}`

### GraphQL Request (`graphql-request`)

Send a GraphQL request and store the parsed response

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - URL (`url`): text, required
  - Query (`query`): textarea, required
  - Variables (`variables`): key-value
  - Headers (`headers`): key-value

### Ping (`ping`)

Check whether a host is reachable over TCP

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Host (`host`): text, required
  - Timeout (seconds) (`timeout`): number, default `5`

### Open Ticket (`itsm-open`)

Open or re-notify a ticket in an ITSM through an incident-capable channel

- **Ports**: in: `input`; out: `output`, `error`
- **Permission**: `alerts.manage` to add it to a workflow; the save is refused without it
- **Fields**
  - Ticketing Channel (`channel_id`): communication-channel-select, ticketing channels only, required
  - Ticket Key (`dedup_key`): text, required; `{{path}}` placeholders; the same key on Open, Update and Resolve names the same ticket
  - Severity (`severity`): select, default `warning`, one of `critical`, `warning`, `info`
  - Title (`title`): text, required
  - Message (`message`): textarea

### Update Ticket (`itsm-update`)

Add a note to the ticket a key opened, without changing its state

- **Ports**: in: `input`; out: `output`, `error`
- **Permission**: `alerts.manage` to add it to a workflow
- **Fields**
  - Ticketing Channel (`channel_id`): communication-channel-select, ticketing channels only, required
  - Ticket Key (`dedup_key`): text, required; `{{path}}` placeholders; the same key on Open, Update and Resolve names the same ticket
  - Message (`message`): textarea, required

### Resolve Ticket (`itsm-resolve`)

Resolve the ticket a key opened

- **Ports**: in: `input`; out: `output`, `error`
- **Permission**: `alerts.manage` to add it to a workflow
- **Fields**
  - Ticketing Channel (`channel_id`): communication-channel-select, ticketing channels only, required
  - Ticket Key (`dedup_key`): text, required; `{{path}}` placeholders; the same key on Open, Update and Resolve names the same ticket
  - Message (`message`): textarea

The three ticket nodes queue the operation in the ticket outbox and emit on `output` once it
is queued, not when the ITSM accepted it. `msg.payload` is
`{ queued, channelId, dedupKey, state }`. A stopped delivery is shown under
[Tickets](../../../settings/monitoring/alerts/tickets/README.md).

### Send Message (`send-communication`)

Send a message through a configured communication channel

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Communication Channel (`channel_id`): communication-channel-select
  - Recipient (`recipient`): text
  - Title (`title`): text
  - Message (`message`): textarea, required

**Saved per-provider nodes.** Workflows saved with the former `send-slack`, `send-discord`,
`send-teams`, `send-telegram`, `send-whatsapp`, `send-signal`, `send-ntfy` or `send-gotify` nodes
keep running as this node. Each comm plugin claims its own former key (a channel-type
`workflow-node-alias`), so while comm-slack is loaded a stored `send-slack` runs as Send Message
on the Slack channel type, and an empty channel picks the default Slack channel; the canvas
shows it as Send Message. With the plugin uninstalled the node stays on the canvas without a
definition and a run passes the message through unchanged, like any node whose plugin is gone.

### Send Email (`send-email`)

Send an email through a configured email server

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Delivery (`delivery_mode`): select, required, default `configured`, one of `configured`, `custom`
  - Email Server (`server_id`): email-server-select; shown when `{"delivery_mode":"configured"}`
  - SMTP Host (`smtp_host`): text, required; shown when `{"delivery_mode":"custom"}`
  - SMTP Port (`smtp_port`): number, required, default `587`; shown when `{"delivery_mode":"custom"}`
  - Encryption (`smtp_encryption`): select, default `starttls`, one of `none`, `starttls`, `ssl_tls`; shown when `{"delivery_mode":"custom"}`
  - Auth Method (`smtp_auth_method`): select, default `plain`, one of `none`, `plain`, `login`, `cram_md5`; shown when `{"delivery_mode":"custom"}`
  - Username (`smtp_username`): text; shown when `{"delivery_mode":"custom"}`
  - Password (`smtp_password`): text; shown when `{"delivery_mode":"custom"}`
  - From Address (`from_address`): text, required; shown when `{"delivery_mode":"custom"}`
  - From Name (`from_name`): text; shown when `{"delivery_mode":"custom"}`
  - To (`to`): multiselect; required when `{"delivery_mode":"custom"}`, optional otherwise
  - Subject (`subject`): text
  - Body (`body`): textarea, required

The **Email Server** list holds the enabled communication channels whose plugin declares that it
delivers email (an `email-channel` capability: comm-smtp, comm-gmail and comm-exchange today). It
reads `GET /api/communication-channels` and needs `communications.view`. With no such plugin
loaded the field is disabled and reads "No installed communication plugin can send email."

**To** works like a report's recipient field: with a configured server, an empty **To** sends to the recipients saved on the channel, and a filled one replaces them for this send only (the channel itself is not changed). The override goes through the dispatcher's recipient check: at most 512 characters, no control characters or line breaks. Values are sent as written; `{{path}}` placeholders are not filled in by this node. With **Other** (custom SMTP) there is no channel to fall back on, so **To** is required.

With no **Email Server** picked, the node sends through the default channel among those email types. If no plugin
declares one, the run fails with `no installed communication plugin can send email` rather than
handing the message to a chat channel. A workflow that stored a server id keeps sending through
that channel by id.

### Send Internal Notification (`send-notification`)

Send an in-app notification inside McHarbor

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Title (`title`): text
  - Message (`message`): textarea, required

### Send Webhook (`send-webhook`)

Send an outbound webhook with message data

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Delivery (`delivery_mode`): select, required, default `custom`, one of `configured`, `custom`
  - Webhook (`webhook_id`): webhook-select, required; shown when `{"delivery_mode":"configured"}`
  - URL (`url`): text, required; shown when `{"delivery_mode":"custom"}`
  - Method (`method`): select, default `POST`, one of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`; shown when `{"delivery_mode":"custom"}`
  - Headers (`headers`): key-value; shown when `{"delivery_mode":"custom"}`
  - Body Source (`body_source`): expression, default `payload`

### SSH Exec (`ssh-exec`)

Run a remote command over SSH with password or key authentication

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Host (`host`): text, required
  - Port (`port`): number, default `22`
  - User (`user`): text
  - Password (`password`): text
  - Private Key (`private_key`): textarea
  - Command (`command`): textarea, required

## Related

- [Node catalog](../README.md): all 90 node types, and how to read these pages
- [Execution model](../../execution/README.md): ports, emissions and error routing
- [The message contract](../../messages/README.md): what flows between these nodes
