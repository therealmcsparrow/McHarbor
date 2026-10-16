# Notifications: Reference

| Setting | Default | What it does |
|---|---|---|
| **Position** | — | Which corner or edge toasts appear from |
| **Duration** | — | How long a toast stays before dismissing itself. A "never" setting keeps them until dismissed by hand |
| **Maximum visible** | — | How many may stack before the oldest is dropped |
| **Rich colours** | — | Tint each toast by its kind instead of one neutral surface |
| **Close button** | — | Show an explicit close control. **Forced on** while duration is "never" |
| **Let users override these defaults** | On | Off disables the personal notification controls on every account |
| **Reset overrides** | — | Returns every account to these defaults |

The close button is forced on while duration is "never": a toast that never dismisses
itself and has no close control is a permanent obstruction, so the setting refuses that
combination and says why.

**Show preview** raises sample toasts (success, warning and error), stacked to the
configured maximum. Use it: position and duration are hard to judge from a form and obvious
from three seconds of the real thing.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| Toasts cover something important | Position and stack limit | Move them, or lower the maximum visible |
| A failure toast disappears before it can be read | Duration is too short | Raise it, or rely on the notification inbox for anything that must not be missed |
| Users cannot change their toast settings | Overrides are switched off | Turn them back on |
| The close button will not turn off | Duration is set to never | Intended; set a duration and the switch is yours again |

## Related

- [Notifications](./README.md): what the section is for
- [Settings](../../README.md): the other groups
