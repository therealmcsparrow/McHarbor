# Demo mode

**Settings → System → Demo mode** switches McHarbor onto a separate SQLite database,
`demo.db`, filled with a simulated estate. Every environment in it is served by an
in-process simulated agent, so nothing real is read, changed or contacted.

| Action | Route | Permission |
|---|---|---|
| Read the status (banner, settings card) | `GET /api/demo/status` | any signed-in user |
| Enter demo mode | `POST /api/demo/enter` | `system.demo` (global) |
| Exit demo mode | `POST /api/demo/exit` | `system.demo` (global) |
| Reset the demo data | `POST /api/demo/reset` | `system.demo` (global) |

Entering, exiting and resetting restart McHarbor, because the database is opened once
at boot. Demo mode is refused on a clustered instance (`MCHARBOR_HA_MODE` set).

## Where the data lives

| File in `DATA_DIR` | What it is |
|---|---|
| `demo.json` | The marker. Its presence at boot is what puts the instance in demo mode. |
| `demo.db` | The demo database. Kept on exit, so the next visit starts where the last ended. |
| `demo.parked.json` | The marker of an exited demo, reused when you enter again with the same sizes. |
| `demo/stacks/` | Compose files of demo stacks. Real stacks live in `stacks/` and are never shared. |

Your accounts, sessions, roles and installed plugins are copied from the real database
into `demo.db`, so everyone can still sign in. Nothing is ever written back.

## What the estate contains

The sizes are chosen when entering or resetting: environments, containers, virtual
machines, LXC system containers, Kubernetes clusters and deployments, Swarm clusters,
nodes and services, and how many days of history (audit log, workflow runs, schedule
runs, alerts, incidents) are generated.

### Git and GitOps

Every demo also carries a fixed Git and GitOps story, written once per `demo.db`:

**Git** (the Git page, `git.view`)

| Repository | Forge | Branch | State |
|---|---|---|---|
| `storefront` | GitHub | `main` | Synced a few minutes ago; token credential; webhook secret set |
| `platform-infra` | GitLab | `main` | Last sync **failed** with an authentication error; basic credential; webhook secret set |
| `homelab-compose` | Gitea | `main` | Synced, no credential, no webhook secret |
| `edge-pos` | GitHub | `release/2026.09` | Synced; token credential; webhook secret set |

Each repository has a sync history with full commit SHAs. The credentials are demo
placeholders that no forge accepts; they and the webhook secrets are stored encrypted,
exactly as real ones are.

**GitOps** (the GitOps page, `gitops.view`)

| Pipeline | Trigger | Stages |
|---|---|---|
| Storefront | auto | dev → staging → production (production needs an approval) |
| Storefront PR previews | pr_preview | one ephemeral environment per pull request |
| Observability stack | manual | dev → staging → prod (prod needs an approval) |
| Edge POS rollout | auto | pilot → wave-1 → wave-2 (each wave needs an approval) |

Stages point at the demo environments by role (Development, Staging EU, Production EU 1,
the edge stores); a smaller estate falls back to the environments it has. The history
holds succeeded, failed and pending promotions, approvals that are pending, approved and
rejected, and four pull-request previews: one **active** (its own environment, stack and
URL), one **expired**, one torn down after a merge, and one that **failed** to deploy with
its reason behind the error icon.

Stacks the promotions deployed appear under Stacks with names such as
`gitops-storefront-dev`, owned by their pipeline stage, and their containers run on the
simulated hosts.

Approving, rejecting and destroying work as usual and persist across restarts. A restart
does not rewrite the story; **Reset demo data** does.

### Secrets vault

**Security → Secrets** (`secrets.view`) holds up to sixteen credentials the demo stacks
plausibly use, written once per `demo.db`:

| Scope | Secrets |
|---|---|
| Global | `REGISTRY_TOKEN`, `SMTP_PASSWORD`, `SLACK_WEBHOOK_URL`, `OIDC_CLIENT_SECRET`, `WILDCARD_TLS_KEY`, `WILDCARD_TLS_CERT`, `DEPLOY_SSH_KEY`, `LOKI_GCS_CREDENTIALS`, `LEGACY_FTP_PASSWORD` |
| Environment | `PAGERDUTY_ROUTING_KEY` (first environment), `BACKUP_S3_SECRET_KEY` (second) |
| Stack | `POSTGRES_PASSWORD` (shop), `STRIPE_API_KEY` and `LEDGER_DB_PASSWORD` (payments), `GRAFANA_ADMIN_PASSWORD` (observability), `RABBITMQ_DEFAULT_PASS` (messaging) |

A secret scoped to an environment or a stack the estate does not have is left out, so a
small estate carries fewer. Each secret has a version history, and its references point at
real demo stacks, at most three stacks per template, so a large estate does not bury the
references panel. The set deliberately contains what an operator should notice:
`REGISTRY_TOKEN`, `WILDCARD_TLS_KEY` and `STRIPE_API_KEY` each have one **stale**
reference still on the previous version, `LEGACY_FTP_PASSWORD` is used by nothing, and
several have never been revealed.

Values are not in the source. Each one is derived from the demo marker's own secret and
shaped like its kind (a password, a `ghp_` token, a PEM key), then encrypted at rest like
any real secret. Revealing one still needs `secrets.reveal` and a step-up window, and a
**Reset demo data** mints a new set.

## What demo mode keeps offline

Nothing in demo mode contacts a real system:

- Environments of any type other than the simulated agents are refused.
- The GitOps poller and preview reaper do not run, so seeded previews and pending
  promotions stay as they are.
- Every GitOps clone, branch lookup, pull-request probe and pull-request comment is
  refused before it leaves McHarbor. A promotion started in demo mode is recorded as
  failed with "demo mode simulates every Git forge, so nothing was fetched from or sent
  to it".
- **Sync** on the Git page does not contact the forge: a healthy repository lands on its
  last deployed commit, and `platform-infra` fails again with the same error.
- Other modules cannot reach a repository's remote through the Git module either (the
  reconciler's Git sources included).
- McHarbor's own host services and host updates, self-update, instance backups, database
  migration, licence, storage locations, TLS, time zone and backup key settings, and
  plugin install and uninstall are refused.

## What goes wrong

| Symptom | Cause |
|---|---|
| A promotion fails at once with "demo mode simulates every Git forge" | Expected: demo mode never fetches a compose file from a forge |
| The active preview's environment shows `agent` rather than `socket` | Demo environments are all simulated agents; a real preview environment is a socket environment |
| A preview destroyed in the demo does not come back after a restart | Expected: the demo keeps what you did; use **Reset demo data** to start over |
| Git and GitOps are empty in a demo built before this release | The story is written at the next boot of that `demo.db`; if it is still empty, reset the demo data |
| The Secrets page is empty in an older demo | The vault is seeded at the next boot of that `demo.db`; the log says `demo: vault secrets not seeded` with the reason if it could not be |

## Related

- [System](../README.md)
- [GitOps](../../../gitops/README.md)
