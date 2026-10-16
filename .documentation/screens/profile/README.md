# Profile

`/profile`

Your own account: who you are, how McHarbor looks to you, what it tells you, and how you
sign in.

## Permissions

| Key | What it unlocks |
|---|---|
| `account.manage` | Everything here acts on your own account |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- Five tabs: Overview, Account, Appearance, Notifications and Security.
- Badges saying where the account comes from: local, or provisioned by an identity provider.
- Your active sessions, and your multi-factor enrolment state.

## Two accounts behave differently

- **The built-in system account cannot be edited.**
- For an identity-provider account, "display name and email are managed by your identity
  provider", and "passwords for identity-provider accounts are managed by your IdP".

## Appearance

Your own theme and language, overriding the app-wide defaults: **Light** ("use a bright
interface for daylight work"), **Dark** ("use the dark-first McHarbor interface") or
**System** ("follow your browser or operating system setting"). Language changes apply
immediately and are saved to your profile, so they follow you to another browser.

An administrator can reset everyone’s personal overrides back to the app-wide defaults, and
can lock overrides off entirely, in which case these controls are disabled.

## Multi-factor

Enrolment is three steps: scan the QR code (or enter the setup key by hand), **save your
recovery codes**, then enter the six digits from the app. Recovery codes are shown only
once, each works once, and they are the way back in when the device is lost.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Settings → General → Themes](../../settings/general/themes/README.md): the app-wide appearance defaults
- [Security](../security/README.md): accounts other than your own
