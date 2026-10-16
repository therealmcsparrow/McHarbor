# Profile: Troubleshooting

When your own profile will not change. Three of the four are someone else's decision
rather than a fault: an identity provider owns your name and email, and an administrator
can lock appearance overrides off or reset them for everyone.

| Symptom | Usual cause | Fix |
|---|---|---|
| Display name and email are read-only | The account comes from an identity provider | Change them at the source |
| Appearance controls are disabled | Overrides are locked off app-wide | An administrator controls this in Settings → General → Themes |
| Your theme reverted | An administrator reset personal overrides | Set it again; the reset applies to every account at once |
| Locked out after losing the authenticator | Two-factor with no recovery codes to hand | Recovery codes are shown once at enrolment; otherwise an administrator has to intervene |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Profile](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
