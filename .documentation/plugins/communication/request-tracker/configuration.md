# Request Tracker: Configuration

Everything this plugin asks for, as the plugin itself declares it, and what the RT administrator
sets up on the RT side.

## Before you start

In RT, as an administrator:

1. **A dedicated, privileged RT user for McHarbor.** Grant it these rights on the target queue:

   | Right | Used for |
   |---|---|
   | `SeeQueue` | Test connection reads the queue |
   | `ShowTicket` | the dedup search, the post-create check, polling |
   | `CreateTicket` | opening tickets |
   | `CommentOnTicket` | repeat and recovery comments |
   | `ModifyTicket` | setting `resolve_status` on recovery, reopening |
   | `SeeCustomField` | reading the dedup custom field |
   | `ModifyCustomField` | writing the dedup custom field on create |

2. **The dedup custom field.** Admin → Custom Fields → Create: name `McHarbor Dedup Key` (or the
   name you put in `dedup_cf_name`), type **Enter one value**, applies to **Tickets**. Then apply
   it to the target queue. RT **silently drops** the value of a custom field that is not applied
   to the queue, which would defeat deduplication, so McHarbor checks it on Test connection and
   again after every create.
3. **An auth token.** Sign in as the McHarbor user and create one under Logged in as → Settings →
   Auth Tokens.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name. |
| `instance_url` | string | Yes | — | Web root of RT, without `/REST/2.0` (a trailing `/REST/2.0` is removed). Must be `https://`, without credentials, a query or a fragment. Example: `https://rt.example.edu/rt`. |
| `auth_token` | secret | Yes | — | The McHarbor user's RT auth token, sent as `Authorization: token …`. Encrypted at rest. |
| `queue` | string | Yes | — | Queue name. Letters, digits, spaces, dots, dashes and underscores only. |
| `dedup_cf_name` | string | No | `McHarbor Dedup Key` | Name of the dedup custom field. Same character rule as `queue`. |
| `resolve_status` | string | No | `resolved` | Status set on recovery. Must exist in the queue's lifecycle. |
| `requestor` | string | No | — | One e-mail address set as requestor on create. Leave blank so autoreply scrips mail nobody. |
| `default_assignment_group` | string | No | — | RT user name set as owner on create (shown as **Owner**). |
| `update_on_repeat` | bool | No | `true` | Add a comment when an alert fires again while its ticket is open. |
| `reopen_window_minutes` | int 0 to 1440 | No | `0` | Reopen a ticket resolved within this many minutes instead of creating a new one. |
| `poll_interval_minutes` | int 0 to 60 | No | `5` | How often open tickets are polled. `0` means Scrip callout only. |
| `severity_map` | string (JSON) | No | — | Overrides of the priority table, keys `critical`, `warning`, `info`, values 0 to 255. |
| `status_map` | string (JSON) | No | — | Extra RT statuses mapped to `open`, `acknowledged`, `resolved`, `closed` or `cancelled`. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and the API
never returns them once saved. Editing an existing channel leaves the stored token in place unless
you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Communications** and add a channel of type **Request Tracker**.
3. Fill in the fields above and press **Test connection**.
4. Grant `channels.use_comm_request_tracker` to the roles allowed to route events here.
5. Select the channel on an alert rule or a workflow.
6. Optionally install the Scrip callout below, so status changes arrive immediately.

### What Test connection checks

Test connection creates nothing. In order, it:

1. validates every field;
2. `GET /REST/2.0/queue/<queue>`: the queue exists and the user has `SeeQueue`;
3. `GET /REST/2.0/queue/<id>/customfields`: the dedup custom field is **applied to the queue**
   (on an RT without that endpoint it falls back to a custom-field search, which proves only that
   the field exists);
4. `GET /REST/2.0/tickets?query=Queue = '<queue>'`: TicketSQL search works.

## Status updates (Scrip callout)

1. Open the saved channel again. Under **Ticket status from the ITSM**, press
   **Create inbound URL** (or **Rotate inbound URL**) and copy the **Inbound URL** and the
   **Signing secret**. They are shown once. This needs `communications.manage`.
2. On the RT server, create `RT_SiteConfig.d/McHarbor.pm` (shipped as `assets/McHarbor.pm`) and
   restart RT:

   ```perl
   Set($McHarborWebhookSecret, 'paste-the-inbound-secret-here');
   Set($McHarborWebhookURL, 'https://mcharbor.example.com/api/incident-webhooks/paste-the-token-here');
   1;
   ```

   The secret lives in the site configuration rather than in the Scrip, so users who can edit
   Scrips in the web UI cannot read it.
3. In RT, Admin → Scrips → Create, applied to the target queue: condition **On Status Change**,
   action **User Defined**, template **Blank**. Custom action preparation code: `return 1;`.
   Custom action commit code (shipped as `assets/mcharbor-scrip.pl`):

   ```perl
   my $t = $self->TicketObj;
   my ($cf) = @{ $t->CustomFieldValues('McHarbor Dedup Key')->ItemsArrayRef };
   return 1 unless $cf;
   require LWP::UserAgent; require JSON; require Digest::SHA;
   my $body = JSON::encode_json({ id => $t->Id + 0, status => $t->Status,
     old_status => $self->TransactionObj->OldValue, owner => $t->OwnerObj->Name,
     queue => $t->QueueObj->Name, dedup_key => $cf->Content,
     updated => $t->LastUpdatedObj->W3CDTF });
   my $ts = time();
   my $sig = Digest::SHA::hmac_sha256_hex("$ts.$body", RT->Config->Get('McHarborWebhookSecret'));
   LWP::UserAgent->new(timeout => 10)->post(RT->Config->Get('McHarborWebhookURL'),
     'Content-Type' => 'application/json', 'X-McHarbor-Timestamp' => $ts,
     'X-McHarbor-Signature' => "sha256=$sig", Content => $body);
   return 1;
   ```

   If `dedup_cf_name` is not the default, change the name in the second line to match. The RT
   server needs `LWP::Protocol::https` to post to an `https://` URL.

### The signing scheme

The callout is `POST <inbound URL>` with a JSON body and two headers:

| Header | Value |
|---|---|
| `X-McHarbor-Timestamp` | Unix seconds when the callout was sent |
| `X-McHarbor-Signature` | `sha256=` + lowercase hex of HMAC-SHA256, keyed with the signing secret, over `<timestamp>` + `.` + `<raw body>` |

McHarbor rejects a callout when the channel has no signing secret, when the timestamp is more than
300 seconds away from its own clock in either direction, or when the signature does not match
(compared in constant time). A ticket without a dedup value is not McHarbor's and is ignored.

The body:

```json
{"id":77,"status":"resolved","old_status":"open","owner":"jdoe","queue":"Operations","dedup_key":"<hex>","updated":"2026-09-25T10:00:00Z"}
```

## Host scopes it requests

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Request Tracker](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
