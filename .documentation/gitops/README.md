# GitOps

Pipelines that promote a commit from one environment to the next, with approvals, and
ephemeral preview environments per pull request.

| Page | Covers |
|---|---|
| [Promotions and deploys](./promotions/README.md) | What a promotion does: the checkout, the stack it deploys, what it records, how a failure reaches you |
| [Ephemeral preview environments](./previews/README.md) | PR previews: webhook authentication, TTL, quotas, DNS, deploy, cleanup, the forge probe, permission keys |
| [Webhook secrets](./webhook-secret/README.md) | The per-repository secret that authenticates PR webhooks: generate, rotate, remove, show-once, provider setup, CLI |

Permission keys across this section: `gitops.view`, `gitops.manage`, `gitops.approve`,
`gitops.delete`, and `environments.manage` for the preview provisioning path.
