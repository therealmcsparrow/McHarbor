# iTop: Configuration

Everything this plugin asks for, as the plugin itself declares it, and what to prepare in
iTop first.

## Before you start

In iTop:

- A **Person** and a **User account** for McHarbor. The account needs the
  **REST Services User** profile, plus a profile that may create, update and apply
  stimuli to tickets of the chosen class in the chosen organization.
- On iTop 3.1 or later, an **application token** for that user (Admin tools →
  Application tokens), or a personal token. On older versions, the user's login and
  password.
- The **ID of a team**, the **ID of an agent who is a member of that team**, and the
  **ID of a service**. iTop cannot resolve a ticket without all three: `ev_assign`
  requires `team_id` and `agent_id`, and the `resolved` state requires `service_id`,
  `resolution_code` and `solution`.
- The exact **name of the organization** the tickets belong to.

The iTop URL must be reachable from the McHarbor server over `https://`.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `instance_url` | string | Yes | — | Base URL of iTop, for example `https://itop.example.fr` or `https://intranet.example.fr/itop`. Must be `https://`, with no credentials, query or fragment. Redirects are not followed, so give the final address. |
| `auth_mode` | string | No | `token` | `token` (iTop 3.1+) or `password`. |
| `auth_token` | secret | With `token` | — | Application or personal token. Encrypted at rest. |
| `auth_user` | string | With `password` | — | REST user login. |
| `auth_pwd` | secret | With `password` | — | REST user password. Encrypted at rest. |
| `ticket_class` | string | No | `Incident` | `Incident` or `UserRequest`. Nothing else is accepted. |
| `org_name` | string | Yes | — | Organization name; sent as `SELECT Organization WHERE name = '…'`. Must match exactly one organization. |
| `caller_email` | string | No | — | Email of the Person used as `caller_id`. Must match exactly one Person. Leave blank if your data model does not require a caller. |
| `origin` | string | No | `monitoring` | Value of the ticket's `origin` field. |
| `team_id` | int | Yes | — | Team assigned by `ev_assign` when a ticket must be assigned before it can be resolved. |
| `agent_id` | int | Yes | — | Agent assigned by `ev_assign`. Must be a member of `team_id`. |
| `service_id` | int | Yes | — | Service set by `ev_resolve`. |
| `resolution_code` | string | No | `other` | Resolution code set by `ev_resolve`, for example `assistance`, `other`, `software patch`, `training`, `hardware repair`. |
| `close_on_resolve` | bool | No | false | Apply `ev_close` straight after `ev_resolve`. |
| `update_on_repeat` | bool | No | true | Append each repeat firing to the private log. |
| `reopen_window_minutes` | int | No | 0 | 0 to 1440. Reopen a ticket resolved within this many minutes (`ev_reopen`) instead of creating a new one. 0 always creates a new one. |
| `poll_interval_minutes` | int | No | 5 | 0 to 60. How often McHarbor reads the status of open tickets. 0 means webhook only. |
| `default_assignment_group` | int | No | 0 | Team ID set at creation. The ticket stays `new`; 0 sets no team. |
| `severity_map` | string | No | — | JSON override of the severity table, for example `{"critical":{"impact":"1","urgency":"1"}}`. Keys are `critical`, `warning`, `info`; `impact` is 1 to 3 and `urgency` 1 to 4. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you replace it.

Credentials travel in the POST form body, never in the query string, so they stay out of
web-server access logs.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Communications** and add a channel of type iTop.
3. Fill in the fields above and press **Test connection**. The test creates nothing. It
   checks, in order: the credentials (`list_operations`), that the organization name
   matches exactly one organization, that the caller email (when set) matches exactly one
   Person, that `team_id`, `agent_id`, `service_id` and `default_assignment_group` (when
   set) exist, and that the agent is a member of the team.
4. Grant `channels.use_comm_itop` to the roles allowed to route events here.
5. Select the channel on an alert rule or a workflow node.
6. Optionally, set up the webhook below so status changes arrive immediately instead of
   on the poll interval.

## Webhook recipe

iTop does not sign webhooks, so McHarbor authenticates them with a static header whose
value is the channel's inbound secret.

1. In McHarbor, open the saved channel again. Under **Ticket status from the ITSM**, press
   **Create inbound URL**, and copy the **Inbound URL** and the **Signing secret**. They
   are shown once.
2. In iTop, create a **Remote application connection** with header-based
   authentication, whose header is `X-McHarbor-Secret` and whose value is the signing
   secret.
3. Create a **Webhook action** (Combodo Webhook Integration) that uses that connection,
   with the inbound URL, method `POST`, the header `Content-Type: application/json`, and
   this payload template:

   ```json
   {"id":"$this->id$","ref":"$this->ref$","class":"$this->finalclass$","title":"$this->title$","status":"$this->status$","agent":"$this->agent_id_friendlyname$","last_update":"$this->last_update$"}
   ```

4. Create a **Trigger (on object update)**, or **Trigger (on entering a state)**, on the
   ticket class, with the filter `SELECT Incident WHERE title LIKE '%[MH-%'` (use
   `UserRequest` for that class), and link it to the action.

A payload whose title has no McHarbor marker, whose status is unknown, or whose ticket
McHarbor never opened is ignored. Rotating the inbound URL issues a new URL and secret;
update both in iTop, or the webhook stops being accepted.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [iTop](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
