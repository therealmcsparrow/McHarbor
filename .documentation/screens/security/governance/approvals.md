# Approvals

**Security → Governance → Approvals** and **Approval rules**

An approval rule makes one category of destructive action need a second person. With **no
rule, nothing changes**: every action runs immediately, exactly as before. A fresh install has
no rules.

## How a held action works

1. A user with the permission for the action: say `containers.delete` (removes a container).
2. A rule covers `container.remove`, so the request is **not executed**. The server stores a
   canonical snapshot (method, path with query, and the JSON body) and answers **HTTP 202** with
   `success: false`, `code: err.forbidden` and `data.reason: "approval_required"`, plus
   `data.approvalId`, `category`, `requiredApprovers` and `expiresAt`.
3. The dialog the user confirmed in shows **"Sent for approval"** with the request id instead of
   an error, and a toast says the same. Every approver (holders of `approvals.decide`) gets a
   push notification.
4. Someone **other than the requester** approves it under **Approvals**. A rule can require up to
   five distinct approvers; a single **Deny** closes the request.
5. The requester presses **Run now**. The stored request is replayed through the full router as
   the requester: every middleware and route gate runs again, so a permission removed in the
   meantime is refused now. It runs **exactly once**: a second **Run now**, or a race between two,
   finds the request already executed.

A caller who would be refused by the route anyway is **not** queued; they get the route's own
403. An approval can therefore never widen what someone is allowed to do.

Clicking the same destructive button twice while a request is open returns the same approval
rather than queuing a second.

### Statuses

`pending` → `approved` | `denied` | `expired` | `cancelled`, and `approved` → `executing` →
`executed`. Nothing moves backwards. A request expires when its window (`expiry_minutes` on the
rule, default 1440) passes, whether it was pending or approved but not yet run; expiry is
applied on every read, so there is no background job to wait for. **Withdraw** lets the
requester cancel an open request.

An executed request records the replayed response's HTTP status and code. When the replay
failed (status 400 or above) the row shows the error icon; open it for the status and code.

### Secrets in the snapshot

Approvers see the body with every value whose key contains `password`, `secret`, `token`,
`key`, `credential`, `private`, `authorization`, `cookie` or `passphrase` replaced by
`[redacted]`. The exact body needed for the replay is stored separately, **encrypted with the
application key**, and is wiped once the request has run. A body that is not JSON is shown only
as `[redacted]`.

## Risk categories

| Category | Held routes | Permission the requester must hold |
|---|---|---|
| `container.remove` | `DELETE /api/containers/{id}`, `POST /api/containers/{id}/remove` | `containers.delete` |
| `container.kill` | `POST /api/containers/{id}/kill` | `containers.manage` |
| `volume.remove` | `DELETE /api/volumes/{name}` | `volumes.delete` |
| `volume.prune` | `POST /api/volumes/prune` | `volumes.manage` |
| `image.remove` | `DELETE /api/images/{id}` | `images.delete` |
| `image.prune` | `POST /api/images/prune` | `images.manage` |
| `stack.down` | `POST /api/stacks/{name}/down` | `stacks.manage` (stack scope) |
| `stack.remove` | `DELETE /api/stacks/{name}` | `stacks.delete` (stack scope) |
| `environment.delete` | `DELETE /api/environments/{id}` | `environments.manage` |
| `plugin.uninstall` | `POST /api/appstore/plugins/uninstall` | `plugins.manage` or `plugins.delete` |
| `user.delete` | `DELETE /api/users/{id}`, `POST /api/users/bulk/delete` (not its dry run) | `users.manage` |
| `role.delete` | `DELETE /api/roles/{id}` | `roles.manage` |

The rule builder lists these routes under **Holds these routes** when a category is picked.

## Rules

| Field | Meaning |
|---|---|
| Risk category | One of the categories above |
| Environment | **All environments**, or one environment. A rule for the request's `?env=` wins over an all-environments rule |
| Required approvers | 1 to 5 distinct people, none of them the requester |
| Approver permission | The key an approver must hold in the request's environment; default `approvals.decide`. Deciding also needs `approvals.decide` on the route |
| Expires after (minutes) | 5 to 10080; the window to decide **and** run |
| Enabled | A disabled rule holds nothing |

Editing or deleting a rule affects new requests only; requests already held keep the terms
they were created with.

## Routes

| Route | Gate |
|---|---|
| `GET /api/approvals` | `approvals.view` or `approvals.decide` |
| `GET /api/approvals/mine` | signed in; the caller's own requests |
| `GET /api/approvals/{id}` | the requester, or `approvals.view` / `approvals.decide` |
| `POST /api/approvals/{id}/decide` `{approve, note}` | `approvals.decide`, plus the rule's approver permission |
| `POST /api/approvals/{id}/execute` | the requester only; the replayed route re-checks its own key |
| `POST /api/approvals/{id}/cancel` | the requester only |
| `GET /api/approvals/categories`, `GET /api/approvals/rules` | `approvals.view` or `approvals.manage` |
| `POST /api/approvals/rules`, `PUT/DELETE /api/approvals/rules/{id}` | `approvals.manage` |

## Audit

`approval.request` (warning), `approval.approve`, `approval.deny`, `approval.cancel`,
`approval.execute` (error severity when the replay failed), and `approval_rule.create` /
`.update` / `.delete`. The replayed action itself is audited by its own route as usual.

## What goes wrong

| Symptom | Cause |
|---|---|
| "Sent for approval" on a delete you expected to run | An enabled rule covers that category in that environment |
| Approve is disabled | You lack `approvals.decide` (on your own requests Approve and Deny are not offered at all) |
| "Forbidden" with reason `approver_permission` | The rule names an approver permission you do not hold in that environment |
| Run now answers 409 | The request was already run, is no longer approved, or expired |
| Run now succeeds but the row shows an error | The replayed route refused or failed, for example because your permission was removed after approval |

### Limits

- Execution is by the requester only; nothing runs automatically on the last approval.
- A request made with an API key is replayed with the executor's own session (or key). The
  key's environment fence is checked when the request is held, and the route re-checks the
  executor at run time.
- The App Store's streaming uninstall (`POST /api/appstore/installations/{id}/uninstall/stream`)
  is not a held route: a streamed response cannot be replayed into a single result.
