# Git: Actions

| Action | Gated by | What it does |
|---|---|---|
| **Add repository** | `git.manage` | Name, repository URL and branch |
| **Sync** | `git.manage` | Pulls the repository now |
| **Remove** | `git.manage` | Removes the repository configuration. Stacks it deployed are not touched |

Credentials for a private repository are encrypted at rest, like every other credential in
McHarbor.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Git](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
