# Upgrade Follow-On Roadmap

This document captures the larger product opportunities opened up by the recent major dependency upgrades that were not fully implemented in the current pass.

## 1. Typed i18n migration

Why:
- `i18next` v26 shifts toward the selector-based TypeScript API and better large-project typing.
- McHarbor has many translation namespaces plus per-node and per-widget locale bundles, so type-safe keys would reduce breakage during refactors.

Goals:
- Add `i18next.d.ts` resource typing for the bundled namespaces
- Enable selector-based `t()` usage for core app code
- Add missing-key CI checks
- Add translation extraction and validation workflow

Suggested steps:
1. Create `src/frontend/@types/i18next.d.ts` and type the static namespaces from `core/i18n/locales`.
2. Enable selector typing in `core/i18n/i18n.ts` after validating node/widget namespace merge behavior.
3. Migrate shared surfaces first:
   - `resources/components`
   - `resources/layout`
   - `modules/dashboard`
4. Add a lint/build check for missing translation keys.
5. Migrate nodes/widgets in batches.

Risk:
- Dynamic namespace merging for nodes/widgets will need careful typing to avoid fighting the current auto-loader design.

## 2. Search and filtering overhaul

Why:
- TypeScript 6 adds modern built-in types like `RegExp.escape`, making safer advanced search UI easier to ship.
- McHarbor has multiple list/detail screens that still rely on simple text matching.

Goals:
- Add exact, contains, and regex-safe search modes
- Add saved filters for logs, audit, activity, and app store
- Improve filtering consistency across list views

Suggested targets:
- `src/frontend/modules/logs/pages/LogsPage.tsx`
- `src/frontend/modules/activity/pages/ActivityPage.tsx`
- `src/frontend/modules/audit/pages/AuditPage.tsx`
- `src/frontend/resources/components/GlobalSearch.tsx`

Suggested steps:
1. Create a shared filter model in `resources/utils`.
2. Add safe regex compilation with `RegExp.escape` fallback behavior for literal searches.
3. Standardize list filter UIs on one shared toolbar/pattern.
4. Add local persistence for saved filters where it improves repeat workflows.

## 3. Deeper bundle splitting

Why:
- Vite 8 is now Rolldown-based and makes more aggressive chunking worthwhile.
- The current frontend build still has large workflow/editor/chart/code-editor chunks.

Goals:
- Reduce initial payload for dashboard and workflow-heavy routes
- Load expensive editors and chart modules only when needed

High-value targets:
- `src/frontend/modules/workflows/pages/WorkflowEditorPage.tsx`
- `src/frontend/modules/workflows/components`
- `src/frontend/resources/components/DataGrid.tsx`
- `src/frontend/resources/components/CodeEditor.tsx`
- `widgets/*` chart-heavy widgets

Suggested steps:
1. Split optional workflow subpanels and heavy editors behind route-local suspense boundaries.
2. Lazy-load code editor integrations only when code fields open.
3. Lazy-load charting widgets by widget type, not just route.
4. Revisit `vite.config.ts` chunking once the biggest dynamic-import boundaries are in place.

## 4. Rich timezone and schedule tooling

Why:
- The current pass adds next-run previews, but there is still room for stronger scheduling UX.
- TypeScript 6 adds built-in `Temporal` typing, which makes a better long-term time model realistic.

Goals:
- Show next 5 runs everywhere a cron schedule is edited
- Detect DST jumps and ambiguous times
- Add environment-aware timezone previews
- Add human-readable schedule summaries

Suggested targets:
- `src/frontend/modules/settings/components/UpdatesTab.tsx`
- `src/frontend/modules/settings/components/CreateScheduleDialog.tsx`
- `src/frontend/modules/environments/components/EnvironmentAutomationTab.tsx`
- `src/frontend/modules/workflows/components/CronField.tsx`

Suggested steps:
1. Reuse the new cron preview component in remaining schedule-related screens.
2. Add DST warning messages for schedules that land on skipped/duplicated local times.
3. Add environment timezone selection where schedule forms currently assume browser time.
4. Evaluate whether a `Temporal` polyfill is worth adopting for frontend-only time calculations.

## 5. Agent metrics and graph support

Why:
- Agent environments currently skip the background metrics collector to avoid congesting the single agent WebSocket transport.
- Docker stats through an agent can be slow, especially across multiple containers, so graphs and metric history are limited or blank for remote-agent environments.

Goals:
- Provide useful metrics and graphs for agent environments without blocking normal Docker operations.
- Make agent metric limitations visible in the UI instead of showing empty graphs without context.
- Keep polling lightweight, bounded, and safe for slower remote hosts.

Suggested targets:
- `src/backend/modules/metrics`
- `src/backend/modules/dashboard`
- `src/frontend/modules/environments/components/EnvironmentCard.tsx`
- `src/frontend/modules/dashboard`

Suggested steps:
1. Add an agent-specific metrics collection path with concurrent, bounded Docker stats polling.
2. Cache agent metric snapshots briefly to avoid repeated expensive calls from dashboard and environment cards.
3. Add per-environment controls for enabling/disabling agent metrics and tuning poll frequency.
4. Show a clear limited-metrics state when an agent is connected but metrics are disabled or unavailable.
5. Verify that metrics polling does not block container list, inspect, logs, moves, or direct-transfer operations.

Risk:
- Overly aggressive polling can saturate the agent transport and degrade normal management actions.

## 6. Auto-heal / healthcheck-based auto-restart

Why:
- One of the most-requested features in self-hosted Docker management (Portainer, Dockge, Yacht, Unraid, CasaOS, autoheal by RunTip42).
- Docker's built-in `RestartPolicy` only reacts to daemon-driven exits, not to containers that are alive but failing their healthcheck.
- A healthcheck-aware restarter is a natural fit next to the existing `updates` (Watchtower-style) module.

Goals:
- Automatically restart containers that fail their Docker healthcheck, with bounded backoff and cooldown.
- Per-container and per-environment opt-in/opt-out, with safe defaults that never restart protected containers.
- Surface a clear audit log entry and notification every time an auto-heal happens.

Suggested targets:
- `src/backend/modules/alerts` (runtime) — add an `AutoHeal` runner alongside the alert engine.
- `src/backend/core/audit` — log heal actions.
- `src/frontend/modules/containers/components` — add an `Auto-heal on healthcheck failure` toggle in the container settings/recreate dialogs.
- `src/frontend/modules/environments/components/EnvironmentCard.tsx` — environment-level enable/disable and last-heal indicator.

Suggested steps:
1. Add a lightweight background poller (cron or `robfig/cron`) that lists running containers with a healthcheck and inspects `State.Health.Status`.
2. On `unhealthy`, skip protected containers, verify the container was healthy at least once (avoid restart-loops on misconfigured healthchecks), then `container restart` with backoff.
3. Persist per-container auto-heal preference (DB or container label round-trip) so the setting survives recreation.
4. Audit-log every auto-heal and fire the existing notification channels.
5. Expose the toggle in the container settings UI and a per-environment default in the environment card.

Risk:
- A container with a permanently-failing healthcheck would restart in a tight loop without backoff; the cooldown window and "was-ever-healthy" guard are mandatory.

## 7. Git-based stack deployment (GitOps)

Why:
- The single most-requested feature in modern self-hosted Docker managers (Portainer, Dockge, Yacht, Coolify all have a flavor of it).
- McHarbor already has a `git` module and a `stacks` module; tying them together turns "edit compose and re-deploy" into "edit compose on GitHub and pull".
- Aligns naturally with the existing webhook and workflow trigger system — a push event can fire a workflow that redeploys a stack.

Goals:
- Bind a stack to a git repository (URL, ref, compose-file path, env-file path, optional private-key auth).
- Pull, diff, and apply on demand or on push via webhook.
- Store a snapshot per deployment so users can roll back to a known-good config.

Suggested targets:
- `src/backend/modules/git` (extend)
- `src/backend/modules/stacks`
- `src/frontend/modules/stacks/components` — new `GitSourceTab` per stack
- `src/frontend/modules/webhooks` — prebuilt `git-push` trigger

Suggested steps:
1. Add a `git_source` table (or per-stack label) holding URL, ref, compose path, env-file path, auth, and last-known SHA.
2. Backend `POST /stacks/{id}/pull` clones (shallow) the repo, computes a compose + env diff against the live deployment, and returns a plan (or applies it with a `dryRun` flag).
3. Hook the existing `webhooks` module to accept GitHub/GitLab/Gitea push payloads and trigger a pull+apply for the matching stack.
4. Store per-deploy snapshots in `$DATA_DIR/stacks/{id}/revisions/{sha}.{compose,env}.json` so rollbacks are O(1).
5. Frontend: render the diff, a "Last deployed: sha @ time" header, and a one-click `Pull & redeploy` / `Rollback to this revision` action.

Risk:
- Private-repo auth handling (deploy keys, fine-grained tokens) needs a clean secrets path; reuse the existing AES-256-GCM encryption.

## 8. Container dependency / topology graph view

Why:
- Frequently requested for visibility into multi-container setups (which container talks to which, which networks/volumes they share).
- Complements the per-resource list views and the move-container planner.

Goals:
- A single interactive graph per environment showing containers as nodes and shared networks/volumes/stack membership as edges.
- Click-through from a node to that container's detail page.
- Filter by stack, by network, or by "everything that depends on X".

Suggested targets:
- `src/frontend/modules/dashboard` (new widget type)
- `src/frontend/modules/environments/components/EnvironmentTopologyTab.tsx` (new)
- `src/backend/modules/containers` (extend list response with stack + network edges) and `src/backend/modules/stacks`

Suggested steps:
1. Extend the container list response with `StackName`, `NetworkNames`, and `VolumeNames` aggregates (or add a dedicated `/environments/{id}/topology` endpoint that returns `{ nodes, edges }`).
2. Pick a graph renderer (Recharts is already in use, but `reactflow`/`@xyflow/react` is the right tool for this; add as a dependency).
3. Layout by stack (subgraphs) with network edges colored by driver and volume edges as dashed lines.
4. Add a "Topology" tab on the environment detail page and a small topology widget for the dashboard.
5. Right-click a node → jump to container detail, logs, terminal, or stats.

Risk:
- Very large environments (hundreds of containers) need force-directed layout performance work; consider virtualization and edge culling.

## 9. Stack revision history and rollback

Why:
- Universal ask in any tool that deploys Compose (Portainer, Dockge, Yacht).
- McHarbor's stack module already tracks `compose` and `env` blobs; storing an append-only history is a small extension with very high value.

Goals:
- Every stack create/update writes an immutable revision (compose + env + metadata + author + note).
- UI lists revisions, shows a structured diff against the live config, and offers one-click rollback.
- Rollback goes through the existing move/apply path so all safety checks still apply.

Suggested targets:
- `src/backend/modules/stacks`
- `src/frontend/modules/stacks/components` — new `RevisionsTab`
- `src/frontend/modules/stacks/pages/StackDetailPage.tsx`

Suggested steps:
1. Add a `stack_revisions` table (id, stack_id, sha/hash, author, note, compose, env, created_at) and write to it on every apply.
2. Add `GET /stacks/{id}/revisions` and `GET /stacks/{id}/revisions/{revId}/diff` and `POST /stacks/{id}/revisions/{revId}/rollback`.
3. Frontend: a revisions list with diff highlighting, a rollback button with the same confirmation dialog as a redeploy, and a "tag as known-good" toggle.
4. Cap revisions per stack (e.g., 50) with an opt-in "keep forever" flag; prune the rest on a schedule.

Risk:
- Rollback must re-run dependent container start order; reuse the existing Compose deploy path rather than reimplementing apply logic.

## 10. Host system metrics and management

Why:
- Per-container metrics are good, but operators also need to see the host itself: CPU, memory, disk, load, network throughput, Docker daemon health, prune opportunities.
- Especially important for agent environments where the host may be a Pi, NAS, or remote VPS with limited resources.

Goals:
- A first-class "Host" page (or environment card section) showing live host metrics, top-N consumers, and Docker daemon status.
- One-click actions: `docker system prune`, `docker builder prune`, `docker volume prune` (with the existing confirm dialog).
- Reuse the existing agent transport safely so the page works for local and remote environments.

Suggested targets:
- `src/backend/modules/docker_info` (extend) and `src/backend/modules/metrics`
- `src/frontend/modules/environments/components/EnvironmentCard.tsx`
- `src/frontend/modules/dashboard` (new host metrics widget)
- A new `src/frontend/modules/host` module (Host detail page)

Suggested steps:
1. Backend: add `GET /environments/{id}/host` returning CPU%, mem%, disk usage, load, network bytes/s, Docker version, and prune-eligible bytes (system, builder, volumes).
2. Use the same bounded-polling pattern as the existing `metrics` module for agent environments; cache for ~5s on the server.
3. Frontend: a Host page with a stat grid, a top-N containers-by-CPU/mem list, and a "Prune" actions panel.
4. Add per-environment permissions (`PermHostView`, `PermHostManage`) so non-admin users can see metrics but not run prunes.

Risk:
- `docker system prune` is destructive; require a typed-confirmation and an explicit `--volumes` opt-in (default off).

## 11. Image registry browser

Why:
- Operators regularly need to find a tag, check digests, or compare image sizes without pulling gigabytes.
- The `registry` module already exists for credentials; a read-only browser is a natural extension.

Goals:
- Browse tags for any configured registry or any public registry (Docker Hub, GHCR, Quay, Gitea, custom).
- Show tag digest, size, last-updated, OS/arch, and a one-click `Pull to environment X` action.
- Optionally compare two tags side-by-side (size, layers).

Suggested targets:
- `src/backend/modules/registry` (extend) and `src/backend/modules/images`
- `src/frontend/modules/images/components` (new `RegistryBrowserDialog`)
- `src/frontend/modules/registries` if it doesn't exist

Suggested steps:
1. Add `GET /registries/{id}/repositories` and `GET /registries/{id}/repositories/{name}/tags` using the appropriate `go-containerregistry` or HTTP calls.
2. Add a `RegistryBrowser` button in the Images page header; open a dialog with a search box, repository list, and tag list.
3. Add `Pull` (calls the existing pull endpoint) and `Inspect manifest` actions per tag.
4. Cache catalog/tag responses for ~5 minutes per registry; respect rate limits.

Risk:
- Public registries rate-limit aggressively; client-side throttling and clear error messaging are required.

## Suggested order

1. Typed i18n migration for shared surfaces
2. Search and filtering overhaul
3. Auto-heal / healthcheck-based auto-restart (small, high-value win)
4. Host system metrics and management (unlocks the rest of the agent story)
5. Stack revision history and rollback (foundational for safe GitOps)
6. Git-based stack deployment (GitOps)
7. Image registry browser
8. Deeper bundle splitting for editor/chart/code paths
9. Rich timezone and schedule tooling follow-up
10. Agent metrics and graph support
11. Container dependency / topology graph view
