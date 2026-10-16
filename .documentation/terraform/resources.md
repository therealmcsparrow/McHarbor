# Resources and data sources

Every key below is the permission the McHarbor route enforces, read from the route's
`x-mcharbor-permission` in `src/backend/api/openapi.json`. A key with a scope of `environment` or
`stack` can be granted on one environment or stack instead of globally. A key with any other
scope (`global` or `account`) counts only from a role assigned globally, so a role assigned on one
environment never carries it.

## `mcharbor_environment`

A Docker environment: the local socket, a TCP or SSH endpoint, or a remote agent.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `connection_type` | required, forces replacement | `socket`, `tcp`, `ssh` or `agent` |
| `host`, `port` | optional | TCP endpoint |
| `socket_path` | optional | Socket connection |
| `ssh_host`, `ssh_port`, `ssh_user` | optional | SSH connection |
| `ssh_key`, `tls_ca`, `tls_cert`, `tls_key` | optional, sensitive | Stored in state; never read back |
| `is_default` | optional, default `false` | |
| `timezone` | optional, computed | IANA zone for this environment's schedules |
| `agent_token` | computed, sensitive | Agent environments only; returned once, at creation |
| `id`, `created_at` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/environments` | `environments.manage` |
| Read | `GET /api/environments/{id}` | `environments.view` |
| Update | `PUT /api/environments/{id}` | `environments.manage` |
| Delete | `DELETE /api/environments/{id}` | `environments.manage` |

Import: `terraform import mcharbor_environment.edge <environment id>`.

## `mcharbor_stack`

A Compose stack on one environment.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | Renaming updates in place |
| `environment_id` | required, forces replacement | |
| `compose` | required, sensitive | The compose file content, stored in state. Read back with reveal, which needs `stacks.manage` on the stack; without it (or where McHarbor withholds its own keys) the read carries `••••••••` and the next plan shows a change. Run `terraform apply -refresh-only` after granting the key, or keep McHarbor's keys out of managed stacks |
| `description` | optional, default `""` | |
| `env_vars` | optional map, sensitive | Stored in state; a masked secret read keeps the configured value |
| `auto_start` | optional, default `true` | Read only at creation |
| `secret_file_mode` | optional, default `false` | Mount secret references as files |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/stacks` | `stacks.manage` |
| Read | `GET /api/stacks/{name}`, `…/compose`, `…/env-vars` | `stacks.view` |
| Update | `PUT /api/stacks/{name}` | `stacks.manage` |
| Delete | `DELETE /api/stacks/{name}?env=<environment_id>` | `stacks.delete` |

Import: `terraform import mcharbor_stack.web <environment_id>/<name>`.

## `mcharbor_role`

A named set of permission keys.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `description` | optional, default `""` | |
| `permissions` | required set | Keys from the `mcharbor_permission_catalog` data source |
| `is_system` | computed | System roles cannot be changed or deleted |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/roles` | `roles.manage` |
| Read | `GET /api/roles/{id}` | `roles.view` |
| Update | `PUT /api/roles/{id}` | `roles.manage` |
| Delete | `DELETE /api/roles/{id}` | `roles.manage` |

Import: `terraform import mcharbor_role.ops <role id>`.

## `mcharbor_role_assignment`

Grants a role to a user, globally or on one environment or stack. Every attribute forces
replacement: an assignment is created or removed, never edited. On an environment or stack the
role grants only its environment-, stack- and owner-scoped keys; global and account keys need a
global assignment.

| Attribute | Kind | Notes |
|---|---|---|
| `user_id` | required | |
| `role_id` | required | |
| `environment_id` | optional | Omit for a global grant |
| `stack_name` | optional | Requires `environment_id` |
| `id` | computed | The assignment id |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/users/{id}/roles` | `users.manage` |
| Read | `GET /api/users/{id}/roles` | `users.view` |
| Delete | `DELETE /api/users/{id}/roles/{assignmentId}` | `users.manage` |

The create route does not return the new assignment's id. The provider lists the user's assignments
before and after, and takes the new one that matches.

Import: `terraform import mcharbor_role_assignment.x <user_id>/<assignment_id>`.

## `mcharbor_api_key`

An API key owned by the provider's own user.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `role_ids` | required set | The caller must hold everything the roles grant |
| `scopes` | optional set, default empty | Permission keys that narrow the roles further |
| `environment_ids` | optional set, default empty | Empty means every environment the roles allow |
| `expires_at` | optional, forces replacement | RFC 3339; McHarbor cannot change it after creation |
| `key` | computed, sensitive | Returned once, at creation; null after import |
| `key_prefix`, `user_id`, `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/api-keys` | `api_keys.manage` |
| Read | `GET /api/api-keys/{id}` | `api_keys.view` or `api_keys.manage` |
| Update | `PATCH /api/api-keys/{id}` | `api_keys.manage` |
| Delete | `DELETE /api/api-keys/{id}` | `api_keys.manage` |

Destroy revokes the key. McHarbor keeps the revoked row for audit, and the provider treats a revoked
key as gone.

Import: `terraform import mcharbor_api_key.ci <api key id>`.

## `mcharbor_secret`

A secret in the McHarbor vault. **Its value never enters Terraform state or plan output.** The
value comes from exactly one of two places:

- **Option A: write-only value.** `value_wo` is a write-only argument (Terraform 1.11 or later).
  It is sent on create, and again only when `value_wo_version` changes. That update goes through
  McHarbor's rotate endpoint, so the new value becomes a new secret version with the usual audit
  entry, and is auto-applied when `auto_apply` is on. Terraform cannot diff a write-only value,
  so **bumping `value_wo_version` is how you rotate.**
- **Option B: the value never passes through Terraform.** Either `generate = true` (optionally
  `generate_length`): McHarbor mints the value with `crypto/rand`, and the provider discards the
  one-time value in the response. Or adopt a secret whose value was set in McHarbor:
  `terraform import`, after which Terraform manages only the metadata. With `generate = true`,
  bumping `value_wo_version` rotates to a freshly generated value.

Setting both `value_wo` and `generate` is rejected at plan time, and so is a new secret with
neither.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required, forces replacement | Uppercase identifier, referenced as `${secret:NAME}` |
| `description` | optional, default `""` | |
| `kind` | optional, default `env` | `env`, `password`, `token`, `cert`, `private_key`, `kubeconfig`, `ssh_key`, `json` |
| `group` | optional | Folder label in the vault listing |
| `scope` | optional, default `global`, forces replacement | `global`, `environment`, `stack`, `application` |
| `environment_id` | optional, forces replacement | Required for `environment` and `stack` scope |
| `stack_name` | optional, forces replacement | Required for `stack` scope |
| `auto_apply` | optional, default `false` | Propagate a rotation to every referencing container and stack |
| `value_wo` | optional, **write-only**, sensitive | Option A. Never stored |
| `value_wo_version` | optional | Bump to rotate |
| `generate` | optional, default `false`, forces replacement | Option B |
| `generate_length` | optional, forces replacement | 16 to 128 bytes |
| `version` | computed | Increases on every rotation |
| `generated` | computed | |
| `id` | computed | |

**What lands in state:** the id, name, description, kind, group, scope, environment id, stack
name, `auto_apply`, `version`, `generated`, `generate`, `generate_length` and
`value_wo_version`. Never the value. Reading a value needs `secrets.reveal`, which the provider
never calls.

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/secrets` | `secrets.manage`; a `global` or `application` scope also needs `secrets.manage_global` |
| Read | `GET /api/secrets/{id}` | `secrets.view` |
| Update metadata | `PATCH /api/secrets/{id}` | `secrets.manage` |
| Rotate | `PUT /api/secrets/{id}` | `secrets.manage` |
| Delete | `DELETE /api/secrets/{id}` | `secrets.delete` |

Import: `terraform import mcharbor_secret.db <secret id>`. After import, `value_wo_version`,
`generate` and `generate_length` are unknown to McHarbor, so set them in configuration only if you
want Terraform to rotate the secret from then on.

## `mcharbor_schedule`

A scheduled container action. McHarbor validates the cron expression and rejects one it cannot
parse.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `description` | optional, default `""` | |
| `cron` | required | Five-field cron expression |
| `action` | required | `start`, `stop`, `restart` or `exec` |
| `target` | required | Container name or id |
| `environment_id` | required | |
| `enabled` | optional, default `true` | |
| `next_run_at` | computed | |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/schedules` | `schedules.manage` |
| Read | `GET /api/schedules/{id}` | `schedules.view` |
| Update | `PUT /api/schedules/{id}` | `schedules.manage` |
| Delete | `DELETE /api/schedules/{id}` | `schedules.delete` |

Import: `terraform import mcharbor_schedule.nightly <schedule id>`.

## `mcharbor_update_policy`

Which containers and images to check for updates, when, and how.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `container_match` | optional, default `""` | Glob or regex over container names |
| `image_match` | optional, default `""` | Glob or regex over image names |
| `schedule` | required | Five-field cron expression |
| `strategy` | optional, default `latest` | `latest`, `semver` or `digest` |
| `auto_restart` | optional, default `false` | |
| `enabled` | optional, default `true` | |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/updates` | `updates.manage` |
| Read | `GET /api/updates/{id}` | `updates.view` |
| Update | `PUT /api/updates/{id}` | `updates.manage` |
| Delete | `DELETE /api/updates/{id}` | `updates.manage` |

Import: `terraform import mcharbor_update_policy.web <policy id>`.

## `mcharbor_alert_rule`

A condition from an alert source, where it applies, and who is notified. The available sources
and conditions are listed by `GET /api/alerts/conditions`. McHarbor fills unset condition fields
from the condition's declaration.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `source_type` | required | For example `container`, `host`, `environment` |
| `condition_key` | required | For example `cpu`, `memory`, `down`, `restart_loop` |
| `condition` | required object | `operator`, `threshold`, `unit`, `for_seconds`, `window_seconds`, `params`; each optional, defaulted by McHarbor |
| `severity` | optional, computed | `info`, `warning`, `critical` |
| `target` | optional, computed | Name or id pattern, or `*` |
| `environment_ids` | optional set, default empty | Empty means every active environment |
| `destinations` | optional list, default empty | `channel_id`, `min_severity` |
| `send_in_app` | optional, default `false` | A rule needs at least one destination or `send_in_app = true` |
| `cooldown_seconds` | optional, computed | |
| `notify_on_recovery` | optional, computed | |
| `message_template` | optional, computed | |
| `enabled` | optional, default `true` | |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/alerts` | `alerts.manage` |
| Read | `GET /api/alerts` (paged list; there is no single-rule read) | `alerts.view` |
| Update | `PUT /api/alerts/{id}` | `alerts.manage` |
| Delete | `DELETE /api/alerts/{id}` | `alerts.delete` |

Import: `terraform import mcharbor_alert_rule.cpu <rule id>`.

## `mcharbor_storage_location`

A backup destination. The location types available are those the installed storage plugins
provide; McHarbor refuses any other. **Credentials are write-only and never land in state.**

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `location_type` | required, forces replacement | `local`, or a plugin-provided type such as `onedrive_business`, `sftp`, `s3`, `smb` |
| `enabled` | optional, default `true` | |
| `host`, `port`, `base_path`, `region`, `bucket`, `endpoint`, `tenant_id`, `site_url`, `drive_id`, `share_name`, `domain`, `username`, `auth_method`, `tls_mode` | optional | Omit rather than set to `""`; McHarbor does not echo empty values |
| `passive_mode` | optional, computed | |
| `password_wo`, `private_key_wo`, `passphrase_wo`, `ca_certificate_wo`, `client_certificate_wo`, `client_key_wo`, `access_key_id_wo`, `secret_access_key_wo`, `client_id_wo`, `client_secret_wo`, `refresh_token_wo`, `token_wo` | optional, **write-only**, sensitive | Sent on create, and again when `credentials_version` changes |
| `config_wo` | optional map, **write-only**, sensitive | Plugin-owned form fields, stored encrypted |
| `credentials_version` | optional | Bump to send the credentials again |
| `id` | computed | |

**What lands in state:** every attribute except the `*_wo` credentials and `config_wo`.

The built-in local location (`default-local-backup`) cannot be disabled, deleted or changed to
another type, because backups fall back to it. Any other location, including an extra `local` one,
can be managed and destroyed normally.

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/storage-locations` | `settings.manage` or `storage_locations.manage` |
| Read | `GET /api/storage-locations/{id}` | `settings.view` or `storage_locations.view` |
| Update | `PUT /api/storage-locations/{id}` | `settings.manage` or `storage_locations.manage` |
| Delete | `DELETE /api/storage-locations/{id}` | `settings.manage` or `storage_locations.delete` |

Import: `terraform import mcharbor_storage_location.offsite <location id>`. Imported credentials are
not read back; set `credentials_version` and the `*_wo` values to have Terraform send them.

## `mcharbor_backup_plan`

A container backup plan. Per-mount selections are managed in the UI and left untouched by
Terraform.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `environment_id` | required, forces replacement | |
| `container_id` | required, forces replacement | Kept stable when Docker recreates the container under the same name |
| `storage_location_ids` | required set | |
| `include_config`, `include_logs`, `include_filesystem`, `include_image` | optional, computed | |
| `selected_mounts` | optional set, default empty | |
| `log_tail_lines`, `retention_count`, `retention_days` | optional, computed | |
| `cron` | optional | Omit for manual runs only |
| `enabled` | optional, default `true` | |
| `consistency_mode` | optional, computed | `pause`, `snapshot` or `live` |
| `snapshot_config_id` | optional | |
| `container_name`, `next_run_at`, `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/container-backups?env=…` | `backups.manage` |
| Read | `GET /api/container-backups?env=…` (list; there is no single-plan read) | `backups.view` |
| Update | `PUT /api/container-backups/{planId}` | `backups.manage` |
| Delete | `DELETE /api/container-backups/{planId}` | `backups.delete` |

Import: `terraform import mcharbor_backup_plan.web <environment_id>/<plan_id>`.

## `mcharbor_workflow`

A workflow. The graph is JSON, not HCL: export the canvas from the workflow editor, keep it in a
file beside the configuration, and pass it with `file("workflow.json")`. JSON that differs only in
formatting or key order is not a change, so reformatting the file never plans an update.

| Attribute | Kind | Notes |
|---|---|---|
| `name` | required | |
| `canvas_json` | required | The workflow graph. Rejected at plan time when it is not valid JSON |
| `description` | optional, default `""` | |
| `status` | optional, default `draft` | `draft` or `active`. Only an active workflow's triggers fire |
| `variables_json` | optional, default `{}` | Workflow variables as a JSON object |
| `id` | computed | |

| Operation | Route | Permission |
|---|---|---|
| Create | `POST /api/workflows`, then `PUT /api/workflows/{id}` with the graph | `workflows.manage` |
| Read | `GET /api/workflows/{id}` | `workflows.view` |
| Update | `PUT /api/workflows/{id}` | `workflows.manage` |
| Delete | `DELETE /api/workflows/{id}` | `workflows.delete` |

Import: `terraform import mcharbor_workflow.nightly <workflow_id>`. After import, `canvas_json`
holds the server's compact form; the next plan with your file shows no change if the documents
are equal.

## `mcharbor_plugin`

A plugin installed from the configured catalogs at an exact version. Terraform states the
version; McHarbor installs it, moves to it, or keeps it, using the same uninstall-and-install
cycle as **Update** in the Plugin Store, so the database row, install directory, manifest and
binary stay in agreement and role grants survive. The version must be one a catalog serves;
list them with `mcharbor_plugin_catalog`.

A plugin's own settings are edited on the plugin's settings screen and are not managed here.

| Attribute | Kind | Notes |
|---|---|---|
| `slug` | required, forces replacement | |
| `version` | required | Exact catalog version, for example `2026.09.02` |
| `enabled` | optional, default `true` | |
| `purge_on_destroy` | optional, default `false` | When `true`, destroying the resource also deletes the plugin's data and tables. Left `false`, a later reinstall finds its data again |
| `loaded` | computed | Whether the running McHarbor has the plugin loaded |
| `signed` | computed | Whether the installed package carried a trusted signature |
| `id` | computed | The slug |

| Operation | Route | Permission |
|---|---|---|
| Create, Update | `POST /api/appstore/plugins/{slug}/reconcile` with `version` and `enabled` | `plugins.manage` |
| Read | `GET /api/appstore/plugins/{slug}` | `plugins.view` |
| Delete | `POST /api/appstore/plugins/uninstall` with `purge_data` | `plugins.manage` or `plugins.delete` |

Import: `terraform import mcharbor_plugin.ntfy comm-ntfy`. `purge_on_destroy` is not stored by
McHarbor and imports as `false`.

## Data sources

| Data source | Route | Permission | Returns |
|---|---|---|---|
| `mcharbor_environments` | `GET /api/environments` | `environments.view` | `environments[]`: `id`, `name`, `connection_type`, `is_default`, `agent_status` |
| `mcharbor_roles` | `GET /api/roles` | `roles.view` | `roles[]`: `id`, `name`, `description`, `permissions`, `is_system` |
| `mcharbor_permission_catalog` | `GET /api/auth/permissions/catalog` | `roles.view` or `permissions.view` | `permissions[]`: `key`, `category`, `action`, `description`, `scope` |
| `mcharbor_secret` | `GET /api/secrets?search=NAME` | `secrets.view` | One secret's metadata by `name` (and optionally `scope`, `environment_id`, `stack_name`): `id`, `kind`, `description`, `group`, `version`, `generated`, `auto_apply`. Never the value |
| `mcharbor_containers` | `GET /api/containers?env=…&all=true` | `containers.view` | `containers[]`: `id`, `name`, `image`, `state`, `status`, `stack`, running and stopped. Optional `environment_id`; the default environment when omitted |
| `mcharbor_images` | `GET /api/images?env=…` | `images.view` | `images[]`: `id`, `tags`, `size` (bytes). Optional `environment_id` |
| `mcharbor_app_catalog` | `GET /api/app-store` | `store_apps.view` | `apps[]`: `id`, `slug`, `name`, `category`, `image`, `description`. Optional `category` and `search`; at most 1000 apps |
| `mcharbor_plugin_catalog` | `GET /api/appstore/plugins/catalog` | `plugins.view` | `plugins[]`: `slug`, `name`, `latest_version`, `versions`, `description` |

## What goes wrong

| Symptom | Cause |
|---|---|
| `… (HTTP 403) … The API key needs the permission X` | The API key's roles lack `X`. Grant it, or use a key that has it. |
| `x509: certificate signed by unknown authority` | McHarbor's default self-signed certificate. Trust its CA, or set `insecure_skip_verify = true`. |
| A stack fails to create on a new agent environment | The agent is not connected yet. Create the stack with `auto_start = false`. |
| A role update is refused | The role is a system role (`is_system = true`). |
| `key` is null after import | Expected: McHarbor never shows an API key's secret after creation. |
| `Missing secret value` on a new `mcharbor_secret` | Set `value_wo` or `generate = true`, or import an existing secret. |
| A secret's value did not change after editing `value_wo` | Write-only values cannot be diffed. Bump `value_wo_version`. |
| `Invalid request body` creating a storage location | No installed storage plugin provides that `location_type`. |
| `terraform destroy` fails on a storage location | It is the built-in `default-local-backup` location, which McHarbor never deletes. Remove it from state with `terraform state rm` instead. |
| `McHarbor refused appstorePluginsReconcile (HTTP 409)` naming the slug and version | No configured catalog serves that version. Pick one from `mcharbor_plugin_catalog`. |
| A plugin resource plans no change but the plugin is not loaded (`loaded = false`) | It is installed but disabled, or failed to load. Check **Plugins** in McHarbor. |
| `Invalid JSON` on `canvas_json` | The file is not valid JSON. Re-export it from the workflow editor. |
