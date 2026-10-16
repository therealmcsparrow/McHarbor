# Permissions

Authentication says who you are. This says what you may do. Every protected route names a
key; the middleware checks it before the handler runs; the frontend disables the control and
names the missing key rather than letting you find out from a 403.

## The shape of a key

`<category>.<action>`, lowercase, `snake_case` category. Three actions carry most of the
catalog:

| Action | Gates |
|---|---|
| `view` | Reads |
| `manage` | Create and update |
| `delete` | Removal |

A fourth verb appears only where those three genuinely do not cover the case: `run`,
`restore`, `export`, `assign`, `approve`, `access`, `use`, `act`, `download`.

One key is not in the catalog: **`*`**, the wildcard. A role holding it passes every check.
It is what the Admin preset carries, and what the synthetic `system` user gets when
authentication is disabled.

## Scopes

Every key declares the narrowest scope at which it can be enforced.

| Scope | Means | Example |
|---|---|---|
| `global` | The whole install | `settings.manage` |
| `environment` | Granted per Docker/Kubernetes environment | `containers.view` |
| `stack` | Granted per Compose stack | `stacks.manage` |
| `owner` | Applies to resources the user owns | `containers.manage` |
| `account` | Applies to an account, often the caller's own | `api_keys.manage` |

The scope is not decoration. Route middleware comes in matching variants:

| Middleware | Checks |
|---|---|
| `RequirePermission` | The key, honouring the request's `?env=` scope |
| `RequireGlobalPermission` | The key at global scope; an environment grant does not satisfy it |
| `RequireStackPermission` | The key for the stack named by a path parameter |
| `RequireAnyPermission` | Any one of several keys |
| `RequireAnyGlobalPermission` | Any one of several keys, at global scope |
| `RequireAnyStackPermission` | Any one of several keys, for the named stack |

Where a route accepts more than one key, the reference page writes them as
`` `settings.view` or `storage_locations.view` ``. Where a route needs two keys at once, they
are written with a `+`.

Route middleware knows the key, not the row. Where a permission is `owner`- or
`stack`-scoped, the service layer re-checks ownership on the specific record.
### A role granted on one environment carries only environment-scoped keys

A role assigned to a user or group **on one environment or stack** grants that role's
`environment`, `stack` and `owner` keys there, and nothing else. Its `global` and `account` keys
(`users.manage`, `roles.manage`, `environments.view`, `dashboard.view`, `alerts.*`,
`webhooks.*`, `plugins.manage` and the rest) count only when the role is assigned **globally**.
A `*` on a scoped role expands to exactly the environment-scoped keys. A key the catalog does
not know is dropped, so the check fails closed.

Before this rule a scoped role of `users.manage` passed a route gated by `users.manage` whenever
the request named that environment with `?env=`, which let an administrator of one environment
assign themselves the Administrator role. A user whose access came only from per-environment
roles therefore no longer sees global pages such as the dashboard or the environment list until
they are also given a global role that carries those keys.

An approved elevation for an environment follows the same rule: it adds only the
environment-, stack- and owner-scoped keys it names, never a global one.

### Which environment a route decides on

`?env=` is what the gate reads, so a route whose path or row already names an environment pins
the scope to it first, and a different `?env=` cannot borrow a grant from another environment:

| Pin | Applies to | What decides |
|---|---|---|
| `PinEnvToPath` | `/api/agents/{envId}/...` and `/api/environments/{id}/...` | The environment in the path. A `?env=` naming another one is overridden, not rejected |
| `PinEnvToResource` | `/api/secrets/{id}`, `/api/container-backups` runs, plans and snapshot configs, `/api/reconciler` watches, proposals and sources, `/api/scans/{id}`, `/api/incidents`, `/api/runbooks/{id}` and its runs, `/api/updates/{id}` | The environment the addressed row lives in. A row with no environment (an update policy, a secret with no environment) needs a **global** grant; a shared runbook keeps the request's own scope; a row that does not exist reaches the handler, which answers `404` |

A body that names a second environment is checked too: moving a container needs
`containers.manage` on the **target** environment as well as the source (`containers.view` for
`move/plan`), and creating or editing a reconciler watch or source checks its `envId` field. A
refusal is `403`. The generated `openapi.json` marks a pinned gate with `env_param` inside its
`x-mcharbor-permission` entry (`envId`, `id` or `resource`).

## Where a grant comes from

Effective permissions are the union of:

1. Roles assigned directly to the user, optionally narrowed to an environment or a stack. A narrowed role contributes only its environment-scoped keys (see above).
2. Roles inherited from the groups the user belongs to.
3. Per-user overrides: explicit grants and explicit denies (`rbac.policies.manage`).
4. ABAC policies, which allow or deny by expression against request attributes.
5. Active elevations: a time-boxed grant an admin approved.

`GET /api/auth/me/permissions` returns the resolved set for the caller. The frontend gates
affordances on it; the server still checks every request independently, so the two can never
drift into an actual privilege difference, only into a UI that is wrong about what is
possible.

## The full catalog

114 keys ship in the catalog, plus the `*` wildcard. `GET /api/auth/permissions/catalog`
returns the same list at runtime, with any keys plugins have added.

| Key | Scope | What it unlocks |
|---|---|---|
| `containers.view` | environment | View container details, logs, stats |
| `containers.manage` | owner | Start, stop, restart, exec into containers |
| `containers.delete` | environment | Remove or kill containers |
| `images.view` | environment | Inspect images and layers |
| `images.manage` | owner | Pull, push, tag, build images |
| `images.delete` | environment | Remove images from registry cache |
| `volumes.view` | environment | List and inspect volumes |
| `volumes.manage` | owner | Create volumes and mounts |
| `volumes.delete` | environment | Remove volumes |
| `networks.view` | environment | List and inspect networks |
| `networks.manage` | owner | Create, connect, disconnect networks |
| `networks.delete` | environment | Remove networks |
| `stacks.view` | environment | List and inspect Compose stacks |
| `stacks.manage` | stack | Create, edit, deploy stacks |
| `stacks.delete` | environment | Remove stacks |
| `environments.view` | global | List and inspect environments |
| `environments.manage` | global | Add, edit, remove environments |
| `users.view` | global | List and inspect users |
| `users.manage` | account | Update user profiles and roles |
| `settings.view` | global | View system settings |
| `settings.manage` | global | Update system settings |
| `communications.view` | global | View notification channels |
| `communications.manage` | global | Manage notification channels |
| `store_apps.view` | global | Browse the app catalog |
| `store_apps.manage` | global | Install, upgrade, uninstall apps |
| `store_nodes.view` | global | Browse workflow node catalog |
| `store_nodes.manage` | global | Install workflow nodes |
| `store_widgets.view` | global | Browse widget catalog |
| `store_widgets.manage` | global | Install widgets |
| `dashboard_widgets.install` | global | Install and remove downloaded dashboard widgets |
| `terminal.access` | environment | Open interactive terminal sessions |
| `logs.view` | environment | View container and workload logs |
| `api_keys.view` | account | List API keys and their grants |
| `api_keys.manage` | account | Create, edit, revoke API keys |
| `groups.view` | global | List groups and memberships |
| `groups.manage` | global | Create, edit, remove groups |
| `roles.view` | global | List roles and permissions |
| `roles.manage` | global | Create, edit, remove roles |
| `scans.view` | environment | View vulnerability scan results |
| `scans.manage` | environment | Trigger scans, configure scanners |
| `host.view` | environment | View host metrics and Docker daemon |
| `host.manage` | environment | Prune, restart daemon, manage host |
| `system.manage` | global | Self-management actions on McHarbor |
| `host_self.view` | global | View the services, OS updates and reboot state of the host McHarbor runs on |
| `host_self.manage` | global | Run service actions and install OS updates on the host McHarbor runs on, and act on McHarbor's own containers. Needed on top of the container key for exec, the terminal, the debug shell, file writes, lifecycle and config changes on a McHarbor container, and for creating a container that reaches the host on the environment McHarbor runs on; see [Containers](../endpoints/containers.md#mcharbors-own-containers) |
| `host_self.reboot` | global | Schedule or cancel a reboot of the host McHarbor runs on |
| `database.view` | global | View the database backend and migration status |
| `database.manage` | global | Change the database backend and migrate the data |
| `permissions.view` | global | Read the permission catalog and effective permissions |
| `permissions.assign` | global | Grant or revoke user-level permission overrides |
| `sessions.view` | account | List active sessions |
| `sessions.manage` | account | Revoke any user's sessions |
| `account.self.manage` | account | Update own profile, email, password, MFA. A session needs no key for these; an API key must carry it |
| `account.manage` | account | Create, disable, enable, delete accounts |
| `account.password.reset` | account | Issue admin-set password or email reset link |
| `audit.view` | global | Read the audit log |
| `audit.export` | global | Export audit entries as CSV/PDF |
| `in_app_notifications.view` | account | View in-app notification center |
| `workflows.view` | global | List and inspect workflows |
| `workflows.manage` | global | Create, edit workflows |
| `workflows.delete` | global | Delete workflows |
| `workflows.run` | global | Trigger a workflow manually |
| `custom_nodes.view` | global | List custom JavaScript nodes |
| `custom_nodes.manage` | global | Upload / edit custom JS nodes |
| `schedules.view` | global | List scheduled jobs |
| `schedules.manage` | global | Create / edit schedules |
| `schedules.delete` | global | Delete schedules |
| `webhooks.view` | global | List webhook endpoints |
| `webhooks.manage` | global | Create / edit webhooks |
| `webhooks.delete` | global | Delete webhooks |
| `alerts.view` | global | List alert rules |
| `alerts.manage` | global | Create / edit alert rules |
| `alerts.delete` | global | Delete alert rules |
| `autoheal.view` | environment | List autoheal configurations |
| `autoheal.manage` | environment | Configure autoheal |
| `backups.view` | environment | View backup plans, runs, and the backups page |
| `backups.manage` | owner | Create / edit backup plans and run backups |
| `backups.delete` | environment | Delete backup plans and runs |
| `backups.download` | environment | Download a backup archive |
| `restore.view` | environment | Browse the contents of a backup archive |
| `restore.run` | environment | Restore a backup into a container |
| `backup_log.view` | environment | View backup log entries |
| `git.view` | global | List git repos and credentials |
| `git.manage` | global | Add / edit git repos |
| `gitops.view` | global | List GitOps pipelines and previews |
| `gitops.manage` | global | Create / edit GitOps pipelines |
| `gitops.delete` | global | Delete GitOps pipelines |
| `gitops.approve` | global | Approve or reject promotion gates |
| `registry.view` | global | List container registries |
| `registry.manage` | global | Add / edit container registries |
| `plugins.view` | global | List installed plugins |
| `plugins.manage` | global | Install / configure plugins |
| `plugins.delete` | global | Uninstall plugins |
| `storage_locations.view` | global | List storage locations |
| `storage_locations.manage` | global | Add / edit storage locations |
| `storage_locations.delete` | global | Delete storage locations |
| `reconciler.view` | environment | View reconciler state |
| `reconciler.manage` | environment | Trigger reconciliation |
| `updates.view` | environment | View available updates |
| `updates.manage` | environment | Trigger an update |
| `identity_providers.view` | global | List identity providers |
| `identity_providers.manage` | global | Add / edit identity providers |
| `ai_assistant.view` | global | View AI assistant |
| `ai_assistant.use` | global | Send prompts to AI assistant |
| `ai_assistant.act` | global | Consent to and execute assistant-proposed actions |
| `ai_assistant.manage` | global | Configure AI assistant provider, models |
| `events.view` | environment | View Docker events |
| `activity.view` | global | View activity feed |
| `metrics.view` | environment | View metrics and charts |
| `dashboard.view` | global | View dashboard widgets |
| `widgets.view` | global | List widget definitions |
| `widgets.manage` | global | Install / configure widgets |
| `rbac.policies.view` | global | List ABAC policies and their history |
| `rbac.policies.manage` | global | Create, edit, delete, and enable ABAC policies |
| `scim.view` | global | List SCIM endpoints and sync history |
| `scim.manage` | global | Add, edit, delete, and trigger SCIM endpoints |
| `license.view` | global | View the licence, its entitlement, and recorded usage |
| `license.manage` | global | Request, refresh, and release the licence |


## Plugin keys

Plugins declare their own keys in their manifest rather than in the host catalog. The host
inserts them with the plugin's slug as their origin and merges them into the catalog the role
editor reads, so a plugin key can be granted to a role exactly like a host key. Two plugins
cannot share a key, and a plugin cannot shadow a host key.

The plugin route registry wraps every declared plugin route in its permission middleware, so
declaring a key correctly *is* enforcing it.

Storage-adapter plugins are a documented exception: they ship with no RBAC declarations,
because the host's `settings.manage` already gates every surface they mount.

## Keys that currently gate no route

Six catalog keys are not referenced by any route in the running route table:

`ai_assistant.view`, `database.view`, `database.manage`, `metrics.view`, `sessions.view`,
`workflows.view`.

Some are used as frontend gates only; `sessions.view` is checked inside a handler rather than
in middleware (self reads its own sessions, other people's need `sessions.manage`); the
database ones gate a wizard whose endpoints run before any account exists. They are listed
here so a reader auditing the catalog does not go looking for a route that is not there.

## Routes with no permission middleware

32 mutating routes carry no `RequirePermission`, and every one of them is exempt by design:

| Group | Routes | Why |
|---|---|---|
| Session lifecycle | `login`, `login/mfa`, `logout`, `setup`, `session/touch` | There is no principal yet, or the action only ends the caller's own session |
| Self-service | `auth/profile`, `auth/preferences`, `auth/mfa/*`, `auth/sessions/*`, `users/{id}/password` | Acts only on the caller's own account; the target comes from the session, never the body. The account-security ones are described in [Self-service routes](#self-service-routes) |
| Password reset | `auth/password/forgot`, `auth/password/reset` | Runs logged out, gated by a token |
| Externally authenticated | `webhooks/gitops/pr`, `identity-providers/{id}/acs`, the agent transfer endpoints | Authenticated by HMAC signature or a one-shot token |
| Own inbox | `in-app-notifications/*`, `license/notice-ack`, `elevations` create and withdraw | Acts only on rows the caller owns; the last three are also self-service routes |
| Checked in the handler | `compute/console/ticket` | The permission check runs on this request; the WebSocket that follows carries only a ticket |
| First-run | `database/setup/test`, `database/setup/apply`, `auth/setup` | The installer has no account yet. Each needs the one-time setup token in the `X-McHarbor-Setup-Token` header; see [Authentication](./authentication.md) |

Three routes used to fall outside every one of those reasons, and the code disagreed with its
own comments about it. They are now gated:

| Route | Gate |
|---|---|
| `/api/appstore/plugins/*` | `plugins.view` or `plugins.manage` for reads, `plugins.manage` for install / update / configure, `plugins.manage` or `plugins.delete` for uninstall |
| `POST /api/plugins/loader/reload` | `plugins.manage` |
| `POST /api/lifecycle` | `settings.manage` at global scope |

The plugin-store case is worth remembering as a shape: a correctly gated mount existed in the
module's `routes.go` the whole time, and nothing called it. The host mounted an ungated
`Handler.Mount` from `handler.go` instead. Two mount functions for one route set is how the
gated one ends up unused. There is one now.

The **read** routes under `/api/plugins/loader/` remain authenticated-only by design: the
frontend loader fetches the menu, translations, widgets, themes and plugin bundles for every
signed-in user, so gating them behind `plugins.view` would leave a user without that key
facing an unrenderable interface rather than a smaller one. The same reasoning keeps
`GET /api/lifecycle` and `GET /api/lifecycle/recent` open: they back the Docker logging tab
and the dashboard panel.

### Self-service routes

A route that acts on the caller's own account is gated by a session, not by a catalog key, so an
ordinary user needs no grant to use it. `RequireSelfService` adds one rule: **an API key must
carry `account.self.manage`**, so a key minted for a narrow job cannot enrol an MFA device, revoke
sessions or approve its own elevation. The routes it covers:

| Area | Routes |
|---|---|
| Profile | `PUT /api/auth/profile` |
| Step-up | `POST /api/auth/sudo` |
| Second factor | `POST /api/auth/mfa/enroll/begin`, `POST /api/auth/mfa/enroll/confirm`, `DELETE /api/auth/mfa` |
| Sessions | `DELETE /api/auth/sessions/{sessionId}`, `POST /api/auth/sessions/revoke-all` |
| Elevations | `POST /api/elevations`, `DELETE /api/elevations/{id}` |
| Approvals | `POST /api/approvals/{id}/execute`, `POST /api/approvals/{id}/cancel` |
| Licence | `POST /api/license/notice-ack` |

Logout, session touch, preferences, notifications, table views and break-glass are not in the list:
they are harmless for a narrow key, or (break-glass) already refuse an API key outright.

## Adding a permission

Covered in the how-to section: [Contracting permissions](../../how-to/permissions/README.md).

## Related

- [Authentication](./authentication.md): obtaining a credential in the first place
- [Roles endpoints](../endpoints/roles.md): roles, ABAC policies, overrides
- [Users endpoints](../endpoints/users.md): assignments, groups, elevations
