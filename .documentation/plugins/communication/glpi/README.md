# GLPI

Opens, follows up and solves GLPI tickets for McHarbor alerts, and reads ticket status back.
One channel type covers GLPI 9.5, 10 and 11: it talks either to the legacy **REST API**
(`apirest.php`, every supported version) or to GLPI 11's **high-level API**
(`api.php/v2.x`). Status comes back through signed **GLPI 11 webhooks**, or by polling on
9.5 and 10.

- **Slug** `comm-glpi`, **version** 2026.09.01, **channel type** `glpi`
- **Host range** `>=2026.09.01`
- **Covers** GLPI 9.5 and 10 (API v1), GLPI 11 (API v1 or v2). GLPI 12 is unverified
- **Where it appears** Settings → Integrations → Communications, as a channel type; its
  tickets are listed under Settings → Monitoring → Alerts → Tickets
- **Gated by** `communications.view` to see channels, `communications.manage` to add or
  change one or to create its inbound URL, and `channels.use_comm_glpi` (global) to route
  events through this channel

## How it works

- **Two APIs, one behaviour.** `api_version` picks the surface. Open, Update, Resolve,
  Test and polling do the same thing on both; only authentication and the URLs differ.

  | | API v1 (`v1`) | API v2 (`v2`) |
  |---|---|---|
  | Base | `<GLPI URL>/apirest.php/` | `<GLPI URL>/api.php/<v2_path>` |
  | Credentials | User API token, plus the API client's App-Token when it has one | OAuth client ID and secret, and an API user's login and password (password grant, scope `api`) |
  | Session | `initSession` per call chain, `killSession` when it ends | Bearer token, cached until 60 seconds before `expires_in` |
  | Versions | 9.5, 10, 11 | 11 and later |

- **v1 sessions never outlive a call.** Every Open, Update, Resolve, Test and poll batch
  opens its own GLPI session and kills it afterwards, also when the ticket call failed,
  and with its own short timeout, so a cancelled request cannot leak a PHP session. When
  GLPI reports a session gone mid-chain (`ERROR_SESSION_TOKEN_INVALID`), McHarbor opens a
  new one and replays the request once.
- **Deduplication uses the ticket's `externalid`.** Each ticket McHarbor opens carries
  `mcharbor:` followed by the first 40 hex characters of the SHA-256 of the alert's ticket
  key (`external_id` in API v2). Before creating, Open searches for it (v1 with
  `searchText[externalid]`, v2 with the RSQL filter `external_id==…;status.id=lt=5`) and
  reuses a ticket only when `externalid` matches **exactly**, its status is below Solved
  and it is not in the trash. `searchText` is a `LIKE` match, so the exact comparison is
  what stops a longer key from being mistaken for this one. A retried Open after a lost
  response therefore never creates a second ticket.
- **Open** creates an Incident (`type` 1) whose title is the alert title (255 characters at
  most) and whose content is minimal HTML: the message, the severity, state, alert,
  environment, subject, source and labels, every value escaped, and the alert link when it
  is http(s). The entity, ITIL category and, on API v1, the assigned group are set when
  their IDs are above 0.
- **Update** adds one **private** follow-up.
- **Resolve** adds a solution, which moves the ticket to **Solved**. With
  *Close after solving* on, it then sets the status to **Closed** (6). When GLPI refuses the
  solution and the ticket is already Solved or Closed, Resolve counts as done, so a retry is
  harmless.
- **Severity** maps to GLPI's urgency, impact and priority. Priority is sent explicitly,
  because an instance's priority matrix may be set to ignore urgency × impact.

  | McHarbor | urgency | impact | priority |
  |---|---|---|---|
  | `critical` | 5 | 5 | 6 (Major) |
  | `warning` | 4 | 3 | 4 |
  | `info` | 2 | 2 | 2 |

  `severity_map` overrides any row, for example
  `{"info":{"urgency":3,"impact":3,"priority":3}}`.
- **Test never creates a ticket.** On API v1 it opens a session, reads
  `getActiveEntities` and fails when `entities_id` is set and is not among the API user's
  active entities, then kills the session. On API v2 it obtains a token and reads one
  ticket from `Assistance/Ticket`.
- **Plain notifications** (a workflow notify node, a backup outcome) become a ticket with
  no `externalid`, so the incident lifecycle never mistakes one for its own.
- **https only.** The GLPI URL must be `https://`, without credentials, query or fragment;
  a trailing `/apirest.php` or `/api.php` is stripped. Redirects are never followed,
  because the App-Token and Session-Token headers would travel to the new host.

## Status coming back

| GLPI status | Tickets tab |
|---|---|
| 1 New | Open |
| 2 Processing (assigned), 3 Processing (planned), 4 Pending | Acknowledged |
| 5 Solved | Resolved |
| 6 Closed | Closed |
| In the trash (`is_deleted`), or a `delete` / `purge` webhook event | Cancelled |

Any other status is dropped rather than guessed.

- **GLPI 11 webhooks.** A core webhook for the Ticket itemtype posts
  `{"event":…,"item":{…}}` to the channel's inbound URL. Each delivery is checked against
  `X-GLPI-signature`, the hex HMAC-SHA256 of the body followed by the `X-GLPI-timestamp`
  value, compared in constant time. A delivery whose timestamp is more than 300 seconds
  from McHarbor's clock, in either direction, is refused even when the signature is valid,
  so a captured delivery cannot be replayed. Tickets whose external ID does not start with
  `mcharbor:` are ignored without error, because the webhook's filter may be broader.
- **The challenge-response option (CRA) must stay off.** With it on, GLPI first sends a
  `GET` with `?crc_token=` to the URL, which the inbound route does not answer, so GLPI
  never delivers. An instance that sets `GLPI_WEBHOOK_CRA_MANDATORY` cannot use webhooks
  with McHarbor; use polling.
- **Polling.** GLPI 9.5 and 10 have no webhooks. Every *Poll interval*, open tickets are
  read one by one, four at a time, inside one session (v1) or with one token (v2). A ticket
  GLPI no longer has (404) is left out of the batch; one ticket that cannot be read does
  not hide the others.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The tokens, OAuth secret, password and webhook secret are stored encrypted and decrypted only at call time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files cannot ride along with a ticket. |
| Incident lifecycle | Yes | Open, Update and Resolve, an inbound webhook, and polling. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order in GLPI and McHarbor |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): how a ticket follows an alert
- [Communications](../../../settings/integrations/communications/README.md): channels and the inbound URL
- [Plugin catalog](../../catalog.md): every shipped plugin
