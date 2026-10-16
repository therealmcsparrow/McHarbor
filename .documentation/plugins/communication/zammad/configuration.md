# Zammad: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Zammad 6.x or 7.x instance reachable from McHarbor over `https://`.
- An **agent user** for McHarbor with `ticket.agent` on the group tickets should land in.
- A **customer user** tickets are opened for, for example `mcharbor@your-domain`. Zammad
  requires a customer on every ticket.
- Token access enabled under **Admin → Settings → System → API**.
- Optional: admin rights in Zammad to add a Webhook and a Trigger, for status coming back
  without polling.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `instance_url` | string | Yes | — | Base URL of the Zammad instance, for example `https://support.example.de`. Must be `https://`, with no credentials, query or fragment. A sub-path is kept. |
| `access_token` | secret | Yes | — | Personal access token of the McHarbor agent user, sent as `Authorization: Token token=…`. |
| `group` | string | Yes | — | Name of the Zammad group new tickets are created in. The token owner needs `ticket.agent` on it. |
| `customer` | string | Yes | — | Login or email of the customer user, or `guess:<email>` to let Zammad find or create one. |
| `on_behalf_of` | string | No | — | User id, login or email sent as the `From` header. Requires `admin.user` on the token owner. |
| `resolve_state` | string | No | `closed` | Zammad state set when the alert recovers, for example `pending close`. |
| `suppress_notifications` | bool | No | `false` | Sends `X-Zammad-Suppress-Notifications: true` on the note and close calls. |
| `status_map` | string | No | — | JSON object mapping Zammad state names to `open`, `acknowledged`, `resolved`, `closed` or `cancelled`, for example `{"waiting for vendor":"acknowledged"}`. Merged over the default table. |
| `update_on_repeat` | bool | No | `true` | When the alert fires again while its ticket is open, add an internal note. |
| `reopen_window_minutes` | int | No | `0` | 0 to 1440. **Reopen window (minutes).** When an alert fires again within this many minutes of the ticket it opened being closed, McHarbor sets that ticket back to `open` with a note instead of creating a new one. `0` always creates a new ticket. A merged ticket is never reopened. |
| `poll_interval_minutes` | int | No | `5` | 0 to 60. `0` means webhook only. |
| `default_assignment_group` | string | No | — | Shown as **Owner login**: the login of the agent who owns new tickets. The Zammad group is the queue, so this key carries the optional owner instead. |
| `severity_map` | string | No | — | JSON object overriding the priority names, for example `{"critical":"3 high"}`. Keys are `critical`, `warning`, `info`. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you replace it. The token never appears in an error message, a log line or
an audit field; a Zammad error that echoes it is redacted.

## Setting it up

### In Zammad

1. Create the McHarbor agent user and give it `ticket.agent` on the target group.
2. Create the customer user tickets are opened for.
3. Make sure **Admin → Settings → System → API** has token access enabled.
4. Sign in as the McHarbor agent, open **Profile → Token Access**, and create a token with
   the `ticket.agent` permission. Copy it.

### In McHarbor

5. Install the plugin from **App Store → Plugins**.
6. Open **Settings → Integrations → Communications**, add a channel of type **Zammad**, and
   fill in **Zammad URL**, **Access token**, **Group** and **Customer**.
7. Save and press **Test connection**. McHarbor checks the token, the group and the
   customer, and creates nothing; see [Troubleshooting](./troubleshooting.md) if it fails.
8. Edit the channel again. Under **Ticket status from the ITSM**, select
   **Create inbound URL** and copy the URL, which ends in `/api/incident-webhooks/` and a
   token, together with its **Signing secret**. Both are shown once. This needs
   `communications.manage`.

### Back in Zammad

9. Open **Admin → Webhook** and add a webhook. Set the endpoint to the inbound URL, paste
   the signing secret into **HMAC SHA1 Signature Token**, and keep **SSL Verify** on.
10. Open **Admin → Trigger** and add a trigger that activates **on action**, with the
    conditions **Tags contains one** `mcharbor` and **State has changed**, and the action
    **Webhook** set to the webhook above.
11. If you give the webhook a custom payload, it must still carry
    `{"ticket":{"id","number","title","state","owner":{"login"}}}`. The default payload
    already does.

### Routing

12. Grant `channels.use_comm_zammad` to the roles allowed to route events here.
13. Select the channel on an alert rule, or in an Open, Update or Resolve Ticket workflow
    node.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Outbound endpoints

All on the configured `instance_url`:

| Endpoint | Used for |
|---|---|
| `GET /api/v1/users/me` | Test: proves the token |
| `GET /api/v1/groups?expand=true` | Test: the group exists and is active |
| `GET /api/v1/users/search` | Test: the customer exists (skipped for `guess:`) |
| `GET /api/v1/tickets/search` | Open: find the ticket already carrying the marker |
| `POST /api/v1/tickets` | Open, and plain notifications |
| `POST /api/v1/tags/add` | Open: tag the ticket `mcharbor` |
| `POST /api/v1/ticket_articles` | Update: internal note |
| `PUT /api/v1/tickets/{id}` | Resolve: state and note together |
| `GET /api/v1/tickets/{id}?expand=true` | Poll |

## Related

- [Zammad](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
