# User lifecycle and bulk actions

**Security → Governance → User lifecycle**

Every account (except the internal `system` and legacy-owner placeholders) with its lifecycle
state, filters with counts, and bulk actions over a selection.

## Filters

| Filter | An account matches when |
|---|---|
| All | always |
| Disabled | an admin disabled it |
| Locked | failed sign-ins locked it and the lock has not expired |
| No MFA | it has no enabled second factor |
| Inactive | its last sign-in (or, if it never signed in, its creation) is older than **Inactive after (days)** (default 90) |
| Recently elevated | an elevation was approved for it, or it opened break-glass, within the last 7 days |

Accounts designated for break-glass carry a **Break-glass** badge.

`GET /api/users/lifecycle?inactiveDays=&elevatedDays=` needs `users.view`. Last sign-in is the
existing `users.last_login`, which every successful local and SSO login already updates.

## Bulk actions

Select accounts, then **Disable**, **Enable**, **Unlock** or **Delete**. The dialog first runs a
**dry run** and lists, per user, whether it "Will change" or is skipped and why; nothing changes
until you confirm, and the confirm stays disabled when nothing would change. The apply uses the
same rules as the preview:

| Skip reason | When |
|---|---|
| Your own account | disabling or deleting yourself |
| Last active admin | the batch would leave no active admin, checked across the whole batch, so selecting every admin keeps the last one |
| Already disabled / Not disabled / Not locked | the action has nothing to do |
| Managed by an identity provider | disabling an account an IdP owns |
| Not found | the id no longer exists |

| Route | Gate |
|---|---|
| `POST /api/users/bulk/account` `{action: disable\|enable\|unlock, userIds, dryRun, reason}` | `account.manage` |
| `POST /api/users/bulk/delete` `{userIds, dryRun}` | `users.manage`; also held by a `user.delete` approval rule (dry runs are not) |

At most 200 accounts per request. An apply writes one audit row per changed account
(`user.disable`, `user.enable`, `user.unlock`, `user.delete`) and a summary row
`user.bulk_<action>` with the applied, skipped and failed counts and the reason. A dry run
writes nothing. Deleting archives each account's API keys first, as a single delete does.
