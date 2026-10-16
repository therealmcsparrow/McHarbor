# Elevations

`/security/elevations` · Security → Management → Elevations

Ask for a permission you do not hold, for a bounded window. Someone else approves it, and the
grant lapses on its own when the window ends.

## Permissions

| Key | What it unlocks |
|---|---|
| none | Requesting, seeing and withdrawing your own requests |
| `users.manage` | The **Pending approvals** inbox, deciding requests, and **Requestable permissions** |

Routes: `POST /api/elevations`, `GET /api/elevations/mine`, `GET /api/elevations/elevatable`
(any signed-in user), `GET /api/elevations/pending`, `GET /api/elevations/all`,
`POST /api/elevations/{id}/decide`, `PUT /api/elevations/elevatable` (`users.manage`),
`DELETE /api/elevations/{id}` (your own pending request).

A request opens the approvers' inbox and notifies every approver, so it is capped: at most
**five** requests can be pending per user, and a second request for the same permission and
environment while one is pending is refused (`too_many_pending`, `already_pending`). A request
can be read only by its requester and by holders of `users.manage` at global scope. An API key
needs `account.self.manage` to request or withdraw.

An elevation approved for an **environment** adds only the environment-, stack- and
owner-scoped keys it names there, exactly as a role assigned on that environment does; a global
or account key needs an elevation that is not tied to an environment.

## Requesting

**Permission** is a searchable list of every permission you may request, shown by its
description with the key underneath, e.g. "Manage virtual machines", `vm.manage`. Plugin
permissions are included. **Minutes** is 1 to 1440; **Justification** is required.

## Deciding

The inbox never offers your own request: an elevation cannot be approved by the person who
asked for it. The same requests appear on the on-call **Approvals** page, with the
permission's description.

## Requestable permissions

A permission is requestable when:

1. an admin switched it on or off under **Requestable permissions**; that choice always wins;
2. otherwise, it was one of the permissions marked requestable before this list existed;
3. otherwise, the default rule applies: operational actions (`manage`, `delete`, `exec`,
   `run`, `restore`, `apply`, `reveal`, `enforce`, `access`, `act`, `export`, `use`,
   `execute`, `deploy`, `promote`, `rollback`, `shell`, `console`, `update`, `create`) are
   requestable, except in the categories that administer access itself: `users`, `roles`,
   `groups`, `permissions`, `api_keys`, `approvals`, `account`, `sessions`,
   `identity_providers`, `scim`, `license`, `rbac`, `elevations`, `audit`.

Each row shows whether the current state is **Set by admin** or **Default**. Changes are
audited as `elevation_policy` updates.

## What goes wrong

| Symptom | Cause |
|---|---|
| "Permission is not elevatable" | The key is switched off, or excluded by the default rule |
| A plugin permission is missing from the list | The plugin is not installed, or it declares no `adds_permissions` |
| An approved request grants nothing | The window has already ended; requests expire as `expired` |
