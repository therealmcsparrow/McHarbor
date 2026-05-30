# Upgrade Follow-On Roadmap

This document captures the larger product opportunities opened up by the recent major dependency upgrades that were not fully implemented in the current pass.

## Implemented in this pass

- Cron schedule previews for workflow cron fields and settings schedule forms
- Targeted lazy loading for the dashboard widget picker/grid and workflow editor side panels

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

## Suggested order

1. Typed i18n migration for shared surfaces
2. Search and filtering overhaul
3. Deeper bundle splitting for editor/chart/code paths
4. Rich timezone and schedule tooling follow-up
