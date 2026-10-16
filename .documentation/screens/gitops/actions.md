# GitOps: Actions

| Action | Gated by | What it does |
|---|---|---|
| **Add pipeline** | `gitops.manage` | Name, source repository, trigger and stages |
| **Promote** | `gitops.manage` | Takes a commit SHA and an optional note, and moves it to the next stage |
| **Approve** | `gitops.approve` | Takes an optional note, and lets the promotion proceed |
| **Reject** | `gitops.approve` | Asks for a reason, and stops it |
| **Delete pipeline** | `gitops.delete` | Removes the pipeline and its configuration |

Approvals and rejections are recorded with who and why, so the audit trail answers "who let
this into production" without anyone having to remember.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [GitOps](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
