# McHarbor Environment Pipeline

This document describes the 6-environment promotion chain used by
McHarbor. Every change flows through the same pipeline:

```
dev  ->  test  ->  alpha  ->  beta  ->  release  ->  main (prod)
```

Each environment has its own:

- **Branch** with the same name
- **GitHub Actions workflow** at `.github/workflows/<env>.yml`
- **Container image tag** at `ghcr.io/therealmcsparrow/mcharbor:<env>`
- **Container image tag** at `ghcr.io/therealmcsparrow/mcharbor:<env>-<sha>`
- **Agent image tag** at `ghcr.io/therealmcsparrow/mcharbor-agent:<env>`
- **Slack / Discord channel** (operated by the matching GitHub team)

## Environment reference

| Env | Branch | Image tag | Audience | Gates | Deploy |
|---|---|---|---|---|---|
| **dev** | `dev` | `:dev` | Core maintainers | Build, vet, tsc | Auto on push |
| **test** | `test` | `:test` | QA team | Build, vet, tsc, version lockstep | Auto on push + webhook |
| **alpha** | `alpha` | `:alpha` | Early-access users | Build, vet, tsc, version lockstep | Auto on push |
| **beta** | `beta` | `:beta` | Paid pilots, soak tests | Build, vet, tsc, version lockstep | Auto on push |
| **release** | `release` | `:release`, `:X.Y.Z` | Release managers | Build, vet, tsc, version lockstep, **CHANGELOG gate** | Auto on push |
| **prod** (main) | `main` | `:X.Y.Z`, `:latest` | Everyone | Build, vet, tsc, version lockstep, **CHANGELOG gate**, **GH release** | Auto on push (after PR) |

## Promotion workflow

Promotions are explicit, not automatic. Use the **Promote** workflow
(`.github/workflows/promote.yml`):

1. Go to **Actions → Promote → Run workflow**
2. Select:
   - **Source**: the current environment of the commit
   - **SHA**: the commit to promote (defaults to HEAD of source)
   - **dry_run**: false (or true to preview)
3. The workflow opens a **PR** from the source's commit SHA to the
   next branch in the chain. This keeps the promotion reviewable.
4. After CI passes and the PR is merged, the target branch's
   workflow runs and publishes the new image.

The chain is enforced at the workflow level. There is no automatic
dev → test promotion — a human reviews the diff first.

## Versioning

Three sources of truth must stay in lockstep:

- `VERSION` (no newline)
- `src/agent/agent.go` `const agentVersion = "X.Y.Z"`
- `src/frontend/package.json` `"version": "X.Y.Z"`

Every workflow verifies this. If they drift, the build fails.

**Bumping a version** is part of the `release` → `main` PR. The
changelog entry is added in the same commit. The `prod` workflow
tags the merged commit as `vX.Y.Z` automatically.

## Required GitHub settings

These should be configured in the repository settings. They are
documented here because they cannot be enforced by code in the repo.

### Branch protection

| Branch | Require PR | Require approvals | Require status checks | Block force push | Block deletion |
|---|---|---|---|---|---|
| `dev` | ❌ (fast inner loop) | ❌ | ❌ | ❌ | ❌ |
| `test` | ✅ | 1 | ✅ | ✅ | ❌ |
| `alpha` | ✅ | 1 | ✅ | ✅ | ❌ |
| `beta` | ✅ | 1 | ✅ | ✅ | ❌ |
| `release` | ✅ | 2 | ✅ | ✅ | ❌ |
| `main` | ✅ | 2 | ✅ | ✅ | ❌ |

### Required status checks (per branch)

| Branch | Required check |
|---|---|
| `test` | `Build & verify / Go build` |
| `alpha` | `Build & verify / Go build` |
| `beta` | `Build & verify / Go build` |
| `release` | `Build & verify / CHANGELOG entry` |
| `main` | `Build & verify / Verify CHANGELOG entry` |

### Repository secrets

| Secret | Used by | Purpose |
|---|---|---|
| `GHCR_PAT` | All publish jobs | Push to GitHub Container Registry (falls back to `GITHUB_TOKEN` if absent) |
| `TEST_DEPLOY_WEBHOOK` | `test.yml` | Calls the test cluster's pull endpoint after the image is published |

## Local development

The recommended inner loop for a maintainer:

```bash
# 1. Branch from dev (never from main)
git checkout dev
git checkout -b feature/my-change

# 2. Iterate, then push to dev to publish the :dev image
git commit -m "feat: my change"
git push origin feature/my-change
# open a PR into dev; CI runs

# 3. Once merged into dev, the :dev image is published.
#    Pull it into a local cluster to test:
docker pull ghcr.io/therealmcsparrow/mcharbor:dev
docker compose -f docker-compose.dev.yml up -d

# 4. When ready, promote dev -> test via the Promote workflow.
#    A PR is opened; review it, merge it, CI publishes :test.

# 5. Repeat for alpha, beta, release.

# 6. Cutting a release:
#    - on the release branch, update VERSION + agentVersion + package.json
#    - add a CHANGELOG entry under ## [X.Y.Z]
#    - commit with message "Release X.Y.Z"
#    - promote release -> main
#    - the prod workflow creates the GH release and the vX.Y.Z tag
```

## Auditing

Every promotion leaves a record in:

- **GitHub Actions** (Actions tab) — workflow run, logs, and PR link
- **Pull Requests** (Pull requests tab) — promotion PR with source, target, SHA
- **git log** — promotion merge commit on the target branch
- **GHCR** — `:env-<sha>` tag preserves the exact image that was promoted

## Rollback

Each environment's image is tagged with both `:env` (mutable, latest
build) and `:env-<sha>` (immutable). To roll back:

```bash
# On the cluster, pull the previous SHA-pinned image
docker pull ghcr.io/therealmcsparrow/mcharbor:test-abc1234
docker compose -f docker-compose.test.yml up -d
```

For a code-level rollback, force a promotion of an older commit
through the chain using the Promote workflow's `sha` input.
