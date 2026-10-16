# SCIM provisioning

**Security → Management → SCIM**

A SCIM endpoint points McHarbor at a SCIM 2.0 server (usually your identity provider) that it
**pulls** users and groups from on a schedule. McHarbor never pushes, and never creates accounts
from the directory: a directory user with no matching McHarbor account is recorded as
*unmatched* for an administrator to resolve.

## Permissions

| Key | What it unlocks |
|---|---|
| `scim.view` | The endpoint list and the sync history |
| `scim.manage` | Create, edit and delete endpoints, **Preview and sync**, dry runs, **Rotate token** |

## What a sync does

Each run reads every page of `/Users` and `/Groups`, compares the directory with the
endpoint's mapping rows and the local accounts, and applies the result in one transaction.

| Action | When | What is written |
|---|---|---|
| **Link** | A directory user matches a local account by user name (case-insensitive) or email | A mapping row pointing at the account |
| **Unmatched** | A new directory user matches no local account | A mapping row with no account, for review |
| **Update** | A linked user's email or name changed in the directory | The mapping snapshot, and the account's email / display name |
| **Disable** | A linked user is `active: false`, or has disappeared from the directory | The account is disabled (`disabled_by = scim:<endpoint id>`) and its sessions are revoked |
| **Skip** | The account holds a wildcard (`*`) role, or is already linked to another directory user | Nothing |

A directory that omits `active` is treated as active. Groups are read and reported (how many,
and which match a McHarbor group by name), but a sync does not change group membership.

### Safety rails

- **Empty directory.** If the directory returns no users while accounts are still linked, the
  plan is **blocked** ("The directory returned no users while accounts are still linked") and
  nothing is disabled. A misconfigured filter on the IdP side cannot lock everyone out.
- **Wildcard holders are never disabled** by a sync; they appear as *Skip* with the reason
  "holds a wildcard role, so a sync never disables it".
- More than 50 000 accounts or mapping rows blocks the plan instead of reconciling against a
  truncated list.

## Preview and sync

The refresh icon on a row opens **Preview sync for &lt;name&gt;**. It runs a **dry run**: the
directory is read and compared, and the dialog shows the counts (Seen, Link, New unmatched,
Update, Disable, Skipped, Unchanged, Awaiting an account) and every row that would change, with
its reason. Nothing is written except a *Dry run* entry in the history.

**Run sync now** queues a real sync. It runs on the next poller tick (within a minute, on the HA
leader) and reads the directory again, so it applies what the directory says at that moment.
The button is disabled while the plan is blocked. If the directory cannot be read, the preview
shows the server's message in full.

`POST /api/scim/endpoints/{id}/dry-run` returns `{ status, error?, plan }`; a failed pull is
still HTTP 200 with `status: "failed"`. `POST /api/scim/endpoints/{id}/sync-now` queues the run.

## Rotating the token

**Rotate token** (key icon, bearer endpoints only) asks "Rotate the bearer token for
&lt;name&gt;?", then generates a new token and replaces the stored one in a single write: the old
token stops authenticating immediately. The new token is shown once in the token dialog and is
never retrievable again. `POST /api/scim/endpoints/{id}/rotate-token`; the endpoint records
`tokenRotatedAt`, and the audit log records `rotate_token`.

The bearer token is the same credential McHarbor sends when it pulls from the directory. If the
endpoint uses a token your identity provider issued, rotate it on the IdP and paste the new one
through **Edit** instead; a McHarbor-generated token will not authenticate against the IdP.

## Sync history

The history icon lists the last 50 runs, newest first. Each entry shows the status, a **Dry run**
badge for previews, the trigger (**Scheduled**, **Manual** or **Preview**), the counts
("seen, linked, unmatched, updated, disabled, skipped, groups") and notes naming every account a
run skipped and why. A failed run's error sits behind the red error icon; the message is shown
in full there. `GET /api/scim/endpoints/{id}/history?limit=`.

## The inbound SCIM API

`/api/scim/v2/Users` and `/Groups` expose McHarbor's own accounts and roles (roles appear as
groups) to a client that presents the endpoint's bearer token. They are read-only.

## Related

- [Security](../README.md)
- [Identity provider setup](../identity-setup/README.md)
