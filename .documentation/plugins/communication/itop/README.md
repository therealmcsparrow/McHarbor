# iTop

Opens, logs and resolves Combodo iTop tickets from McHarbor alerts. The plugin talks to the
iTop REST/JSON web service (`POST <instance_url>/webservices/rest.php?version=1.3` with a
single form field `json_data`) and reads ticket status back through the Combodo Webhook
Integration or by polling.

- **Slug** `comm-itop`, **version** 2026.09.01, **channel type** `itop`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type; its tickets are listed under Settings → Alerts → Tickets
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels.use_comm_itop` (global) to route events through this channel
- **Ticket classes** `Incident` (default) or `UserRequest`
- **iTop versions** 3.1 and later with an application token; 2.7 and 3.0 with a REST user and password. The webhook recipe needs the Combodo Webhook Integration extension, bundled with iTop Community 3.x; without it the channel polls.

## How it works

### The lifecycle, in words

iTop is a state machine, and McHarbor walks it rather than writing a status field.

1. **Firing.** McHarbor looks for an open ticket whose title starts with this alert's
   marker. If it finds one, it reuses it. Otherwise it creates one with `core/create`,
   in state `new`.
2. **Firing again.** When `update_on_repeat` is on, each repeat is appended to the
   ticket's private log (`core/update`, `private_log`, format `text`). The state does not
   change.
3. **Recovered.** McHarbor first reads the ticket's status.
   - `resolved`, `closed` or `rejected`: a person got there first. Nothing is sent.
   - `new`, `escalated_tto`, `pending` (and, for User Requests, `dispatched`,
     `redispatched`, `approved`): iTop does not allow `ev_resolve` from these states, so
     McHarbor applies `ev_assign` with the configured `team_id` and `agent_id` first.
   - `assigned` or `escalated_ttr`: resolve directly.

   It then applies `ev_resolve` with `service_id`, `resolution_code` and `solution` (the
   three fields iTop makes mandatory in the `resolved` state) and a private-log line.
4. **Close.** With `close_on_resolve` on, `ev_close` follows immediately.
5. **Flapping.** With `reopen_window_minutes` above 0, a firing alert whose ticket was
   resolved within that many minutes gets `ev_reopen` on the same ticket instead of a new
   one. The window is measured by iTop's own clock.

Any other state, or a stimulus iTop refuses because someone moved the ticket at the same
moment, stops the delivery with iTop's own message. The next poll or webhook reconciles
the state.

### Deduplication

iTop has no correlation field, so McHarbor writes a marker at the start of the title:
`[MH-` followed by the first ten hex characters of the SHA-256 of the alert's dedup key,
then `]`. For example `[MH-3fa91c07d2] Disk almost full`. The marker is how a retried
create finds the ticket it already made, and how the webhook trigger's filter picks out
McHarbor's tickets. Do not edit it out of a title while the alert is open.

### Severity

| McHarbor | `impact` | `urgency` |
|---|---|---|
| critical | 1 (department) | 1 (critical) |
| warning | 2 (service) | 2 (high) |
| info | 3 (person) | 3 (medium) |

A larger `impact` number is a smaller impact in iTop. `priority` is computed by iTop from
the two and is never sent. `severity_map` overrides any entry.

### Status read back

| iTop status | McHarbor state |
|---|---|
| `new`, `escalated_tto`, `waiting_for_approval`, `approved` | open |
| `assigned`, `escalated_ttr`, `pending`, `dispatched`, `redispatched` | acknowledged |
| `resolved` | resolved |
| `closed` | closed |
| `rejected` | cancelled |

A ticket deleted in iTop is reported as gone the next time McHarbor touches it, and its
link is marked orphaned.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The token or password is stored encrypted at rest and decrypted only for the call. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files riding along with the message. |

It also implements the host's incident lifecycle, the inbound webhook parser and the
poller, so the channel appears as a ticketing channel rather than a plain notifier.

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites in iTop, every field, the webhook recipe and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
