# Webhook secrets

Every GitOps repository signs its pull-request webhook deliveries with a secret. McHarbor
verifies an HMAC-SHA256 over the raw request body against that secret before it reads the
payload, so a repository without a secret cannot receive deliveries at all.

Permission keys on this page: `gitops.view`, `gitops.manage`.

## Where to find it

**GitOps** page, right-hand column, section **Webhook secrets**. One row per repository, with
a badge (**Configured** or **Not set**) and the controls below. The section is hidden when no
repository exists.

| Control | Does | Needs |
|---|---|---|
| **Generate** (no secret yet) | Mints a secret and shows it once | `gitops.manage` |
| **Rotate** (secret exists) | Asks for confirmation, mints a new secret and shows it once | `gitops.manage` |
| Remove (trash icon) | Asks for confirmation, then deletes the secret | `gitops.manage` |

A control you lack the key for is shown disabled, and its tooltip names `gitops.manage`. The
badge is read with `gitops.view`.

## The secret is shown once

Generate and Rotate open a dialog titled **Webhook secret for <repository>**. The value is
masked; **Show secret** reveals it and **Copy secret** copies it (on a plain-HTTP page the
browser blocks the clipboard, so a manual-copy dialog opens instead). Closing the dialog
discards the value. McHarbor stores the secret encrypted and no later response contains it,
so a lost secret is rotated, never recovered.

## Setting it in the Git provider

| Provider | Webhook URL | Where the secret goes |
|---|---|---|
| GitHub | `https://<host>/api/webhooks/gitops/pr?repo=<repoId>` | The webhook's **Secret** field; McHarbor checks `X-Hub-Signature-256` |
| GitLab | same | The webhook's **Secret token** field; McHarbor checks `X-Gitlab-Token` |
| Gitea | same | The webhook's **Secret** field; McHarbor checks `X-Hub-Signature-256` or `X-Gitea-Signature` |

Send the pull-request events. See [Ephemeral preview environments](../previews/README.md) for
what a delivery does.

## What rotating and removing do

- **Rotate** replaces the secret at once. Deliveries signed with the old value are refused
  (401) until the webhook at the provider holds the new one.
- **Remove** leaves the repository with no secret. Every delivery is refused until you
  generate a new one.

Both are audited (`gitops.webhook_secret.generated`, `gitops.webhook_secret.cleared`, warning
severity, entity type `git_repo`).

## From a terminal

```bash
mcharbor gitops webhook-secret show <repo-id>
mcharbor gitops webhook-secret rotate <repo-id> --yes
mcharbor gitops webhook-secret remove <repo-id> --confirm <repo-id>
```

`rotate` prints only the secret on stdout, so it can be piped into a secret store; the
show-once notice goes to stderr. `remove` is irreversible and needs `--confirm` with the
repository id. See the [command reference](../../cli/reference/mcharbor_gitops_webhook-secret.md).

## API

| Method | Path | Permission | Returns |
|---|---|---|---|
| `GET` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.view` | `{repoId, configured}` |
| `POST` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.manage` | `201 {repoId, secret}`, the only response that carries a secret |
| `DELETE` | `/api/gitops/repos/{repoId}/webhook-secret` | `gitops.manage` | `204` |

## When it goes wrong

| Symptom | Cause |
|---|---|
| The provider shows 401 for every delivery | The repository has no secret, or it was rotated and the provider still holds the old one |
| Generate answers an error naming the repository | The repository id does not exist |
| The badge shows an error instead | The status read failed; the server's message is behind the error icon |
