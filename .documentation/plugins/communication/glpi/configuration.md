# GLPI: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A GLPI 9.5, 10 or 11 instance reachable from McHarbor over **https**.
- A dedicated GLPI user whose profile can create, update and solve tickets and add
  follow-ups in the target entity.
- **API v1:** the REST API enabled, login with an external token enabled, an API client
  whose IPv4 range admits McHarbor's address, and the user's API token (the "remote access
  key" in the user's preferences).
- **API v2 (GLPI 11):** an OAuth client with the **password** grant and the `api` scope,
  and that user's login and password.
- Optional, GLPI 11 only: permission to add a webhook, for status coming back without
  polling.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `instance_url` | string | Yes | — | Base URL of GLPI, `https://` only, without `/apirest.php` or `/api.php` (a trailing one is stripped). |
| `api_version` | string | No | `v1` | `v1` (`apirest.php`, GLPI 9.5 to 11) or `v2` (`api.php` high-level API, GLPI 11+). Anything else fails the test. |
| `app_token` | secret | No | — | API v1. The API client's App-Token; needed only when the API client defines one. |
| `user_token` | secret | v1 | — | API v1. The API user's API token. Sent as `Authorization: user_token …` to `initSession` only. |
| `v2_path` | string | No | `v2.0` | API v2. The version segment after `/api.php/`, for example `v2.0` or `v2.3`. Must match `v2.<number>`. |
| `oauth_client_id` | string | v2 | — | API v2. The OAuth client's identifier. |
| `oauth_client_secret` | secret | v2 | — | API v2. The OAuth client's secret. |
| `oauth_username` | string | v2 | — | API v2. The API user's login. |
| `oauth_password` | secret | v2 | — | API v2. The API user's password. |
| `entities_id` | int | No | `0` | Entity new tickets land in. `0` leaves them in the API user's active entity; above 0 it is sent as `entities_id` (v1) or `entity` (v2), and Test checks it is active for the user on v1. |
| `itilcategories_id` | int | No | `0` | ITIL category for new tickets; `0` sets none. |
| `close_on_resolve` | bool | No | `false` | After the solution, also set the ticket to Closed (6) instead of leaving it Solved (5). |
| `webhook_signing_secret` | secret | No | — | GLPI 11. The secret from GLPI's webhook form. Takes precedence over the channel's inbound signing secret. |
| `update_on_repeat` | bool | No | `true` | When the alert fires again while its ticket is open, add a private follow-up. |
| `reopen_window_minutes` | int | No | `0` | 0 to 1440. Reopen a ticket solved within this window instead of opening a new one. |
| `poll_interval_minutes` | int | No | `5` | 0 to 60. How often open tickets are read. `0` turns polling off (webhook only). |
| `default_assignment_group` | int | No | `0` | API v1. Group assigned to new tickets, sent as `_groups_id_assign`; `0` assigns none. Not sent on API v2. |
| `severity_map` | string | No | — | JSON object overriding the severity table, for example `{"critical":{"urgency":5,"impact":5,"priority":6}}`. Keys are `critical`, `warning`, `info`; urgency and impact 1 to 5, priority 1 to 6. Anything else fails the test. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you replace it. No token, secret, password or session token appears in an
error message; a GLPI message that echoes one is shown with it redacted.

## Setting it up

### In GLPI

1. Open **Setup → General → API**. Enable the REST API and, for API v1, enable login with
   an external token.
2. API v1: under the same page, add an API client whose IPv4 range includes McHarbor's
   address. Copy its App-Token if one is generated.
3. Create a dedicated user with a profile that can create, update and solve tickets and add
   follow-ups in the target entity. For API v1, generate the API token in that user's
   preferences.
4. API v2 on GLPI 11: open **Setup → OAuth clients**, create a client with the password
   grant and the `api` scope, and note its ID and secret.

### In McHarbor

5. Install the plugin from **App Store → Plugins**.
6. Open **Settings → Integrations → Communications**, add a channel of type **GLPI**, pick
   the API and fill in the credentials for it, and the entity, category and group IDs.
7. Save and press **Test connection**. Nothing is created; see
   [Troubleshooting](./troubleshooting.md) if it fails.

### Status from GLPI 11 (optional)

8. Edit the channel again. Under **Ticket status from the ITSM**, select
   **Create inbound URL** and copy the URL, which ends in `/api/incident-webhooks/` and a
   token, and the **Signing secret** shown beside it. Both are shown once. This needs
   `communications.manage`.
9. In GLPI, open **Setup → Notifications → Webhooks** and add a webhook for itemtype
   **Ticket** on the **update** event (add **delete** and **purge** to read trashed tickets
   back), with the inbound URL and the HTTP method **POST**. Leave the challenge-response
   (CRA) option **off**.
10. Make GLPI and McHarbor share one secret: copy the webhook's secret into
    **GLPI webhook secret** on the channel (or into **Signing secret issued by the ITSM**),
    or enter McHarbor's signing secret as the webhook's secret in GLPI.
11. Optionally, filter the webhook to tickets whose external ID starts with `mcharbor:`.

GLPI 9.5 and 10 have no webhooks: leave steps 8 to 11 out and keep a poll interval above 0.

### Routing

12. Grant `channels.use_comm_glpi` to the roles allowed to route events here.
13. Select the channel on an alert rule, or in an Open, Update or Resolve Ticket workflow
    node.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Outbound endpoints

All on the configured GLPI URL.

| Path | Used for |
|---|---|
| `GET /apirest.php/initSession`, `GET /apirest.php/killSession` | v1 session per call chain |
| `GET /apirest.php/getActiveEntities` | v1 Test |
| `GET /apirest.php/Ticket/?searchText[externalid]=…`, `POST /apirest.php/Ticket/` | v1 Open |
| `GET` / `PUT /apirest.php/Ticket/<id>` | v1 poll, resolve check, close |
| `POST /apirest.php/ITILFollowup/`, `POST /apirest.php/ITILSolution/` | v1 Update and Resolve |
| `POST /api.php/token` | v2 password grant |
| `GET` / `POST /api.php/<v2_path>/Assistance/Ticket` | v2 Test, Open |
| `GET` / `PATCH /api.php/<v2_path>/Assistance/Ticket/<id>` | v2 poll, resolve check, close |
| `POST /api.php/<v2_path>/Assistance/Ticket/<id>/Timeline/Followup` and `/Solution` | v2 Update and Resolve |

## Related

- [GLPI](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
