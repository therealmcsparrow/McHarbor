# Request Tracker

Opens, comments on and resolves tickets in Best Practical Request Tracker (RT) from McHarbor
alerts, through RT's REST2 API. Status changes come back either through a signed Scrip callout
that the RT administrator installs, or through polling.

- **Slug** `comm-request-tracker`, **version** 2026.09.01, **channel type** `request_tracker`
- **Host range** `>=2026.09.01`
- **RT versions** 5.0 and 6.0, where REST2 (`/REST/2.0/`) and auth tokens are built in. RT 4.4 needs
  `RT::Extension::REST2` and `RT::Authen::Token` and is not tested.
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one
  and to create the inbound URL, and `channels.use_comm_request_tracker` (global) to route events
  through this channel

## How it works

| McHarbor event | RT call |
|---|---|
| Alert fires | `GET /REST/2.0/tickets?query=…` for an active ticket carrying the dedup value; on a miss, `POST /REST/2.0/ticket` in the configured queue, then `GET /REST/2.0/ticket/{id}` to prove RT stored the dedup value |
| Alert fires again while the ticket is open | `POST /REST/2.0/ticket/{id}/comment`: an internal comment, when `update_on_repeat` is on |
| Alert recovers | One `POST /REST/2.0/ticket/{id}/comment` that carries the closing note **and** `Status: <resolve_status>` |
| Send test notification / plain notifications | `Test` only reads (see [Configuration](./configuration.md#what-test-connection-checks)); a plain notification creates a ticket with no dedup value |

- **Dedup is native to RT.** A custom field (default name `McHarbor Dedup Key`) applied to the
  queue holds the SHA-256 hex digest of McHarbor's dedup key. The search is
  `Queue = '<queue>' AND 'CF.{<field>}' = '<hex>' AND Status = '__Active__'`, so a retried create
  finds the ticket instead of opening a second one, and a new firing after recovery opens a fresh
  ticket.
- **Comments, never correspondence.** McHarbor uses `/comment`, which RT does not mail to
  requestors. `/correspond` is never called.
- **No requestor by default.** With the `requestor` field blank, a queue's "On Create Autoreply To
  Requestors" scrip has nobody to e-mail. Set a requestor only if you want that mail.
- **Reopen window.** With `reopen_window_minutes` above 0, an alert that fires again within that
  many minutes of its ticket being resolved reopens that ticket (status `open`, with a comment)
  instead of creating a new one. A rejected or deleted ticket is never reopened.
- **Only safe values reach TicketSQL.** The queue and custom-field names must match
  `^[\p{L}\p{N} _.\-]{1,200}$`, the dedup value is hex, ticket ids must be decimal, and every
  literal is quoted with `'` escaped as `\'` on top of that.
- **HTTPS only.** `instance_url` must be an absolute `https://` URL without credentials, a query or
  a fragment, and redirects are not followed, so the token is never sent anywhere but the
  configured host.

### Severity → RT priority

| McHarbor | Priority |
|---|---|
| critical | 90 |
| warning | 50 |
| info | 20 |

`severity_map` overrides entries, for example `{"critical":99}`.

### RT status → McHarbor ticket state

| RT status | McHarbor state |
|---|---|
| `new` | open |
| `open` | acknowledged |
| `stalled` | acknowledged |
| `resolved` | resolved |
| `rejected` | cancelled |
| `deleted` | cancelled |

`status_map` adds statuses from a custom lifecycle, for example `{"waiting":"acknowledged"}`.
Values must be `open`, `acknowledged`, `resolved`, `closed` or `cancelled`; a status that maps to
nothing is ignored. The ticket's owner is reported as the assignee unless it is `Nobody`.

## Status updates from RT

RT has no built-in outbound webhook, so McHarbor defines its own signed callout, sent by a Scrip
the RT administrator installs. The Scrip and the site-configuration file ship with the plugin as
`assets/mcharbor-scrip.pl` and `assets/McHarbor.pm`, and the channel's setup guide shows both with
a copy button. The recipe is on the [Configuration](./configuration.md#status-updates-scrip-callout)
page.

Without the Scrip, McHarbor polls open tickets every `poll_interval_minutes` (default 5), up to 50
tickets per TicketSQL search.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The auth token is stored encrypted at rest and decrypted only at call time. |
| `SupportsRichAttachments` | No | Tickets are plain text. |
| `SupportsFileAttachments` | No | No files are attached to tickets. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | RT rights, the custom field, every field, the Scrip and site-config recipe, the signing scheme |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
