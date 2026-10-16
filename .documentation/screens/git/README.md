# Git

`/git`

Git repositories linked to McHarbor, so a Compose stack can come from a repository rather
than from a pasted file.

## Permissions

| Key | What it unlocks |
|---|---|
| `git.view` | See linked repositories |
| `git.manage` | Add, sync and remove them |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per repository: name, URL, branch, status and last synced.
- Status as **synced**, **pending** or **error**, with the last successful sync time kept so you can see how long it has been broken.

## What this gives you

A repository on its own changes nothing. It becomes useful in two places: **stacks**
deployed from a repository path rather than a pasted file, and
[GitOps](../gitops/README.md) pipelines that watch a repository and deploy on change.

Git is the source of truth in that arrangement: editing such a stack’s Compose file in the
interface is overwritten on the next sync.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [GitOps](../gitops/README.md): pipelines, approvals and previews
- [Stacks](../stacks/README.md): what gets deployed
