# General

How the application looks and behaves.

| Section | Covers | Gated by |
|---|---|---|
| [General](./general/README.md) | Language, time zone, date and time formats, and how often data refreshes | `settings.manage` |
| [Customization](./themes/README.md) | Application logo, theme variant, accent colour, fonts, motion, per-variable overrides | `settings.manage` |
| [Notifications](./notifications/README.md) | Where toasts appear, how long they stay, and what users may override | `notifications.view`, `settings.manage` |

## The pattern in this group

Two of these sections set an **app-wide default** and then decide whether individual users
may depart from it. Both Customization and Notifications carry **Let users override these
defaults** (off means every account uses the defaults and the personal controls on
[Profile](../../screens/profile/README.md) are disabled) and **Reset overrides**, which
returns every account to the defaults at once.

The one setting here that is not cosmetic is the **time zone**, on the General section. It
decides what every cron expression in the product means, and changing it restarts McHarbor.
Set it during first run.

## Related

- [Settings](../README.md): the other five groups
