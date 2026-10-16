# Break-glass

**Security → Governance → Break-glass**

Emergency access for when normal access is not enough: the identity provider is down, the
only admin is unreachable. It is deliberately simple and deliberately loud.

## Designating accounts

An account manager (`account.manage`) picks a user and a **Longest window (minutes)** (5 to 240,
default 60) and presses **Designate**. The list shows every designated account, any session it
has open, and the recent sessions. **Remove** takes the designation away and ends any open
session with it.

Keep break-glass accounts local (not SSO-managed), with a strong password and MFA, stored
where the people on call can reach them; the designation grants nothing until it is used.

## Opening emergency access

A designated user signs in normally and presses **Open emergency access**. A live session is
not enough on its own, because the account is about to hold every permission: the interface
asks for the account's **password** first and opens a `break_glass` step-up window
(`POST /api/auth/sudo` with scope `break_glass`, five minutes, bound to that session). Without
the window the activation is refused with `403` `err.sudo_required`. They must then give a
**reason** of at least 10 characters and a window no longer than their designation allows.
Then, until the window ends:

- the account holds **every permission** (`*`) in every environment, through the normal
  permission path (the same place elevations are applied), not cached, so it stops the moment
  the window passes or the session is ended;
- the grant applies to the interactive session only; API keys owned by the account never
  receive it, and activating through an API key is refused;
- a **`break_glass.activate`** audit entry is written at **error** severity with the reason and
  the end time, an in-app notification is posted, and every holder of `account.manage` gets a
  critical push notification.

**End emergency access** closes it early (`break_glass.end`). An account manager can end
anyone's session from the accounts list.

Only one session can be open per account at a time, and the database enforces it: a second
activation, including two requests racing, answers `409` with `reason: already_active`.

## Routes

| Route | Gate |
|---|---|
| `GET /api/break-glass/status` | signed in; the caller's own designation and session |
| `POST /api/break-glass/activate` `{reason, minutes}` | the caller must be designated, must hold a live `break_glass` step-up window on this session, and cannot use an API key (self-service; audited and paged) |
| `POST /api/break-glass/end` | ends the caller's own session |
| `GET /api/break-glass/accounts`, `GET /api/break-glass/sessions` | `users.view` |
| `PUT /api/break-glass/accounts/{userId}` `{maxMinutes, note}` | `account.manage` |
| `DELETE /api/break-glass/accounts/{userId}` | `account.manage` |
| `POST /api/break-glass/accounts/{userId}/end` | `account.manage` |

## Audit

`break_glass.designate`, `break_glass.undesignate`, `break_glass.activate` (error),
`break_glass.end`. The policy simulator and the effective-access view both show an open
session as the reason a permission is held.
