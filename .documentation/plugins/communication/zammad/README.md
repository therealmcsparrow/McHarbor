# Zammad

Opens, notes and closes Zammad tickets for McHarbor alerts, and reads ticket state back.
Writes go through Zammad's **REST API** (`/api/v1`) with a personal access token; status
comes back through a Zammad **Trigger** calling a **Webhook** signed with HMAC-SHA1, or by
polling the ticket.

- **Slug** `comm-zammad`, **version** 2026.09.01, **channel type** `zammad`
- **Host range** `>=2026.09.01`
- **Covers** Zammad 6.x and 7.x, self-hosted or hosted
- **Where it appears** Settings → Integrations → Communications, as a channel type; its
  tickets are listed under Settings → Monitoring → Alerts → Tickets
- **Gated by** `communications.view` to see channels, `communications.manage` to add or
  change one or to create its inbound URL, and `channels.use_comm_zammad` (global) to
  route events through this channel

## How it works

- **A token of a dedicated agent user.** Every call carries
  `Authorization: Token token=<access token>`. The token's owner needs `ticket.agent` on
  the channel's group. With **Act on behalf of** set, every call also carries a `From`
  header naming that user, which Zammad honours only when the token owner has
  `admin.user`.
- **The ticket key is a title marker.** Zammad has no correlation field, so every ticket
  McHarbor opens starts with `[MH-<10 hex>]` (a SHA-256 of the ticket key and the moment
  the alert fired), followed by the alert title, cut to Zammad's 250 characters. The ticket
  is also tagged `mcharbor`, which is what the status trigger matches on.
- **Open never duplicates.** Before creating, McHarbor searches
  `/api/v1/tickets/search` for the marker and accepts a hit only when the title **starts
  with** the exact marker and the state is neither `closed` nor `merged`. Without
  Elasticsearch, Zammad's search falls back to a fuzzy database match, and this filter is
  what keeps a forwarded or quoted marker from being taken for the ticket. A retried open
  after a lost response, or after the tag call failed, finds the ticket it already created
  and tags it.
- **A new firing is a new ticket.** Because the marker includes the moment the alert
  fired, an alert that recovers and fires again opens a fresh ticket, unless the **Reopen window**
  is set: a repeat firing of the same condition within that many minutes of the ticket being closed reopens it (state `open`, with a note) instead. The title carries a second marker, `[MC-<hash>]`, that identifies the condition across firings, and the search for the closed ticket matches that marker only. A merged ticket never returns.
- **Open** creates the ticket in **Group** for **Customer**, with the mapped priority, the
  optional **Owner login**, and a first internal note describing the alert. **Update** (an
  alert firing again while its ticket is open, when **Add a note on repeat** is on) adds an
  internal note (`POST /api/v1/ticket_articles`). **Resolve** is a single
  `PUT /api/v1/tickets/{id}` that sets **State on recovery** and adds the recovery note, so
  the note and the close land together.
- **Severity** maps `critical` → `3 high`, `warning` → `2 normal`, `info` → `1 low`.
  `severity_map` overrides any of the three with another priority name.
- **Plain notifications** (a workflow notify node, a backup outcome) create an ordinary
  ticket in the same group, with no marker and no tag, so neither dedup nor the status
  trigger touches it.
- **Test never creates anything.** It calls `GET /api/v1/users/me` to prove the token,
  reads `GET /api/v1/groups` to check the group exists and is active, and, unless the
  customer starts with `guess:`, searches `GET /api/v1/users/search` for a user whose login
  or email equals **Customer**.
- **HTTPS only.** The Zammad URL must be an absolute `https://` address with no
  credentials, query or fragment. Redirects are refused, so the token is never carried to
  another host.

## Status coming back

| Zammad state | Tickets tab |
|---|---|
| `new` | Open |
| `open` | Acknowledged |
| `pending reminder` | Acknowledged |
| `pending close` | Resolved |
| `closed` | Closed |
| `merged` | Closed |
| any other state | Acknowledged |

`status_map` maps custom states to `open`, `acknowledged`, `resolved`, `closed` or
`cancelled`. The assignee is the ticket owner's login; Zammad's unassigned placeholder
user `-` counts as nobody. A ticket merged into another one reads as **Closed**, and
McHarbor stops updating it.

- **Webhook.** A Zammad Trigger calls a Zammad Webhook pointed at the channel's inbound
  URL. Each delivery must carry `X-Hub-Signature: sha1=<hex>`, the HMAC-SHA1 of the raw
  body keyed with the channel's signing secret, compared in constant time. SHA-1 is the
  only algorithm Zammad offers; that is acceptable for an HMAC, and both ends are HTTPS.
  Zammad signs no timestamp, so a replayed delivery only re-applies a state McHarbor
  already has. Tickets whose title does not start with `[MH-` are ignored.
- **Polling.** Each open ticket is read with `GET /api/v1/tickets/{id}?expand=true`, four
  at a time, every *Poll interval*. A ticket that answers 404 is skipped. `0` turns polling
  off.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The access token is stored encrypted and decrypted only at call time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files cannot ride along with a ticket. |
| Incident lifecycle | Yes | Open, Update and Resolve, an inbound webhook, and polling. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order in Zammad and McHarbor |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): how a ticket follows an alert
- [Communications](../../../settings/integrations/communications/README.md): channels and the inbound URL
- [Plugin catalog](../../catalog.md): every shipped plugin
