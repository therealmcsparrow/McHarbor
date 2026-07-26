---
description: McHarbor's 6-environment promotion pipeline (dev -> test -> alpha -> beta -> release -> main). Use when the user asks about environment promotion, deployment workflow, branching strategy, release process, CHANGELOG, version bumping, or how to move code between environments.
---

# McHarbor Development Process

This skill documents how McHarbor code flows from a developer's
laptop to a production release. The full reference is at
`.documentation/development/environment-pipeline.md`; this file
captures the operational rules you need to act.

## The six environments

| Env | Branch | Image tag | When to use it |
|---|---|---|---|
| dev | `dev` | `:dev` | Inner loop. Push here to publish an image for local testing. |
| test | `test` | `:test` | QA team smoke tests. Version lockstep enforced. |
| alpha | `alpha` | `:alpha` | Early-access users. |
| beta | `beta` | `:beta` | Paid-pilot regression soak. |
| release | `release` | `:release` | Release managers cut a `Release X.Y.Z` commit here. CHANGELOG gate enforced. |
| main | `main` | `:X.Y.Z`, `:latest` | Production. Tag and GH release on every merge. |

## The chain

```
dev  ->  test  ->  alpha  ->  beta  ->  release  ->  main
```

Promotions are **never automatic**. A human triggers the **Promote**
workflow (`.github/workflows/promote.yml`), which opens a PR from
the source commit to the next branch in the chain. CI runs on the
PR. The PR is reviewed and merged, which triggers the target
branch's own workflow, which publishes the next image.

## What you, the model, should do

### When the user asks to "promote X to Y"

1. Verify the chain is valid:
   - dev -> test
   - test -> alpha
   - alpha -> beta
   - beta -> release
   - release -> main
   - Any other direction is rejected. Tell the user.

2. Verify the SHA is reachable. Use `git log origin/<source> --oneline -5`
   to show recent commits and ask the user which one.

3. Run the workflow:
   ```bash
   gh workflow run promote.yml \
     --field source=<source> \
     --field sha=<commit-sha> \
     --ref main
   ```
   (Or use `gh workflow run promote.yml --json` to confirm the input
   types match.) Without `gh` auth, build the PR via git:
   ```bash
   git fetch origin <source>
   git checkout -b promote/<source>-to-<target>/<short-sha> <sha>
   git push origin HEAD
   gh pr create --base <target> --head promote/<source>-to-<target>/<short-sha> \
     --title "Promote <source> -> <target> (<short-sha>)" \
     --body "Auto-generated promotion PR"
   ```

4. After the PR is opened, tell the user to review and merge. After
   merge, the target branch's workflow runs.

### When the user asks to cut a release

1. Confirm we're on `release`.
2. Bump three files in lockstep:
   - `VERSION` (no newline, e.g. `2.0.8`)
   - `src/agent/agent.go` `const agentVersion = "2.0.8"`
   - `src/frontend/package.json` `"version": "2.0.8"`
3. Add a CHANGELOG entry at the top of the file:
   ```markdown
   ## [2.0.8] - 2026-07-26

   ### Added
   - ...

   ### Fixed
   - ...
   ```
4. Commit with message `Release 2.0.8` (exactly that text — the prod
   workflow parses it to detect a release).
5. Push to `release`. The `release.yml` workflow builds and tags
   `:2.0.8` and `:release`.
6. Promote `release -> main` via the Promote workflow.
7. After the prod workflow runs, confirm:
   - The image is at `ghcr.io/therealmcsparrow/mcharbor:2.0.8`
   - The GH release exists with the CHANGELOG entry as body
   - The git tag `v2.0.8` exists

### When the user asks "is the build green?"

Run all the environment workflows in parallel using `gh`:
```bash
gh run list --workflow "Dev" --limit 1
gh run list --workflow "Test" --limit 1
gh run list --workflow "Alpha" --limit 1
gh run list --workflow "Beta" --limit 1
gh run list --workflow "Release" --limit 1
gh run list --workflow "Prod" --limit 1
```
Or use the GitHub UI. Without `gh` auth, fall back to `git log
origin/<branch>` and look for the green check on the latest commit.

### When the user asks "what's in test right now?"

```bash
git log origin/test --oneline -10
docker pull ghcr.io/therealmcsparrow/mcharbor:test
```

The `:test` tag is mutable (always the latest test build); the
`:test-<sha>` tag is immutable (one per commit).

## Conventions

- **Commit messages** follow Conventional Commits where possible:
  `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`.
  Releases use a single line: `Release X.Y.Z`.
- **PR titles** mirror the commit message. No "WIP" in titles; mark
  drafts with the GitHub Draft PR feature instead.
- **One concern per PR.** Don't bundle a refactor with a feature
  unless the refactor is required to land the feature.
- **Never force-push to** `test`, `alpha`, `beta`, `release`, or `main`.
  `dev` allows force-push for rebase hygiene.
- **Never delete** `test`, `alpha`, `beta`, `release`, or `main`.

## What you should NOT do

- Don't cut a release without a CHANGELOG entry. The release workflow
  fails on missing entries, but you should also fail the human
  process before pushing.
- Don't promote across more than one stage. If you need to go from
  `test` to `release`, do `test` -> `alpha` first, then `alpha` -> `beta`,
  then `beta` -> `release`. Each promotion gets its own review.
- Don't skip environments. Every commit must walk the full chain.
  The exception is hotfixes: see the hotfix section below.

## Hotfixes

For urgent production fixes:

1. Branch from `main`, not from `release`:
   ```bash
   git checkout main
   git checkout -b hotfix/critical-bug
   ```
2. Make the fix, commit, push. The CI runs.
3. Open a PR directly into `main`. The PR body must include a
   **Why this skipped the chain** section explaining the
   justification.
4. After the PR merges, **back-merge** to the active chain:
   ```bash
   git checkout release
   git merge --no-ff origin/main
   git push origin release
   ```
   Repeat for `beta`, `alpha`, `test`, `dev` so the chain catches up.
5. The prod workflow tags the hotfix as `vX.Y.Z+1` automatically.
6. Add a CHANGELOG entry retroactively if the hotfix was urgent
   enough to skip docs.

## See also

- `.documentation/development/environment-pipeline.md` — the full
  pipeline reference, including the promotion flow diagram and the
  rollback procedure.
- `CODEOWNERS` — who reviews what.
- `.github/workflows/promote.yml` — the promote workflow definition.
- `AGENTS.md` — McHarbor's general agent instructions.
