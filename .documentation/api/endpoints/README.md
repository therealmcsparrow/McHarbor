# Endpoints

Every route the backend mounts, grouped by area. 724 endpoints across 28 pages.

Each page opens with the permission keys that govern the area and the handful of things that
are not obvious from a route table, then lists every route with its permission, parameters
and request body.

| Page | Endpoints | Covers |
|---|---|---|
| [Containers](./containers.md) | 41 | Lifecycle, configuration, files, exec, health, move |
| [Images and registries](./images.md) | 15 | Pull, import, export, tag, prune; registry credentials |
| [Volumes](./volumes.md) | 5 | Create, inspect, prune, remove |
| [Networks](./networks.md) | 7 | Create, connect, disconnect, remove; host interfaces for macvlan and ipvlan |
| [Stacks](./stacks.md) | 29 | Compose projects, deployment, adoption, stack webhooks |
| [Environments, agents and consoles](./environments.md) | 32 | Hosts, duplicates, merge and connection migration; the remote agent, its SSH access and redeploy; VM and LXC consoles |
| [Host, metrics and logs](./host.md) | 39 | Daemon info, metrics, McHarbor's own Prometheus exposition, events, log streams, terminals, OS updates, restart and reset |
| [Backups, restore and storage](./backups.md) | 46 | Plans, runs, provider plans and runs, the restore browser, destinations, snapshots |
| [Instance backups](./instance-backups.md) | 17 | Exporting the whole instance, schedules, verification, restore |
| [Authentication](./auth.md) | 35 | Login, setup, profile, preferences, sessions, MFA, the step-up window, table views, CLI device sign-in |
| [Users, groups and elevations](./users.md) | 41 | Accounts, bulk actions, the lifecycle report, membership, role assignments, overrides, the elevation queue |
| [Roles and policies](./roles.md) | 19 | Roles, presets, impact analysis, ABAC policies, user labels |
| [Approvals, break-glass and access review](./governance.md) | 23 | The approval queue and rules, break-glass sessions, the permission simulator, access-review exports |
| [API keys](./api-keys.md) | 10 | Minting, rotation, grants, revocation, usage telemetry |
| [Secrets vault](./secrets.md) | 14 | Stored credentials, rotation, references, apply, reveal |
| [Identity providers and SCIM](./identity.md) | 31 | SSO providers and their setup helpers, the SCIM 2.0 surface |
| [Workflows, nodes, schedules and webhooks](./workflows.md) | 32 | The workflow engine, custom nodes, cron schedules, outbound webhooks |
| [Git, GitOps and the reconciler](./gitops.md) | 48 | Repositories, promotion pipelines, approvals, desired state, plans, enforcement, Git sources |
| [Incidents and ITSM](./incidents.md) | 20 | Incidents, evidence, narration, behaviour findings, ITSM channels and ticket links |
| [Dashboard, audit, alerts and notifications](./monitoring.md) | 49 | Dashboard data, widgets, the audit log and its forwarders, alert rules, channels, the inbox |
| [App store, plugins and catalogs](./store.md) | 44 | Docker apps, the plugin store, the plugin loader, catalog sources |
| [Push and install](./push.md) | 8 | The phone app's manifest and icons, Web Push devices, the instance push keys |
| [AI assistant and providers](./ai.md) | 22 | Chat, proposed actions, provider connections |
| [Scans and auto-heal](./security.md) | 17 | Vulnerability scanning, scanner instances, per-container auto-heal |
| [System, updates and licence](./system.md) | 40 | Health and probes, versions, self-update, licence, cluster and HA control, database wizard, CLI downloads |
| [Database](./database.md) | 11 | The active database, connection test, health check and repair, migration, cleanup |
| [Support and diagnostics](./support.md) | 10 | Diagnostics, state inventory, pre-upgrade check, support bundles |
| [Settings](./settings.md) | 27 | The settings API and the public branding read; every key is in the [settings reference](../settings/reference.md) |

## Finding an endpoint

| If you are looking for | Go to |
|---|---|
| Something you saw a screen do | The area that matches the screen |
| A permission key you were denied | [Permissions](../conventions/permissions.md), then follow it here |
| An error `code` you received | [Errors](../conventions/errors.md) |
| A stream | [Streaming](../conventions/streaming.md) lists every SSE and WebSocket route |
| A setting key | [Settings reference](../settings/reference.md) |

## Reading the tables

| Column | Means |
|---|---|
| **Method** | The HTTP verb, exactly as mounted |
| **Path** | The full path including `/api` |
| **Permission** | The key the route middleware checks. `—` means no permission middleware; the page says why |
| **What it does** | One line, from the handler |
| **Parameters** | Path parameters, query parameters, multipart fields |

Request bodies follow each table. A `?` suffix marks an optional field: on a create it means
"use the default", on an update it means "leave unchanged".
