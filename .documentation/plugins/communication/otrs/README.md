# OTRS / Znuny / OTOBO

Opens, notes and closes tickets in OTRS, Znuny or OTOBO for McHarbor alerts, and reads
ticket state back. All three products still ship the OTRS **GenericInterface**, so one
channel type covers them: McHarbor talks to a REST web service the admin imports from a
YAML file that ships with the plugin, and status comes back through a GenericInterface
**invoker** calling McHarbor, or by polling.

- **Slug** `comm-otrs`, **version** 2026.09.01, **channel type** `otrs`
- **Host range** `>=2026.09.01`
- **Covers** Znuny 6.5 and 7.x, OTOBO 10.1 and 11.x, OTRS 6 and 7. OTRS 8 and later are
  best effort (see *OTRS 8 and later* below)
- **Where it appears** Settings → Integrations → Communications, as a channel type; its
  tickets are listed under Settings → Monitoring → Alerts → Tickets
- **Gated by** `communications.view` to see channels, `communications.manage` to add or
  change one or to create its inbound URL, and `channels.use_comm_otrs` (global) to route
  events through this channel

## How it works

- **A dedicated agent and a session.** McHarbor logs in once with `SessionCreate`
  (`POST /Session`) as the configured agent and caches the `SessionID` per instance, login
  and password. Every later call carries the `SessionID` in its **JSON body**. The shipped
  web service maps every route to `POST` or `PATCH`, so neither the password nor the session
  ever lands in a URL or an access log. When a session expires, the next call gets an
  `AuthFail` error; McHarbor logs in again once and retries once, then gives up with a
  message for the operator.
- **Errors arrive as HTTP 200.** A GenericInterface operation that fails still answers
  `200 OK` with `{"Error":{"ErrorCode":…,"ErrorMessage":…}}`. McHarbor treats every such
  envelope as a failure. Only `*.MissingParameter` and `*.InvalidParameter` messages are
  shown to the operator, with control characters removed, cut to 200 characters, and with
  the password and session redacted.
- **The ticket key lives in a dynamic field.** Every ticket McHarbor opens carries the
  alert's ticket key in a Ticket **Text** dynamic field, `McHarborDedupKey` by default.
- **Open never duplicates.** Before creating, McHarbor searches (`POST /TicketSearch`) the
  configured queue for a ticket in an open state type (`new`, `open`, `pending reminder`,
  `pending auto`) whose dynamic field equals the key, then re-reads each hit with
  `POST /TicketGet` and reuses it **only** when the field really holds the key. The re-read
  matters: a stock `TicketSearch` ignores a filter on a dynamic field that does not exist,
  so a misspelt field name would otherwise match the first open ticket in the queue.
- **Open** creates the ticket (`POST /Ticket`) in **Queue** for **Customer user**, state
  `new`, the mapped priority, the optional **Ticket type**, the dynamic field, and a first
  internal article describing the alert. **Update** (an alert firing again while its
  ticket is open, when **Note on repeat** is on) adds an internal article only
  (`PATCH /Ticket/{TicketID}`) and never touches state or priority. **Resolve** is a single
  `PATCH /Ticket/{TicketID}` that sets **Close state** and adds the recovery article, so
  the ticket never shows closed without an explanation.
- **Articles are internal.** Every article McHarbor writes uses communication channel
  `Internal`, sender type `system`, and is not visible to the customer.
- **Plain notifications** (a workflow notify node, a backup outcome) create an ordinary
  ticket in the same queue, with no dynamic field value, so a later Open never reuses it.
  Level `error` maps to the critical priority, `warning` to the warning one, anything else
  to info.
- **Test never creates anything.** It logs in afresh, searches the queue, and searches the
  dynamic field for a random value that cannot exist. A hit on that random value means the
  filter was ignored (the field name is wrong), and Test fails.
- **HTTPS only.** The instance URL must be an absolute `https://` address with no
  credentials, query or fragment. Redirects are refused, so the password is never sent to
  another host.

## Priorities

| McHarbor severity | Default priority name |
|---|---|
| `critical` | `5 very high` |
| `warning` | `4 high` |
| `info` | `3 normal` |

Priorities are sent by **name**. Admins who renamed them override any of the three with
`severity_map`, for example `{"critical":"P1 - Kritisch"}`.

## Status coming back

The state type decides, when the payload carries one; otherwise the stock state name
does. Names listed in **Extra resolved states** or **Extra cancelled states** win over
both, so a renamed state still maps.

| State type / stock state | Tickets tab |
|---|---|
| `new` | Open |
| `open` | Acknowledged |
| `pending reminder`, `pending auto` / `pending auto close+`, `pending auto close-` | Acknowledged |
| `closed` / `closed successful` | Resolved |
| `closed` / `closed unsuccessful` | Closed |
| `merged` | Closed |
| `removed` | Cancelled |
| anything else | Open |

The assignee is the ticket **Owner**; `root@localhost` counts as nobody.

- **Webhook.** A GenericInterface requester with an invoker on the `TicketStateUpdate`
  event posts to the channel's inbound URL. GenericInterface cannot sign a request, so
  each delivery must carry the header `X-McHarbor-Webhook-Secret` with the channel's
  signing secret; it is compared in constant time, and a channel without a secret rejects
  every delivery. Both invoker shapes are read: Znuny's `Ticket::Generic`
  (`Ticket.StateType` and flat `DynamicField_<name>` keys) and OTOBO's
  `Ticket::TicketUpdate` (`TicketID` at the root and a `Ticket.DynamicField` list).
  Deliveries for tickets McHarbor did not open are ignored by the host.
- **Polling.** Open tickets are read with `POST /TicketGet`, 50 per call, every **Poll
  interval**. When a batch fails because one ticket was deleted, the batch is retried per
  ticket and the deleted one is skipped. `0` turns polling off.

## The web service files

Two importable definitions ship in the plugin, and the setup guide on the channel form
copies either to the clipboard:

| File | For |
|---|---|
| [`McHarborConnectorREST.yml`](../../../../src/plugins/Plugins/comm-otrs/webservice/McHarborConnectorREST.yml) | Znuny and OTRS 6/7: invoker `Ticket::Generic` |
| [`McHarborConnectorREST-otobo.yml`](../../../../src/plugins/Plugins/comm-otrs/webservice/McHarborConnectorREST-otobo.yml) | OTOBO: invoker `Ticket::TicketUpdate` with `RequestTicketFields` `State`, `Owner`, `DynamicField` |

Both define the same provider routes. The requester's host, controller and secret header
hold placeholders that you replace after import.

## OTRS 8 and later

OTRS 8 replaced `Session::SessionCreate` with `AccessToken::Create`. The plugin reads an
`AccessToken` from the session response when no `SessionID` is present and sends it the
same way, but this has not been verified against a live OTRS 8 system, and it is not known
whether OTRS SaaS tenants allow custom web services. Import the **Provider** part only,
keep polling on, and rely on **Test connection** to tell you whether it works. The channel
form shows a notice when the product is set to OTRS.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The agent password is stored encrypted and decrypted only at call time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files cannot ride along with a ticket. |
| Incident lifecycle | Yes | Open, Update and Resolve, an inbound webhook, and polling. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order in the ITSM and in McHarbor |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): how a ticket follows an alert
- [Communications](../../../settings/integrations/communications/README.md): channels and the inbound URL
- [Plugin catalog](../../catalog.md): every shipped plugin
