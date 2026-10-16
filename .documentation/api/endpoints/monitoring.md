# Dashboard, audit, alerts and notifications

49 endpoints: the dashboard's numbers, the widget catalog behind it, the audit log and its
integrity tooling, alert rules and their history, notification channels, and the per-user
inbox.

| Key | Covers |
|---|---|
| `dashboard.view` | Dashboard stats and host metric history |
| `widgets.view` or `store_widgets.view` | Widget definitions |
| `widgets.manage` or `store_widgets.manage` | Install, update and uninstall widgets |
| `audit.view` | Read the log, action catalog, coverage, verification, denials, the AI trail, retention |
| `audit.export` | Export entries and the coverage report |
| `alerts.view` / `.manage` / `.delete` | Alert rules and their history |
| `communications.view` / `.manage` | Notification channels and their adapter capabilities |
| `in_app_notifications.view` | The caller's own inbox |
| `settings.manage` | Purge the audit log; add, edit, test and remove log forwarders |
| `audit.view` and `settings.manage` | List log forwarders, their adapters and delivery history; change audit retention |

## Things worth knowing first

**The audit log can be made tamper-evident.** With `audit.hash_chain.enabled` on, each entry
chains to the previous one and `GET /api/audit/verify` re-walks the chain. WORM mode
(`audit.worm`) blocks deletion outright unless `audit.worm_delete_allowed` says otherwise.
The clock is part of the same story: an audit trail is only as trustworthy as the clock that
stamped it. A background check compares the system clock with the reference in
`audit.clock.reference_url` every six hours, writes a `clock.drift_check` audit entry, and
posts an in-app broadcast when the drift exceeds the threshold. There is no route to read the
result; the audit log and the notification are where drift shows.

**Coverage answers "what is not being audited".**
`GET /api/audit/coverage` reports which actions the catalog knows about and which are
actually emitted; the export needs `audit.export` while the read needs only `audit.view`.

**Exports are signed.** `GET /api/audit/export` returns CSV or JSON with checksum metadata,
so an exported file can be shown to be the one that left the system.

**Alerts and channels are separate things.** A channel is a destination: Slack, Telegram,
SMTP, a plugin. An alert rule is a condition plus a list of destinations.
`GET /api/alerts/conditions` assembles the condition catalog at request time rather than
caching it, so a plugin installed a minute ago contributes its conditions without a restart.

**`POST /api/alerts/{id}/test` fires through the real destinations**, severity filtering
included. A test that took a shortcut would prove the channel works while saying nothing
about whether this rule would ever reach it.

**`GET /api/communication-channels/capabilities` reports configured transports**, not
installed ones. The installed-plugin surface is `/api/plugins/loader/notification-adapters`;
the two are orthogonal.

**The in-app inbox is per-user and mostly ungated.** Marking read and deleting act only on the
caller's own rows, so they carry no permission; listing and the unread count do carry
`in_app_notifications.view`.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/alerts` | `alerts.view` | Returns a paginated list of alert rules | — |
| `POST` | `/api/alerts` | `alerts.manage` | Creates a new alert rule | — |
| `GET` | `/api/alerts/conditions` | `alerts.view` | Returns every condition that can be alerted on | — |
| `GET` | `/api/alerts/template-reference` | `alerts.view` | Returns the template placeholders and the chosen condition's `{{ctx.*}}` keys | `?source=`, `?condition=` |
| `POST` | `/api/alerts/template-preview` | `alerts.view` | Renders a draft template with sample values; stores and sends nothing | — |
| `GET` | `/api/alerts/events` | `alerts.view` | Returns a paginated slice of alert history | `?alertId=`, `?severity=`, `?state=` |
| `DELETE` | `/api/alerts/{id}` | `alerts.delete` | Removes an alert rule | `{id}` |
| `PUT` | `/api/alerts/{id}` | `alerts.manage` | Updates an alert rule | `{id}` |
| `POST` | `/api/alerts/{id}/test` | `alerts.manage` | Fires a sample alert through a rule's real destinations | `{id}` |
| `DELETE` | `/api/audit` | `settings.manage` | Deletes audit entries older than the retention cutoff, and records the purge | `?vacuum=` |
| `GET` | `/api/audit` | `audit.view` | Deletes audit entries older than the retention cutoff, and records the purge | `?expand=` |
| `GET` | `/api/audit/actions` | `audit.view` | Returns the catalog of auditable actions, optionally for one module | `?module=` |
| `GET` | `/api/audit/ai-actions` | `audit.view` | Pages AI actions grouped by action id, each with its preview, consent and execution steps and redacted payloads | `?actor=`, `?from=`, `?to=` |
| `GET` | `/api/audit/ai-tools` | `audit.view` | Pages `ai.tool.*` events with their details redacted | `?actor=`, `?from=`, `?to=` |
| `GET` | `/api/audit/coverage` | `audit.view` | Reports which auditable actions are actually being emitted | — |
| `GET` | `/api/audit/coverage/export` | `audit.export` | Coverage Export | — |
| `GET` | `/api/audit/denials` | `audit.view` | Pages RBAC and ABAC denials with actor, permission, route and environment parsed out | `?actor=`, `?permission=`, `?environment=`, `?from=`, `?to=` |
| `GET` | `/api/audit/export` | `audit.export` | Returns a signed CSV or JSON audit export with checksum metadata | `?format=` |
| `GET` | `/api/audit/logging` | `audit.view` and `settings.manage` | Lists every configured log forwarder with its config; secret fields come back only as a stored/not-stored flag | — |
| `POST` | `/api/audit/logging` | `settings.manage` | Adds one forwarder instance | — |
| `GET` | `/api/audit/logging/adapters` | `audit.view` and `settings.manage` | Lists the installed forwarder adapter types | — |
| `DELETE` | `/api/audit/logging/{id}` | `settings.manage` | Removes one forwarder instance; the plugin stays installed | `{id}` |
| `PUT` | `/api/audit/logging/{id}` | `settings.manage` | Rewrites one forwarder instance | `{id}` |
| `GET` | `/api/audit/logging/{id}/history` | `audit.view` and `settings.manage` | Returns this forwarder's recent delivery attempts | `{id}` |
| `POST` | `/api/audit/logging/{id}/test` | `settings.manage` | Asks the adapter to hit the wire; a transport failure is a `400`, not a `500` | `{id}` |
| `GET` | `/api/audit/plugins/summary` | `audit.view` | Summarises audit activity per installed plugin | — |
| `GET` | `/api/audit/retention` | `audit.view` | Returns the per-severity audit retention policy | — |
| `PUT` | `/api/audit/retention` | `audit.view` and `settings.manage` | Stores per-severity retention overrides; the hourly pruner picks them up | — |
| `GET` | `/api/audit/verify` | `audit.view` | Re-walks the audit hash chain and reports whether it is intact | — |
| `GET` | `/api/communication-channels` | `communications.view` | Returns all communication channels | — |
| `POST` | `/api/communication-channels` | `communications.manage` | Creates a new communication channel | — |
| `GET` | `/api/communication-channels/adapters` | `communications.view` | Returns what each registered channel type can do, chiefly whether it can carry a file | — |
| `GET` | `/api/communication-channels/capabilities` | — | Returns configured notification transport capabilities | — |
| `DELETE` | `/api/communication-channels/{id}` | `communications.manage` | Removes a communication channel | `{id}` |
| `GET` | `/api/communication-channels/{id}` | `communications.view` | Returns a single communication channel | `{id}` |
| `PUT` | `/api/communication-channels/{id}` | `communications.manage` | Updates an existing communication channel | `{id}` |
| `POST` | `/api/communication-channels/{id}/default` | `communications.manage` | Sets a communication channel as the default | `{id}` |
| `POST` | `/api/communication-channels/{id}/test` | `communications.manage` | Sends a test notification via the specified channel and returns the per-step report | `{id}` |
| `GET` | `/api/dashboard/metrics` | `dashboard.view` | Returns historical host metrics time series | `?limit=` |
| `GET` | `/api/dashboard/stats` | `dashboard.view` | Returns overall Docker resource counts | — |
| `GET` | `/api/dashboard/summary` | `dashboard.view` | Returns estate-wide counts for the dashboard page head: environments, containers, stacks, and how many hosts did not answer. Each figure is computed only over the environments the caller may read, and a resource they may not read is **omitted** rather than returned as zero | — |
| `GET` | `/api/in-app-notifications` | `in_app_notifications.view` | Returns a paginated list of in-app notifications for the current user | `?q=`, `?mode=` (contains, exact, regex), `?level=` (comma list), `?status=` (read, unread) |
| `GET` | `/api/in-app-notifications/counts` | `in_app_notifications.view` | Returns the caller's totals (all, unread, and by level) for the filter chips | — |
| `POST` | `/api/in-app-notifications/read-all` | — | Marks all notifications as read for the current user | — |
| `GET` | `/api/in-app-notifications/unread-count` | `in_app_notifications.view` | Returns the unread notification count for the current user | — |
| `DELETE` | `/api/in-app-notifications/{id}` | — | Hides a single notification from the current user's inbox | `{id}` |
| `POST` | `/api/in-app-notifications/{id}/read` | — | Marks a single notification as read for the current user | `{id}` |
| `GET` | `/api/widgets/definitions` | `store_widgets.view` or `widgets.view` | Returns all widget definitions with translations | — |
| `PUT` | `/api/widgets/{key}` | `store_widgets.manage` or `widgets.manage` | Updates widget availability | `{key}` |
| `GET` | `/api/widgets/catalog` | `store_widgets.view` or `widgets.view` | Lists signed widget entries from the local plugin catalogs, each with `signed` and `installed` | — |
| `POST` | `/api/widgets/catalog/{key}/install` | `dashboard_widgets.install` | Verifies and installs a downloaded widget; `409 err.widget.downgrade` for an older version | `{key}` |
| `DELETE` | `/api/widgets/{key}` | `dashboard_widgets.install` | Removes a downloaded widget | `{key}` |
| `GET` | `/api/widgets/{key}/bundle` | `store_widgets.view` or `widgets.view` | Serves an installed widget's bundle after re-verifying it | `{key}` |


## Request bodies

**`POST` `/api/alerts`**: `name`, `severity`, `sourceType`, `conditionKey`, `condition`, `target`, `environmentIds`, `destinations`, `sendInApp`, `cooldownSeconds`, `notifyOnRecovery`?, `messageTemplate`

**`POST` `/api/alerts/template-preview`**: `template`, `name`, `severity`, `sourceType`, `conditionKey`, `condition`, `state` (`firing` or `resolved`). Returns `message` and `unknown`, the placeholders left as written. Templates over 4000 characters are `400`.

**`PUT` `/api/alerts/{id}`**: `name`?, `severity`?, `sourceType`?, `conditionKey`?, `condition`?, `target`?, `environmentIds`?, `destinations`?, `sendInApp`?, `cooldownSeconds`?, `notifyOnRecovery`?, `messageTemplate`?, `enabled`?

**`POST` `/api/audit/logging`**: `name`, `forwarderType`, `enabled`, `config`. `forwarderType` is read on create only.

**`PUT` `/api/audit/logging/{id}`**: `name`, `enabled`, `config`. The type is fixed at create; a `forwarderType` sent here is ignored.

**`PUT` `/api/audit/retention`**: `overrides`

**`POST` `/api/communication-channels`**: `name`, `channelType`, `method`, `isDefault`, `webhookUrl`, `serverUrl`, `token`, `topic`, `chatId`, `phoneNumberId`, `recipientPhone`, `senderNumber`, `recipients`, `username`, `password`, `priority`, `config`

**`PUT` `/api/communication-channels/{id}`**: `name`?, `method`?, `isDefault`?, `enabled`?, `webhookUrl`?, `serverUrl`?, `token`?, `topic`?, `chatId`?, `phoneNumberId`?, `recipientPhone`?, `senderNumber`?, `recipients`?, `username`?, `password`?, `priority`?, `config`

**`PUT` `/api/widgets/{key}`**: `enabled`?

## Related

- [Host](./host.md): the metrics behind the dashboard
- [Settings reference](../settings/reference.md): audit and retention keys
- [Audit screen](../../screens/audit/README.md)
