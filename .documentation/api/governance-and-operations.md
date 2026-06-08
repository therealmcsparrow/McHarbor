# Governance and Operations

This file covers user and RBAC management, settings, notification routing,
auditing, scanners, plugins, update policies, and dashboard-oriented endpoints.

## Users, Roles, and Groups

### Users

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/users/` | Lists users. |
| GET | `/api/users/{id}` | Returns one user. |
| PUT | `/api/users/{id}` | Updates a user. |
| DELETE | `/api/users/{id}` | Deletes a user. |
| PUT | `/api/users/{id}/password` | Changes a user's password. |
| GET | `/api/users/{id}/groups` | Lists group memberships for a user. |
| GET | `/api/users/{id}/roles` | Lists role assignments for a user. |
| POST | `/api/users/{id}/roles` | Assigns a role to a user. |
| DELETE | `/api/users/{id}/roles/{assignmentId}` | Removes a role assignment from a user. |

### Roles

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/roles/` | Lists roles. |
| GET | `/api/roles/permissions` | Lists assignable permissions. |
| GET | `/api/roles/{id}` | Returns one role. |
| POST | `/api/roles/` | Creates a role. |
| PUT | `/api/roles/{id}` | Updates a role. |
| DELETE | `/api/roles/{id}` | Deletes a role. |

### Groups

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/groups/` | Lists groups. |
| POST | `/api/groups/` | Creates a group. |
| GET | `/api/groups/{id}` | Returns one group. |
| PUT | `/api/groups/{id}` | Updates a group. |
| DELETE | `/api/groups/{id}` | Deletes a group. |
| POST | `/api/groups/{id}/members` | Adds a user to a group. |
| DELETE | `/api/groups/{id}/members/{userId}` | Removes a user from a group. |
| POST | `/api/groups/{id}/roles` | Assigns a role to a group. |
| DELETE | `/api/groups/{id}/roles/{assignmentId}` | Removes a role assignment from a group. |

## Settings and Integrations

### Settings

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/settings/` | Lists settings. |
| PUT | `/api/settings/` | Bulk-updates settings. |
| GET | `/api/settings/agent` | Returns agent settings. |
| PUT | `/api/settings/agent` | Updates agent settings. |
| GET | `/api/settings/scanners` | Returns scanner settings. |
| PUT | `/api/settings/scanners` | Updates scanner settings. |
| GET | `/api/settings/retention` | Returns retention settings. |
| PUT | `/api/settings/retention` | Updates retention settings. |
| GET | `/api/settings/tls` | Returns TLS settings. |
| PUT | `/api/settings/tls` | Updates TLS settings. |
| POST | `/api/settings/backup-key/generate` | Generates a 32-byte backup encryption key for one-time copy. The key is returned once and must be stored as the `mcharbor_backup_key` Docker secret. |
| GET | `/api/settings/{key}` | Returns one setting by key. |
| PUT | `/api/settings/{key}` | Updates one setting by key. |

### Registry Integrations

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/registries/` | Lists registry definitions. |
| POST | `/api/registries/` | Creates a registry definition. |
| GET | `/api/registries/{id}` | Returns one registry. |
| PUT | `/api/registries/{id}` | Updates a registry. |
| DELETE | `/api/registries/{id}` | Deletes a registry. |
| POST | `/api/registries/{id}/test` | Tests registry connectivity. |

### Email Servers

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/email-servers/` | Lists email servers. |
| POST | `/api/email-servers/` | Creates an email server. |
| GET | `/api/email-servers/{id}` | Returns one email server. |
| PUT | `/api/email-servers/{id}` | Updates an email server. |
| DELETE | `/api/email-servers/{id}` | Deletes an email server. |
| POST | `/api/email-servers/{id}/default` | Sets the default email server. |
| POST | `/api/email-servers/{id}/test` | Tests the email server. |

### Communication Channels

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/communication-channels/capabilities` | Lists supported channel capabilities. |
| GET | `/api/communication-channels/` | Lists communication channels. |
| POST | `/api/communication-channels/` | Creates a communication channel. |
| GET | `/api/communication-channels/{id}` | Returns one communication channel. |
| PUT | `/api/communication-channels/{id}` | Updates a communication channel. |
| DELETE | `/api/communication-channels/{id}` | Deletes a communication channel. |
| POST | `/api/communication-channels/{id}/default` | Sets the default communication channel. |
| POST | `/api/communication-channels/{id}/test` | Tests a communication channel. |

### Storage Locations

Storage locations define reusable external destinations for backups, exports,
and storage workflows. Secrets are encrypted at rest and are never returned by
read endpoints.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/storage-locations/` | Lists configured storage locations. |
| POST | `/api/storage-locations/` | Creates a storage location. |
| GET | `/api/storage-locations/{id}` | Returns one storage location without secret values. |
| PUT | `/api/storage-locations/{id}` | Updates a storage location. Empty secret fields keep the existing encrypted value. |
| DELETE | `/api/storage-locations/{id}` | Deletes a storage location. |
| POST | `/api/storage-locations/{id}/oauth/authorize` | Starts Google Drive, OneDrive, or SharePoint consent. |
| GET | `/api/storage-locations/oauth/callback` | Completes provider consent and stores returned tokens encrypted. |

Supported `locationType` values:

| Type | Notes |
| --- | --- |
| `ftp` | Plain FTP. Uses username/password and normally TCP 21 plus dynamic data ports. |
| `ftps` | FTP over TLS. Uses `tlsMode` as `explicit` on TCP 21 or `implicit` on TCP 990. Optional CA/client certificate fields are supported. |
| `sftp` | SSH File Transfer Protocol. Uses `authMethod` as `password`, `private_key`, or `password_private_key`; default port is TCP 22. |
| `samba` | SMB/CIFS share using host, share, optional domain, username, and password. |
| `aws` | Amazon S3 or S3-compatible storage using bucket, region, endpoint, and access keys. |
| `google_drive` | Google Drive OAuth consent using client ID/secret and optional drive ID. |
| `onedrive_personal` | Personal Microsoft OneDrive consent using client ID/secret and optional drive ID. |
| `onedrive_business` | Microsoft 365 OneDrive consent using tenant ID, client ID/secret, and optional drive ID. |
| `sharepoint` | SharePoint document library consent using tenant ID, site URL, client ID/secret, and optional drive ID. |

FTP/FTPS/SFTP transfer fields:

| Field | Applies to | Purpose |
| --- | --- | --- |
| `authMethod` | SFTP | `password`, `private_key`, or `password_private_key`. |
| `tlsMode` | FTPS | `explicit` or `implicit`. |
| `passiveMode` | FTP/FTPS | Enables passive mode. Useful behind NAT, but the server still needs passive data ports open. |
| `privateKey` | SFTP | Write-only OpenSSH private key. |
| `passphrase` | SFTP | Write-only private-key passphrase. |
| `caCertificate` | FTPS | Write-only PEM CA certificate for validating the server certificate. |
| `clientCertificate` | FTPS | Write-only PEM client certificate for mutual TLS. |
| `clientKey` | FTPS | Write-only PEM private key for the client certificate. |

## Notifications, Alerts, Activity, and Audit

### Notifications

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/notifications/` | Lists notification rules or routes. |
| POST | `/api/notifications/` | Creates a notification rule. |
| GET | `/api/notifications/configured-types` | Lists configured notification types. |
| GET | `/api/notifications/{id}` | Returns one notification rule. |
| PUT | `/api/notifications/{id}` | Updates a notification rule. |
| DELETE | `/api/notifications/{id}` | Deletes a notification rule. |
| POST | `/api/notifications/{id}/test` | Sends a test notification. |

### In-App Notifications

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/in-app-notifications/` | Lists in-app notifications. |
| GET | `/api/in-app-notifications/unread-count` | Returns unread count. |
| POST | `/api/in-app-notifications/read-all` | Marks all in-app notifications as read. |
| POST | `/api/in-app-notifications/{id}/read` | Marks one in-app notification as read. |
| DELETE | `/api/in-app-notifications/{id}` | Deletes an in-app notification. |

### Webhooks

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/webhooks/` | Lists global outbound webhooks. |
| POST | `/api/webhooks/` | Creates a webhook. |
| GET | `/api/webhooks/{id}` | Returns one webhook. |
| PUT | `/api/webhooks/{id}` | Updates a webhook. |
| DELETE | `/api/webhooks/{id}` | Deletes a webhook. |
| POST | `/api/webhooks/{id}/test` | Tests a webhook. |
| GET | `/api/webhooks/{id}/deliveries` | Lists delivery attempts. |

### Alerts

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/alerts/` | Lists alert rules. |
| POST | `/api/alerts/` | Creates an alert rule. |
| PUT | `/api/alerts/{id}` | Updates an alert rule. |
| DELETE | `/api/alerts/{id}` | Deletes an alert rule. |

### Activity and Audit

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/activity/` | Lists activity records. |
| POST | `/api/activity/` | Creates an activity record. |
| GET | `/api/audit/` | Lists audit records. |
| POST | `/api/audit/` | Creates an audit record. |

## Scanners, Plugins, Updates, and Dashboard Data

### Vulnerability Scans

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/scans/` | Lists scan jobs or results. |
| POST | `/api/scans/` | Starts a vulnerability scan. |
| GET | `/api/scans/summary` | Returns summary statistics. |
| GET | `/api/scans/scanners` | Lists available scanner backends. |
| GET | `/api/scans/by-image` | Returns scan data grouped by image. |
| GET | `/api/scans/{id}` | Returns one scan result. |
| DELETE | `/api/scans/{id}` | Deletes a scan result. |
| GET | `/api/scans/{id}/vulnerabilities` | Returns vulnerabilities for one scan. |

### Plugins

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/plugins/` | Lists plugins. |
| POST | `/api/plugins/` | Installs a plugin. |
| GET | `/api/plugins/{id}` | Returns one plugin. |
| PUT | `/api/plugins/{id}` | Updates plugin configuration. |
| DELETE | `/api/plugins/{id}` | Uninstalls a plugin. |
| POST | `/api/plugins/{id}/toggle` | Enables or disables a plugin. |

### Update Policies

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/updates/` | Lists update policies. |
| POST | `/api/updates/` | Creates an update policy. |
| GET | `/api/updates/check` | Runs an immediate update check. |
| GET | `/api/updates/{id}` | Returns one update policy. |
| PUT | `/api/updates/{id}` | Updates an update policy. |
| DELETE | `/api/updates/{id}` | Deletes an update policy. |
| GET | `/api/updates/{id}/history` | Lists update history for the policy. |

### Dashboard and Metrics Snapshots

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/dashboard/stats` | Returns top-level dashboard statistics. |
| GET | `/api/dashboard/metrics` | Returns dashboard metric snapshots. |
| GET | `/api/metrics/host` | Returns a host metrics snapshot. |
| GET | `/api/metrics/containers` | Returns current container metrics snapshot. |
