# OTRS / Znuny / OTOBO: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Znuny 6.5/7.x, OTOBO 10.1/11.x or OTRS 6/7 instance reachable from McHarbor over
  `https://`. OTRS 8 and later are best effort; see the [overview](./README.md#otrs-8-and-later).
- Admin rights in that system, to create an agent, a customer user, a dynamic field and a
  web service.
- A **dedicated agent** for McHarbor with `rw` on the group of the target queue. Without
  `ro` the dedup search returns nothing and every firing opens a duplicate; without `rw`
  creates fail with `AccessDenied`.
- A **customer user** tickets are opened for. Every ticket needs one.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `instance_url` | string | Yes | — | Base URL of the instance **without** `/otrs` or `/otobo`, for example `https://helpdesk.example.com`. Must be `https://`, with no credentials, query or fragment. A sub-path is kept. |
| `flavour` | string | Yes | `znuny` | Shown as **Product**: `znuny`, `otobo` or `otrs`. Picks the default script path. |
| `script_path` | string | No | `otrs` (`otobo` for OTOBO) | Path segment in front of `nph-genericinterface.pl`. Only for installs under a custom path. |
| `webservice` | string | Yes | `McHarborConnectorREST` | Name the web service was imported under. |
| `user_login` | string | Yes | — | Login of the McHarbor agent. |
| `password` | secret | Yes | — | Password of the McHarbor agent. |
| `default_assignment_group` | string | Yes | — | Shown as **Queue**: the full queue name, for example `Raw` or `Operations::Alerts`. |
| `customer_user` | string | Yes | — | Login or email of the customer user set on created tickets. |
| `ticket_type` | string | No | — | Ticket type, for example `Incident`. Required only when `Ticket::Type` is enabled. |
| `dedup_field` | string | Yes | `McHarborDedupKey` | Name of the Ticket **Text** dynamic field holding the ticket key. Letters and digits only. |
| `close_state` | string | No | `closed successful` | State set when the alert recovers. |
| `resolved_states` | string | No | — | Comma-separated state names that mean resolved, for renamed states. |
| `cancelled_states` | string | No | — | Comma-separated state names that mean cancelled, for renamed states. |
| `update_on_repeat` | bool | No | `true` | Shown as **Note on repeat**: when the alert fires again while its ticket is open, add an internal note. |
| `reopen_window_minutes` | int | No | `0` | 0 to 1440. Accepted, but not acted on yet: every new firing after a recovery opens a new ticket. |
| `poll_interval_minutes` | int | No | `5` | 0 to 60. `0` means webhook only. |
| `severity_map` | string | No | — | JSON object overriding the priority names, for example `{"critical":"P1 - Kritisch"}`. Keys are `critical`, `warning`, `info`. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you replace it. The password and the session never appear in a URL, an
error message or a log line; an ITSM error that echoes either is redacted.

## Setting it up

### In OTRS, Znuny or OTOBO

1. **Admin → Agents**: create the agent `mcharbor` with a strong password. Under
   **Admin → Agents ↔ Groups**, give it `rw` on the group of the target queue.
2. **Admin → Customer Users**: create the customer user for McHarbor tickets, or pick an
   existing one, and note its login.
3. **Admin → Dynamic Fields**: add a **Ticket** field of type **Text** named
   `McHarborDedupKey`. It does not need to be shown on any screen.
4. **Admin → Web Services → Add Web Service → Import web service**: import
   [`McHarborConnectorREST.yml`](../../../../src/plugins/Plugins/comm-otrs/webservice/McHarborConnectorREST.yml)
   for Znuny or OTRS 6/7, or
   [`McHarborConnectorREST-otobo.yml`](../../../../src/plugins/Plugins/comm-otrs/webservice/McHarborConnectorREST-otobo.yml)
   for OTOBO, under the name `McHarborConnectorREST`. The setup guide on the channel form
   copies either file to the clipboard. On OTRS 8 and later, keep the **Provider** part only.
   If OTOBO rejects the requester block, import the provider part and add the
   `Ticket::TicketUpdate` invoker in the web service editor instead.

### In McHarbor

5. Install the plugin from **App Store → Plugins**.
6. Open **Settings → Integrations → Communications**, add a channel of type
   **OTRS / Znuny / OTOBO**, choose the **Product**, and fill in the fields above.
7. Save and press **Test connection**. McHarbor logs in, searches the queue and checks the
   dynamic field, and creates nothing; see [Troubleshooting](./troubleshooting.md) if it
   fails.
8. Edit the channel again. Under **Ticket status from the ITSM**, select
   **Create inbound URL** and copy the URL, which ends in `/api/incident-webhooks/` and a
   token, together with its **Signing secret**. Both are shown once. This needs
   `communications.manage`.

### Back in the ITSM: status coming back

9. Open the imported web service and the requester transport **HTTP::REST**. Set **Host**
   to McHarbor's base URL, the `McHarborStateChanged` controller to
   `/api/incident-webhooks/<token>` (the path part of the inbound URL), and the additional
   header `X-McHarbor-Webhook-Secret` to the signing secret.
10. Optional but recommended: on the `McHarborStateChanged` invoker's `TicketStateUpdate`
    event, add a condition that `McHarborDedupKey` is not empty. Without it the invoker
    fires for every ticket in the system; McHarbor ignores the unknown ones, but the load
    is yours.
11. Skip steps 8 to 10 on OTRS 8 or when the requester cannot reach McHarbor, and keep the
    **Poll interval** above `0` instead.

### Routing

12. Grant `channels.use_comm_otrs` to the roles allowed to route events here.
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

All under `<instance_url>/<script_path>/nph-genericinterface.pl/Webservice/<webservice>`,
each with a JSON body:

| Route | Operation | Used for |
|---|---|---|
| `POST /Session` | `Session::SessionCreate` | Login, on first use and after an expired session |
| `POST /TicketSearch` | `Ticket::TicketSearch` | Open: find the ticket carrying the key; Test: queue and field probes |
| `POST /TicketGet` | `Ticket::TicketGet` | Open: confirm a search hit; Poll |
| `POST /Ticket` | `Ticket::TicketCreate` | Open, and plain notifications |
| `PATCH /Ticket/{TicketID}` | `Ticket::TicketUpdate` | Update: internal note; Resolve: close state and note together |

## Related

- [OTRS / Znuny / OTOBO](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
