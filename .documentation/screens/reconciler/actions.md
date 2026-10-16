# Reconciler: Actions

## On the watch list

| Action | Gated by | What it does |
|---|---|---|
| **Add Desired State** | `reconciler.manage` | A name, what to watch, a desired status, and **When it drifts** (**Observe** or **Propose a fix**) |
| **Reconcile** | `reconciler.manage` | Makes a single container match; may start, stop or remove it. Disabled for stack and environment watches |
| **Check Drift** | `reconciler.manage` | Compares declared against actual now, without changing anything |
| **Change how drift is answered** | `reconciler.manage`; **Enforce** needs `reconciler.enforce` | Choose **Observe**, **Propose a fix** or **Enforce**. Choosing **Enforce** warns what it means, and the button reads **Let McHarbor fix it automatically** |
| **Propose a fix** | `reconciler.manage` | Checks now and writes a plan for the current drift; opens it for review. Disabled when nothing drifted |
| **Accept drift in Git** | `reconciler.manage` | Git-managed watches only: rewrites the document to match what runs, pushes a branch and opens a pull request |
| **Delete** | `reconciler.manage` | Removes the watch and closes its open plans. The container is not touched. Disabled for a Git-managed watch; delete its document instead |

Checking is separate from acting on purpose. A drift check is safe to run at any time.

## Plans

| Action | Gated by | What it does |
|---|---|---|
| **Approve and apply** → **Apply now** | `reconciler.manage` | Checks reality again, then applies the plan. If the drift changed meanwhile, nothing is applied and the fresh plan is shown instead |
| **Reject** → **Reject plan** | `reconciler.manage` | Closes the plan, with an optional reason |

## Git sources

| Action | Gated by | What it does |
|---|---|---|
| **Add Git source** | `reconciler.manage` | A repository, branch and directory; synced once immediately |
| **Sync now** | `reconciler.manage` | Reads the branch now, whether or not it moved |
| **Delete** → **Delete source** | `reconciler.manage` | Stops reading from the repository. Its watches stay and become editable |

## Settings

| Action | Gated by | What it does |
|---|---|---|
| **Passes to confirm drift** → **Save** | `reconciler.manage` | How many consecutive checks must see drift before it counts |
| **Save enforcement** | `reconciler.enforce` | **Enforcement switched on**, **Changes per pass**, **Cooldown per watch (minutes)**, **Failures before stopping** |
| **Reset and resume** → **Resume enforcement** | `reconciler.enforce` | Shown while enforcement is stopped; resumes it |

## Confirmation

Everything that changes a container or a repository asks twice: approving a plan, enabling
**Enforce**, pushing a pull request branch, deleting a source, resuming enforcement. A
failure keeps the dialog open with the server's own message.

## Related

- [Reconciler](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Plans and enforcement](./plans-and-enforcement.md): what applying really does
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
