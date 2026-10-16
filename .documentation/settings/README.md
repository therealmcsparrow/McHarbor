# Settings

`/settings`

Twenty sections in six groups. Each section has a folder with two pages: what it governs,
and a reference table of every setting with its default and what changing it costs.

The **group** is how the interface navigates; the **section id** is the unit of navigation
in a URL, so `?tab=<id>` keeps resolving no matter how groups are rearranged.

## The six groups

| Group | Answers | Sections |
|---|---|---|
| [General](./general/README.md) | How does the app look and behave? | General, Themes, Notifications |
| [Integrations](./integrations/README.md) | What is it wired up to? | AI, Communications, Storage, Snapshots, Logging, Catalogs |
| [Monitoring](./monitoring/README.md) | What does it tell me about? | Alerts, Data retention |
| [Automation](./automation/README.md) | What does it do on its own? | Schedules, Webhooks |
| [Updates](./updates/README.md) | What keeps itself current? | Containers, Agents, McHarbor |
| [System](./system/README.md) | What is it running as? | Database, Agent, Licence, About |

## Every section

| Section | `?tab=` | Covers | Gated by |
|---|---|---|---|
| [General](./general/general/README.md) | `general` | Language, time zone, date and time formats, and how often data refreshes | — |
| [Themes](./general/themes/README.md) | `themes` | Theme variant, accent colour, fonts, motion, and per-variable overrides | — |
| [Notifications](./general/notifications/README.md) | `notifications` | Where toasts appear, how long they stay, and what users may override | `notifications.view` |
| [AI](./integrations/ai/README.md) | `ai` | Connect AI providers so AI-backed features have a model to call | `ai_assistant.view` |
| [Communications](./integrations/communications/README.md) | `communications` | Chat and messaging channels that alerts and events are delivered to | `communications.view` |
| [Storage](./integrations/storage/README.md) | `storage` | Reusable storage locations, and the backup encryption key | `storage_locations.view` |
| [Snapshots](./integrations/snapshots/README.md) | `snapshots` | Filesystem snapshot drivers detected per environment | — |
| [Logging](./integrations/logging/README.md) | `logging` | Forward audit and application logs to an external platform | `audit.view` |
| [Catalogs](./integrations/catalogs/README.md) | `catalogs` | Sources McHarbor reads app and plugin definitions from | — |
| [Alerts](./monitoring/alerts/README.md) | `alerts` | Rules that watch metrics and container state, and where they deliver | `alerts.view` |
| [Data retention](./monitoring/data-retention/README.md) | `retention` | How long audit, activity, lifecycle, metrics and scan history are kept | `settings.view` |
| [Schedules](./automation/schedules/README.md) | `schedules` | Recurring jobs McHarbor runs on its own | `schedules.view` |
| [Webhooks](./automation/webhooks/README.md) | `webhooks` | Outbound HTTP callbacks fired on McHarbor events | `webhooks.view` |
| [Containers](./updates/containers/README.md) | `updates-containers` | Policies that check for and apply new container images | `updates.view` |
| [Agents](./updates/agents/README.md) | `updates-agents` | Keep remote agents on the version this McHarbor expects | `updates.view` |
| [McHarbor](./updates/mcharbor/README.md) | `updates-mcharbor` | Check for new McHarbor releases and install them | `updates.view` |
| [Database](./system/database/README.md) | `database` | Storage engine, size, and maintenance | `database.view` |
| [Agent](./system/agent/README.md) | `agent` | Defaults and diagnostics for the remote agent transport | — |
| [Licence](./system/license/README.md) | `license` | Licence key, edition, and entitlement status | `license.view` |
| [About](./system/about/README.md) | `about` | Version, dependencies, and factory reset | — |

Sections with no key in the last column are readable by anyone who can reach Settings
(General, Themes and About) or have no dedicated key of their own yet, which is the case for
Snapshots, Catalogs and Agent. All three are covered by `settings.manage` on the backend;
the gap is that they cannot be granted independently.

## What is not here

Two groups of settings live on other screens:

| Setting | Where |
|---|---|
| HTTPS certificate, users, groups, roles, policies, SCIM, API keys, identity providers, scanners, password and session policy | [Security](../screens/security/README.md) |
| Your own theme, language, password, MFA and toast overrides | [Profile](../screens/profile/README.md) |

And anything that has to be true before the database opens (listeners, TLS, the database
connection, the secrets) is an environment variable rather than a setting. See
[Configuration](../getting-started/configuration/README.md).

## Related

- [Screens](../screens/README.md): every screen in the interface
- [Getting started](../getting-started/README.md): install, first run, configuration
- [Plugins](../plugins/README.md): what fills the Integrations group
