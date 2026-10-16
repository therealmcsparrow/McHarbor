# Frontend: Layout

How a page sits inside McHarbor's shell, how it is registered, and which layout pieces it is
built from. All paths are under `src/frontend/`.

## The shell

Every signed-in page renders inside `AppLayout` (`resources/layout/AppLayout.tsx`). A module
page never draws its own sidebar, topbar or outer padding.

```text
AppLayout
├── Aurora                      background behind the glass panels
├── Sidebar                     left navigation, built from sidebar-data.ts + plugin menus
└── column
    ├── Header                  topbar: #header-slot (breadcrumb) and #header-right-slot
    ├── ShellSlot afterHeader
    ├── #pagehead-slot          the page-header band portals here
    ├── ShellSlot banners       update, licence, standby, encryption-key, demo banners
    ├── main
    │   └── PageShell           scroll mode, gutter and optional right sidebar from the route
    │       └── <Outlet/>       your page
    └── Footer                  status bar
```

`AppLayout` also handles the auth redirect and the redirect to `/setup`. It starts the plugin
loader and registers the global shortcuts and the Docker event stream. Banners and bridges are
registered with `registerShellSlot(slot, id, component)` from `resources/layout/shellSlots.tsx`.
The registrations live in `core/router/shellWiring.tsx`, so you never mount one from a page.

Sign-in, setup and password-reset pages use `AuthLayout` instead.

## Registering a page

A page is a **default export** under `modules/<module>/pages/`. Registering it takes two lines:

1. A lazy export in `core/router/lazyPages.ts`:

   ```ts
   export const VolumesPage = lazy(() => import("@modules/volumes/pages/VolumesPage"));
   ```

2. A route in `core/router/appRoutes.tsx`:

   ```ts
   pageRoute("volumes", P.VolumesPage, data),
   pageRoute("volumes/:name", P.VolumeDetailPage, pageFlush),
   ```

The third argument is the route **handle**, which tells the shell how to lay the page out:

| Preset | Handle | Use it for |
|---|---|---|
| `data` | `{ scroll: "data" }` | Lists and tables. The shell does not scroll, and the table scrolls inside itself |
| `page` | `{ scroll: "page" }` | Settings and long forms. The whole page scrolls as one canvas |
| `dataFlush` | `{ scroll: "data", padded: false }` | A data page that draws its own gutter |
| `pageFlush` | `{ scroll: "page", padded: false }` | Detail pages that draw their own header band and gutter |

Two more flags are available. `sidebar: true` reserves the right-hand panel, which is shown only
on screens at least 2000 px wide. `onPhone: true` keeps the route reachable in the on-call phone
layout.

The router does not check permissions. Navigation hides links through `NavItem.permission`. Inside
the page, controls are disabled with a tooltip. The backend enforces either way. See
[Data and state → Permissions](./data-and-state.md#permissions).

### Module folder

```text
modules/<module>/
├── pages/        default-exported route pages, plus page-only hooks such as useXDetailData
├── components/   the module's own dialogs, cards, tabs and their tests
├── hooks/        the module's own queries and mutations
└── stores/       optional, a Zustand store for the module's view state
```

`modules/X` never imports `modules/Y`. Code that two modules need moves to `@resources`. Oxlint's
`mcharbor/module-boundaries` rule enforces this, and `no-restricted-imports` stops `core/**` and
`resources/**` from importing a module. `core/router.tsx` and `core/router/` are the only exception.

## The scroll contract

`PageShell` (`resources/layout/PageShell.tsx`) enforces four rules:

1. **There is one scroller per screen.**
2. **A table scrolls inside its own wrapper.**
3. **Wide content owns its overflow.** Wrap it in `PageOverflow`, a `data-scroll-exempt` box with
   `overflow-x-auto`.
4. **The page body never scrolls sideways.**

In development mode, a second scrollable region logs a warning after 800 ms. To mark a deliberate
inner scroller, set `data-scroll-exempt` on it. `test/page-scroll-contract.test.ts` guards the
contract.

| Export | What it is |
|---|---|
| `PageShell` | Applied by the shell from the route handle. You do not render it |
| `PageBody` | The scrolling column in `data` mode. It is inert in `page` mode. A detail page puts its content in `<PageBody className="gap-4 p-6">` |
| `PageSplit` | `PageBody` plus the right-hand `PageSidebar`, for routes with `sidebar: true` |
| `PageOverflow` | A horizontal-overflow box for wide content |
| `usePageScrollMode()` | Reads the current mode |

## The page header

Pick one of three header shapes:

| Page | Header | What it does |
|---|---|---|
| List page | `TablePage` (below) renders it | Title, description and actions |
| Any other page | `PageHeader` from `resources/layout/PageHeader.tsx` | Portals a breadcrumb into the topbar (Home, then `breadcrumbs`, then the title). With `description` or `actions`, it adds an 80 px band under the topbar. `favoritePath` adds a favourite star and `trackRecent` adds the page to the recents list |
| Detail page | `HeaderBreadcrumb` plus `PageHeadIdentity` / `PageHeadActions` | `HeaderBreadcrumb` takes `crumbs`, `siblings` and `currentId`. With siblings, the last crumb becomes a switcher to the neighbouring objects. `PageHeadIdentity` portals the identity band, and `PageHeadActions` portals buttons into its right side |

A detail page claims the topbar with `useHeaderSlot(...).setActive(true)` in an effect. That puts
the breadcrumb in the topbar and hands the shell's padding back to the page. Use
`HeaderActionButton` for the band's icon actions, as in
`modules/containers/pages/ContainerDetailActions.tsx`.

## List pages: `TablePage`

`TablePage<T>` (`resources/layout/TablePage.tsx`) is the layout every list page uses:

```text
PageHeader → TableFilterBar → DataGridTable (the only scroller) → TablePagination
```

| Prop | What it does |
|---|---|
| `id` | A stable id. It namespaces the column, density and saved-filter preferences |
| `title`, `description`, `actions` | The header. Set `header={false}` inside a tab that already has a heading |
| `banner` | A slot above the table, usually `DegradedStateBanner` |
| `search`, `savedFilters`, `filters`, `facets`, `toolbar` | The filter bar. With `search.fields`, `TablePage` filters `data` itself (contains, exact or regex). Without `fields` it only renders the search box, and the page filters its own `data`. `toolbar` takes columns, density, refresh, export, chips and extra actions |
| `columns`, `data`, `loading`, `error`, `onRetry`, `empty` | The table and its states |
| `selectable`, `batchActions` | Row selection and the bulk-action bar (`BatchAction[]`) |
| `grouping` | Collapsible row groups |
| `getRowId`, `onRowClick`, `getRowClassName`, `isRowDisabled`, `getRowTestId` | Row behaviour |
| `body` | Replaces the table, for example with a card grid. Use the function form `(rows) => node` so the pager still works |
| `pagination` | `{ mode: 'client' }` (the default), `{ mode: 'server', … }` or `{ mode: 'none' }` |

Column widths come from a fixed token set: `auto`, `xs`, `sm`, `md`, `lg`, `xl`, `2xl`, `3xl`.
`resources/layout/table-page/` holds the toolbar, density, column editor, saved filters, export and
`useTableView*` hooks behind it. See [Composites → Tables](./composites.md#tables) for the grid
underneath.

## The canonical example: Volumes

`modules/volumes` is small and complete. Copy its shape.

**List: `pages/VolumesPage.tsx`** uses the `data` handle.

1. Data comes from `useVolumes()` (TanStack Query) and the create and remove mutations. View mode
   comes from the module's Zustand store `useVolumesViewStore`. Dialog state is local.
2. `hooks/useVolumesPageConfig.ts` returns `columns` and `batchActions`. The columns are built with
   the `textColumn`, `listColumn`, `bytesColumn` and `boolColumn` helpers.
3. `<TablePage id="volumes" … selectable batchActions body>`. The toolbar buttons carry
   `permission={PERM.VOLUMES_MANAGE}`.
4. After the table come `CreateVolumeDialog`, `RemoveVolumeDialog` and the prune confirmation.
5. Every string comes from `useTranslation('volumes')`.

**Detail: `pages/VolumeDetailPage.tsx`** uses the `pageFlush` handle.

```text
useParams · useTabFromUrl · useHeaderSlot.setActive(true)
loading   → CardSkeleton
not found → <ErrorState variant="page" primaryAction>
<div class="flex h-full min-h-0 flex-col">
  <HeaderBreadcrumb crumbs siblings currentId/>
  <PageBody className="gap-4 p-6">
    <Card> <VolumeDetailHeader/> <VolumeDetailTabs/> … </Card>
```

## Section-level layout pieces

| Component | Where | Use it for |
|---|---|---|
| `SettingsShell` | `@resources/components/ui/SettingsShell` | A settings-style form, with a header, a scrolling body and a sticky save/cancel `footer` |
| `SectionHeader` | `@resources/components/ui/SectionHeader` | A small uppercase heading for a block, with an optional description and action on the right |
| `SectionHero` | `@resources/components/SectionHero` | The large heading that opens an area: icon tile, `h1`, badge, description and actions |
| `SubNav` | `@resources/components/SubNav` | A tab rail with an animated pill. A disabled item stays visible with `disabledReason`, so the rail does not change width between roles |
| `ViewModeToggle` | `@resources/components/ViewModeToggle` | A table/timeline switch |
| `DegradedStateBanner` | `@resources/components/DegradedStateBanner` | A warning above a list whose last refresh failed. Pass it as `TablePage`'s `banner` |
| `StaleNotice` | `@resources/components/StaleNotice` | "Showing cached data from …" for offline or PWA-cached reads |
| `Box`, `Flex`, `Grid`, `Stack` | `@resources/components/ui/Layout` | Flex and grid wrappers whose `gap`, `columns` and alignment props map to static classes |
| `Card` and its parts | `@resources/components/ui/Card` | The standard panel |

## Navigation entries

Host navigation is data, in `resources/layout/sidebar-data.ts`:

```ts
type NavItem = {
  to: string;
  labelKey: string;              // i18n key, for example 'nav.volumes'
  icon: IconType;                // from @tabler/icons-react
  permission?: string | string[];// hides the link; a UX hide, not a security gate
  matchPaths?: string[];         // extra paths that mark the item active
};
```

To add one, put the item in the right section (`dockerSection`, `operateSection`, `governSection`,
`enterpriseSection` or `deliverSection`), and add the `nav.*` key to all six locales. Navigation
is the one place where a denied entry is **hidden** rather than disabled. Docker, Notifications,
Security and Settings live in the bottom dock, `SidebarBottomDock.tsx`.

Plugins do not edit this file. Their menu comes from the plugin itself, and
`pluginMenuToSections` turns it into sidebar sections.

## Plugin pages

One wildcard route, `plugins/:slug/*`, renders `core/plugins/PluginPageHost.tsx`. The host loads
the plugin's UMD bundle and calls its `mount(el, ctx)` inside the same `PageShell`. Tabs that a
plugin adds to host pages go through `PluginTabPanel` and `usePluginPageTabs`. See
[Hooks → Frontend hook points](../hooks/README.md#frontend-hook-points).

## Path aliases

| Alias | Points to |
|---|---|
| `@core` | `src/frontend/core` |
| `@resources` | `src/frontend/resources` |
| `@modules` | `src/frontend/modules`. Only the router may import across modules |
| `@widgets` | `widgets/` at the repository root |
| `@nodes` | `nodes/` at the repository root |
| `@` | `src/frontend` |

Vite pins a single copy of `react`, `react-dom`, `react-router`, `react-i18next`, `recharts`,
`@tabler/icons-react` and `@tanstack/react-query`. `app.css` adds Tailwind `@source` globs for
`widgets/`, `nodes/` and the plugin frontends, so a utility class used only there is still
generated.

## Related

- [Components](./components.md)
- [Composites](./composites.md)
- [Styling](./styling.md)
