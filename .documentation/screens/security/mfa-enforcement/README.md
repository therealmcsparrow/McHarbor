# Requiring two-factor authentication

`/security` → **Password** → **Require two-factor authentication**

Two-factor authentication (TOTP, with one-time recovery codes) is something each user turns on
for themselves under **Profile → Two-factor authentication**. This control makes it
**mandatory** for a group of users. A covered user who has not enrolled is held on a setup
screen straight after their password until they enrol; nothing else in McHarbor answers them
until they do.

## The setting

| | |
|---|---|
| Settings key | `security.mfa_required` (category `security`), seeded by migration `111_mfa_enforcement.sql` |
| Values | `off` · `admins` · `all` |
| Default | `off`; an upgrade never locks anyone out |
| Read | `GET /api/settings/security.mfa_required`, gated by `settings.view` |
| Write | `PUT /api/settings/security.mfa_required`, gated by `settings.manage`. Any other value is refused with `err.settings.mfa_requirement_invalid` |

No new permission key exists for this: it reuses `settings.view` and `settings.manage`, and
the **Save** button renders disabled with a tooltip naming `settings.manage` for anyone who
lacks it.

| Value | Label in the UI | Who must enrol |
|---|---|---|
| `off` | Optional | Nobody. Everyone chooses for themselves |
| `admins` | Administrators | Every user holding `settings.manage` globally: directly, through a group, or through a wildcard role. It is the key that can turn this policy off again, so it is the one whose password alone must not be enough |
| `all` | Everyone | Every interactive user, **including single sign-on accounts**; the policy does not know whether the identity provider already asked for a second factor |

A write takes effect on the next request; there is no restart and no wait.

## What a covered user sees

1. They sign in with their password as usual. The session is created (they need it to reach
   the enrolment routes), and the login is audited as `login` followed by
   `login_mfa_enrollment_required` (severity `warning`).
2. Every page redirects to `/mfa-enrollment`, titled **"Two-factor authentication
   required"**: **Set up two-factor authentication**, then the same three steps as the profile
   page (scan, save recovery codes, verify).
3. On **Verify and turn on** the hold lifts at once and they continue to the dashboard.
   **Sign out instead** is the only other way off the screen.

An administrator who turns the policy on and is covered by it without having enrolled is
taken to the same screen straight after saving.

## How it is enforced

The screen is only the visible half. The server holds the session itself: while a user owes
enrolment, **every** authenticated request (host routes and plugin routes alike) answers
`403` with code `err.mfa.required` ("Two-factor authentication is required"), except:

| Still reachable | Why |
|---|---|
| `GET /api/auth/session`, `/api/auth/session/*` | The browser learns it is held (`mfaEnrollmentRequired: true` in the session body) and keeps its idle countdown |
| `POST /api/auth/logout` | Leaving |
| `/api/auth/mfa`, `/api/auth/mfa/*` | Enrolling |

Skipping the redirect in the browser's developer tools therefore gets nothing: the session
is useless until the factor is enrolled.

Two further rules:

- **A covered user cannot turn their factor off.** `DELETE /api/auth/mfa` answers `403`
  `err.mfa.required` while the policy covers them.
- **API keys are not held.** A request authenticated with an API key is its own credential,
  not a password session, and passes through. Scope API keys with the usual care.

## Recovery: locked out by your own policy

A covered user who still knows their password is never locked out by the policy itself; they
are held at enrolment, and enrolling is something they can do. Two situations need an operator.

**A user lost their authenticator and their recovery codes.** McHarbor has no screen to reset
another user's second factor, and the policy stops them removing it themselves. Delete their
factor in the database; at next sign-in they are held at the setup screen and enrol a new
device:

```bash
# SQLite (the default), from a shell in the container
sqlite3 /app/data/mcharbor.db \
  "DELETE FROM user_mfa WHERE user_id = (SELECT id FROM users WHERE username = 'alice');"
```

**Enrolment itself cannot be completed** (for example the host clock is so far off that no
code verifies), and every covered administrator is held. Turn the policy off directly, fix the
cause, sign in and turn it back on:

```bash
sqlite3 /app/data/mcharbor.db \
  "UPDATE settings SET value = 'off', updated_at = CURRENT_TIMESTAMP WHERE key = 'security.mfa_required';"
```

On PostgreSQL, MySQL or SQL Server run the same statements against the same tables. An empty or
unrecognised `security.mfa_required` value also reads as `off`, so deleting the row works
too. The server caches the value for at most fifteen seconds, so neither change needs a
restart.

There is no environment variable that overrides this setting, deliberately: an override anyone
with access to the container's environment could set would be a second way around the control.

## Related

- [Security](../README.md): the rest of the section
- [Profile](../../profile/README.md): where a user enrols voluntarily
- [Authentication](../../../api/conventions/authentication.md): sessions, cookies and API keys
