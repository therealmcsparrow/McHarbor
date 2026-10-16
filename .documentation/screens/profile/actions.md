# Profile: Actions

Everything you can change about your own account (name, email, password, two-factor
enrolment and your own sessions), all of it behind `account.manage`. Recovery codes are
shown once, at enrolment.

| Action | Gated by | What it does |
|---|---|---|
| **Save profile** | `account.manage` | Display name and email, "as other McHarbor users see them" |
| **Update password** | `account.manage` | Current, new and confirmation. Minimum eight characters |
| **Enable two-factor** | `account.manage` | QR code or setup key, recovery codes, then a six-digit confirmation |
| **Turn off two-factor** | `account.manage` | "Your account will be protected by a password only" |
| **Copy recovery codes** | `account.manage` | Shown once, at enrolment |
| **Sign out a session** | `account.manage` | Ends one of your own sessions |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Profile](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
