# Audit: Troubleshooting

When the trail does not say what you expected. Three of these four are about what the
trail legitimately does not contain: fallback coverage, retention, and actions nobody
asked McHarbor to take. The fourth, a failed verification, is an incident.

| Symptom | Usual cause | Fix |
|---|---|---|
| An action you expected is missing | It was covered by fallback middleware, not a canonical action | Look for the less specific `<module>.route.*` entry on Events; the Coverage tab says how many routes are in that state |
| A plugin you removed is still listed under Coverage | Its audit history outlives it, and the uninstall is itself one of its events | Expected; the row is marked **Uninstalled** and its last-event date says when it was last active |
| Verification fails | The chain is broken from some point on | Treat it as an incident: the entry that breaks it is where to start |
| Verification fails at the oldest surviving row after an upgrade | Rows were pruned before retention tombstones existed, so nothing records the gap | Expected once on an install that pruned before this release; see [Integrity](./integrity.md) |
| **Tombstones with a bad seal** is not 0 | A tombstone was written or altered by something other than the pruner, or the HMAC secret changed | Treat as an incident, like a broken row |
| A denial shows a user id instead of a name | The user has since been deleted; ABAC rows store only the id, and the name is looked up when you read | The id is still in the row's details |
| The same denial appears once although it happened many times | Repeats of one user, permission and route within a minute are recorded once | Expected; the throttle keeps polling pages from flooding the trail |
| The trail is shorter than expected | Retention purged it | Retention is the one legitimate way to remove entries; agree the window before an incident |
| Nothing corresponds to a container stop | Nobody asked McHarbor to stop it | That is the answer: check Activity and the Lifecycle log for what the daemon did |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Audit](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
