# Git: Troubleshooting

What goes wrong with a tracked repository. The one that surprises people is the last: for
a repository-backed stack, Git is the source of truth, so edits made in McHarbor are
overwritten on the next sync.

| Symptom | Usual cause | Fix |
|---|---|---|
| Status is error | Wrong branch, bad credentials, or a host the container cannot reach | The row keeps the last successful sync time; start from when it broke |
| A private repository will not clone | No credential, or the wrong kind for the URL scheme | HTTPS wants a token; SSH wants a key |
| Stack changes keep reverting | Git is the source of truth for a repository-backed stack | Change the repository, not the stack |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Git](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
