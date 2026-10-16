# Frontend Development

What to build a McHarbor screen from. The host frontend is React 19, Vite, TanStack Query,
Zustand, Tailwind 4 and Radix, with icons from `@tabler/icons-react`. The layout, the components,
the data hooks and the styling are centralised under `src/frontend/resources/` (`@resources`).
A module composes them and does not re-create them.

## Pages

| Page | Covers |
|---|---|
| [Layout](./layout.md) | The app shell, registering a route, the scroll contract, page headers, `TablePage`, the Volumes example, navigation entries, plugin pages, path aliases |
| [Components](./components.md) | The UI primitives in `@resources/components/ui`: buttons, form controls, feedback, dialogs, layout, data display, and the lint rules that enforce them |
| [Composites](./composites.md) | Tables and columns, batch actions, editors and schedules, charts, logs, shared dialogs, the terminal, the components the shell mounts |
| [Data and state](./data-and-state.md) | The API client and `assertSuccess`, queries and mutations, error toasts, the shared hooks, Zustand stores, permissions, i18n, utilities, test helpers |
| [Styling](./styling.md) | Semantic tokens, status and chart colours, control heights, no inline styles, themes, motion |
| [Lint and format](./tooling.md) | Vite+ with Oxlint and oxfmt: the commands, where the configuration lives, the rule set, the out-of-tree trees, and what the hooks and CI run |

## Where things live

```text
src/frontend/
├── core/          api client, auth + permissions, i18n + locales, router, plugin loader
├── resources/     @resources: everything shared
│   ├── layout/        AppLayout, PageShell, PageHeader, TablePage, sidebar-data
│   ├── components/
│   │   ├── ui/        the primitives
│   │   └── …          composites: data grid, editors, charts, dialogs, terminal
│   ├── hooks/         shared queries and behaviour hooks
│   ├── stores/        Zustand client state
│   ├── theme/         status, chart and token helpers
│   ├── utils/         cn, format, schedule, clipboard, download, api-mutation
│   └── types/
└── modules/<x>/   pages/, components/, hooks/, optional stores/
```

## Building a new page

1. **Route.** Export the page lazily from `core/router/lazyPages.ts` and add a `pageRoute(...)` in
   `core/router/appRoutes.tsx`, with the `data` or `page` handle.
   [Layout → Registering a page](./layout.md#registering-a-page)
2. **Navigation.** If the page needs a menu entry, add a `NavItem` with `permission: PERM.X_VIEW`
   to `sidebar-data.ts`, and a `nav.*` key to all six locales.
3. **Data.** Find the hook in `@resources/hooks`, or write one in `modules/<x>/hooks/`. The
   `queryFn` ends in `.then(assertSuccess)`, and the key includes the environment id.
   [Data and state → Queries](./data-and-state.md#queries)
4. **List or detail.** A list is a `TablePage`. A detail page uses `HeaderBreadcrumb`, `PageBody`
   and a `Card`. A settings page uses `SettingsShell`. [Layout](./layout.md)
5. **States.** A skeleton while loading, `<ErrorState>` on `isError`, `EmptyState` or `NoResults`
   when there is nothing to show, and `StatusError` for a row that failed.
6. **Actions.** Use `Button` / `ActionButton` / `HeaderActionButton` with `permission={PERM.…}`. A
   destructive action goes through `MutationConfirmDialog`. Name a mutation's success in
   `meta.success`, and leave the failure toast to the global cache.
7. **Text.** Use `useTranslation('<ns>')`, reuse `common` keys, add new keys to all six locales
   already translated (see [Data and state → i18n](./data-and-state.md#i18n)), and never use
   `defaultValue`.
8. **Style.** Use semantic tokens, layout primitives and Tailwind classes. No inline styles, no
   palette colours, no hex values.
9. **Size.** Keep a component under 300 lines of code. Extract the logic to a hook.
10. **Tests.** A destructive control, a shared primitive or pure logic owes its test, registered in
    `test/floor.test.ts` (`.claude/rules/test-floor.md`). Then run `npx vitest run <files>`,
    `npx tsc -b`, `npm run lint` and `npm run format:check`.

`modules/volumes` is the smallest complete example of all ten steps.

## Plugins

A plugin frontend is a separate UMD bundle. It loads the host's React and i18next at run time, but
everything else it imports is compiled into the bundle.

- A bundle whose Vite config has the `@resources` alias can use the primitives and `TablePage`.
- It then carries its own copy of `@tanstack/react-query` and `@tanstack/react-table`, and its own
  `QueryClientProvider`.
- A host hook or store imported into a plugin is a second copy, not the host's.
- Router hooks throw, because a plugin page has no router.
- Every root it creates goes through its own `guardedRoot(el)` (`mountGuard.tsx`), so a render crash
  shows "This plugin view failed to render." instead of a blank area. The host wraps the places it
  mounts plugin bundles in `PluginMountBoundary`; see [Components](./components.md#feedback-and-status).
- Plugins reach the host through `window.McharborHost` and their mount context.
- A bundle without the alias uses `PlainButton`-style markup with the documented lint exemption.
- The same lint rules apply to plugin frontends. See [Hooks → Frontend hook points](../hooks/README.md#frontend-hook-points) and
[Plugin development](../plugins/README.md).

## Related

- [Development workflow → Conventions](../workflow/README.md#conventions-that-hold-across-src)
- [Hooks](../hooks/README.md)
- [How-to → Widgets](../../how-to/widgets/README.md)
- `.claude/rules/react-components.md`, `.claude/rules/i18n-keys.md`, `.claude/rules/permissions.md`,
  `.claude/rules/error-handling.md`, the authoritative rules behind these pages
