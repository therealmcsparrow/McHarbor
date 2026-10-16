# GitOps

`/gitops`

Pipelines that promote a commit across environments (dev to staging to production), with
the approvals you choose to require.

## Permissions

| Key | What it unlocks |
|---|---|
| `gitops.view` | See pipelines, deployments and approvals |
| `gitops.manage` | Create and change pipelines |
| `gitops.approve` | Approve or reject a promotion |
| `gitops.delete` | Delete a pipeline |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per pipeline: name, repository, stages, trigger, enabled and updated.
- Deployment history: stage, commit, status, trigger, started and finished.
- Pending approvals, and PR preview environments where the pipeline uses them.

## Why the approve key is separate

`gitops.approve` exists so the person who can change a pipeline and the person who can let
a change into production do not have to be the same person.

## Stages

A pipeline is an ordered list of stages, each an environment plus how to get there: stage
name, order, target environment, watched branch, Compose path, deploy mode
(**Auto**, **Manual**, **PR preview**), and whether a promotion into it requires approval.

The usual shape is dev on auto, staging on auto, production requiring approval.

## PR previews

A pipeline in PR-preview mode stands up a throwaway environment per pull request and tears
it down when the PR closes. This needs a webhook on your forge; the page gives you the
webhook URL and an optional secret, and GitHub, GitLab and Gitea are all supported callers.
Without the webhook, previews never open and never close.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Git](../git/README.md): the repositories pipelines draw from
- [Stacks](../stacks/README.md): what a stage actually deploys
- [Audit](../audit/README.md): where approvals and rejections are recorded
