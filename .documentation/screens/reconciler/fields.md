# Reconciler: Fields

## Watch list columns

| Column | What it is |
|---|---|
| Name | Your label, with a **Git** badge when a Git source owns the watch (hover for the file) |
| Response | **Observe**, **Propose a fix** or **Enforce** |
| Container | What it watches: the container, the stack, or **Whole environment** |
| Image | The image reference a container should run |
| Status | Current reconciliation state |
| Last Reconciled | Or "Never" |
| Drift | The drift classes; click for field, expected, actual and first seen |

## Add Desired State

| Field | Type | What it is |
|---|---|---|
| Name | text | Your label |
| What to watch | select | **One container**, **A stack** or **Everything in this environment** |
| Container Name, Image Reference | text | For one container |
| Stack | select | For a stack |
| Desired Status | select | **Running**, **Stopped** or **Removed** |
| When it drifts | select | **Observe** or **Propose a fix**. **Enforce** is set afterwards, under its own permission |

"Removed" is a legitimate desired state: it is how you assert that something must *not*
be running, and have that checked continuously rather than remembered.

## Plans

| Column | What it is |
|---|---|
| Watch | Opens the plan |
| Status | **Waiting for review**, **Applying**, **Applied**, **Failed**, **Rejected** or **Superseded**; a failed plan's reason sits behind the icon beside it |
| From | **Proposal** or **Enforcement** |
| Steps | What the plan does |
| Written, Resolved by | When, and who approved or rejected it (`system:enforce` for enforcement) |

Filter: **Status**, defaulting to **Waiting for review**.

## Git sources

| Field | What it is |
|---|---|
| Name | Your label |
| Repository | One of the repositories under **Git** |
| Branch | Default `main` |
| Directory | Where the documents are, relative to the repository root |
| State | **Syncing**, **Paused** or **Failing**, with the failure's reason behind the icon |
| Watches, Commit, Last sync | What the last good sync produced |

## Settings

| Field | Range | Default |
|---|---|---|
| Passes to confirm drift | 1 to 10 | 2 |
| Enforcement switched on | on/off | off |
| Changes per pass | 1 to 25 | 3 |
| Cooldown per watch (minutes) | 0 to 1440 | 10 |
| Failures before stopping | 1 to 20 | 3 |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Reconciler](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
