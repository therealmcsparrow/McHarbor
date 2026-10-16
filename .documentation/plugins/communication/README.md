# Communication Channels

A communication-channel plugin gives McHarbor somewhere to send a notification. The host
owns the channel rows, the credential encryption, the routing rules and the retry; the
plugin supplies two calls: test the configuration, and send one title, message, level
and optional attachment.

Alerts, workflow notify nodes, backup outcomes and schedule failures all route through
the same channels.

| Plugin | Channel type | Attachments | What it delivers to |
|---|---|---|---|
| [Discord](./discord/README.md) | `discord` | Files | Discord communication channel. |
| [Exchange](./exchange/README.md) | `exchange` | Files | Exchange Online communication channel. |
| [Gmail](./gmail/README.md) | `gmail` | Files | Gmail communication channel. |
| [Gotify](./gotify/README.md) | `gotify` | Text only | Gotify communication channel. |
| [Microsoft Teams](./microsoft-teams/README.md) | `teams` | Text only | Microsoft Teams Incoming Webhook connector channel. |
| [ntfy](./ntfy/README.md) | `ntfy` | Files | ntfy.sh or self-hosted ntfy notification channel. |
| [Signal](./signal/README.md) | `signal` | Files (REST API method only) | Signal notification channel. |
| [Slack](./slack/README.md) | `slack` | Text only | Slack communication channel. |
| [SMTP](./smtp/README.md) | `smtp` | Files | SMTP communication channel. |
| [Telegram](./telegram/README.md) | `telegram` | Files | Telegram Bot API notification channel. |
| [WhatsApp](./whatsapp/README.md) | `whatsapp` | Files (Cloud API method only) | WhatsApp notification channel. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## Ticketing and on-call

These plugins also implement the incident lifecycle: an alert opens a ticket, repeat firings
add notes, recovery resolves it, and the ITSM's own status comes back. See
[Tickets](../../settings/monitoring/alerts/tickets/README.md) for how the host drives them.

| Plugin | Channel type | Status back from the ITSM | What it talks to |
|---|---|---|---|
| [Alertmanager webhook](./alertmanager-webhook/README.md) | `alertmanager_webhook` | One-way | Any receiver that ingests Alertmanager webhook v4: Grafana IRM, Squadcast, Zenduty, Keep. |
| [GLPI](./glpi/README.md) | `glpi` | Webhook (GLPI 11) or polling | GLPI 9.5, 10 and 11 tickets through the legacy or the high-level API. |
| [iTop](./itop/README.md) | `itop` | Webhook or polling | Combodo iTop incidents or user requests, following the iTop lifecycle. |
| [OTRS / Znuny / OTOBO](./otrs/README.md) | `otrs` | Webhook or polling | Tickets through a GenericInterface REST web service the plugin ships. |
| [PagerDuty](./pagerduty/README.md) | `pagerduty` | Signed v3 webhook or REST polling | Incidents through Events API v2. |
| [Request Tracker](./request-tracker/README.md) | `request_tracker` | Signed Scrip callout or polling | RT 5 and 6 tickets through REST2. |
| [Zammad](./zammad/README.md) | `zammad` | HMAC-signed trigger webhook or polling | Zammad tickets through the REST API. |

## What they have in common

- They register through `AdapterRegistry.Notification()` and appear in the channel-type
  dropdown under **Settings → Integrations → Communications**.
- Each declares one permission key of its own, `channels.use_comm_<name>` at global scope, so
  a role can be allowed to route events to a team chat but not to the paging channel.
- Secrets are encrypted at rest with AES-256-GCM. Microsoft Teams is the exception: its
  connector URL is held as plain configuration, so treat that URL with the same care you
  would a token.
- `SupportsRichAttachments` is false on every shipped channel. Messages are a title and a
  body; Teams renders that as an Adaptive Card, the rest as text.
- **Test sends a real message** on the chat and push channels. The ticketing plugins below
  test by reading from the ITSM instead, so a test never opens a ticket.

## Choosing one

| You want | Use |
|---|---|
| A chat channel your team already watches | Slack, Discord, Microsoft Teams |
| Push to phones without a chat product | ntfy, Gotify, Telegram |
| Mail to a mailbox, list or ticket queue | SMTP first; Gmail or Exchange when SMTP is disabled by policy |
| A message to a personal phone number | Signal, WhatsApp (both need a gateway you run or trust) |

## Related

- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
