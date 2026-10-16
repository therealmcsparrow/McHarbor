# Frontend: Shared composites

The larger shared building blocks in `src/frontend/resources/components/`: tables, editors, charts,
dialogs and the terminal. A composite that two modules need lives here and is never copied into a
module.

## Tables

A module never mounts the data grid directly. A list page uses `TablePage`
([Layout](./layout.md#list-pages-tablepage)), which composes these parts:

```text
PageHeader
TableFilterBar        search, saved filters, filters, facet chips | TableToolbar
DataGridTable         the page's only scroller
  DataGridHeader · DataGridRow · DataGridGroupHeader
TableBulkActions      the selection bar, with DataGridBatchActions for confirmations
TablePagination
```

The canonical example is `modules/containers/pages/ContainersPage.tsx`. It uses search,
filters, saved filters, the toolbar, selection, batch actions and a card-view `body`.

### Columns

Import table types from `@resources/components/table-features`, never from
`@tanstack/react-table`. That file is the one shared TanStack Table feature registry. It covers
filtering, ordering, sizing, visibility, pagination, selection and sorting, with the
`alphanumeric`, `datetime` and `text` sort functions. Because every table uses the same registry,
any `ColumnDef` works in any table.

Column `meta`:

| Field | What it does |
|---|---|
| `width` | A token: `auto`, `xs`, `sm`, `md`, `lg`, `xl`, `2xl` or `3xl`. Widths are tokens because inline styles are banned. Resizing snaps to the next token |
| `locked` | The column cannot be hidden or moved |
| `defaultVisible` | `false` hides the column until the user turns it on |
| `noExport` | Leaves the column out of the CSV and JSON export |
| `group`, `label`, `wrap` | The column-editor group, the label in that editor, and wrapping cell text |

### Batch actions

```ts
type BatchAction<T> = {
  label: string; icon?: Icon; variant?: 'default' | 'destructive';
  onClick: (selectedRows: T[]) => void;
  confirm?: boolean;       // ask first, through DataGridBatchActions' ConfirmDialog
  permission?: string | string[];
};
```

A destructive batch action takes both `confirm` and `permission`. Selection lives in
`useDataGridSelection`. That hook drops ids that disappear after a poll and never selects a
disabled row.

### Grid parts

| Part | Use it for |
|---|---|
| `DataGridTable` | The renderer behind `TablePage`. It draws the loading skeleton, the `ErrorState` with retry, and the empty state. `density` is `compact`, `default` or `comfortable`. `fillHeight` makes the table the page's scroller |
| `DataGridGroupHeader`, `bucketRows` | Collapsible groups, in the order the rows first appear |
| `TablePagination` | 1-based pages with page sizes 25, 50, 75 and 100. Leave out `onPageSizeChange` to hide the size selector |
| `FilterChips` | Single-select facet chips with counts (`{ id, label, count, tone }`) |
| `DateRangeFilter` | Presets plus a custom range. Starts from `DEFAULT_DATE_RANGE_VALUE` |
| `EnvironmentScopeFilter` | An environment picker for cross-environment pages |
| `ViewModeToggle` + `EventTimeline` | A table with a timeline view, as on the Activity and Lifecycle pages |

### A row that failed

A row whose state is `failed` or `error` shows the backend's message through
`<StatusError error={row.lastError} />`, next to its state badge. Pass the field even when it may
be empty, because the component renders nothing for a blank message. Never truncate the message
into the cell, put it in a `title`, or add a second line under the name. The message must be one a
service deliberately stored, never a raw request error (`.claude/rules/table-status-errors.md`).

## Editors and inputs

| Component | Props | Notes |
|---|---|---|
| `CodeEditor` | `value`, `onChange`, `readOnly`, `language` (`yaml`, `javascript` or `typescript`), `minHeight` | CodeMirror 6, loaded lazily. A read-only editor renders a plain `<pre>` and loads nothing |
| `KeyValueEditor` | `entries`, `onChange`, `keyLabel`, `valueLabel`, `addLabel` | Labels are already translated. Rows keep stable keys |
| `ReadableScheduleField` | `value` (a cron string), `onChange`, `label`, `hint`, `timezone` | **The only cron input.** It switches between a readable builder and an advanced cron field, and shows the next run times |
| `CronSchedulePreview` | `expression`, `timezone` | The next run times on their own. Renders nothing for a blank expression |
| `storage-location-options` | `storageProviderOptionsWithPlugins()` | Storage provider options for a `Select`. Plugin adapters appear without the host naming them |

### Schedules

Every cron input is `<ReadableScheduleField>`. The cron string is the single stored value, in
state, in the request body and in the database. There is no separate "mode" flag, because the
field works out the mode itself. Wherever a schedule is listed, show
`describeCron(cron, t)` from `@resources/utils/schedule`, the raw cron, and `nextRunAt`
(`.claude/rules/schedule-fields.md`). Copy
`modules/settings/components/CreateScheduleDialog.tsx`.

## Charts and metrics

| Component | Use it for |
|---|---|
| `StatCard` | A metric tile: `title`, `value`, `unit`, `icon`, `trend`, and a `chart` slot (usually a `Sparkline`) |
| `Sparkline` | A tiny SVG line from `number[]`, with no dependencies |
| `StatsAreaChart` | An area chart with one or two series. Recharts, loaded lazily |
| `MeasuredResponsiveContainer` | Renders a chart only once it has a measured size. Use it instead of Recharts' `ResponsiveContainer` |
| `MetricHistoryPanel` | Stored history (CPU, memory, network, disk, GPU) with a range picker. `history-metrics.ts` holds the metric definitions |

Chart colours come from the `chart-1` to `chart-8` tokens through `@resources/theme/chart-colors`.
Never pick a hex value. See [Styling](./styling.md).

## Text and logs

| Component | Use it for |
|---|---|
| `LogViewer` | Line-based logs: `lines`, `autoScroll`, `levels`, `colorizeSeverity`, and line selection with the keyboard |
| `Markdown` | Rendering user or plugin Markdown. Input is capped at 100,000 characters |
| `EventTimeline` | Date-grouped entries (`TimelineEntry`) with a tone, an icon, badges and fields |
| `ContainerIcon` | A logo for an image name, with a fallback |

## Dialogs

| Component | Use it for |
|---|---|
| `OperationProgressDialog` | Progress for a batch operation. State comes from `useBatchProgressOperation` |
| `ManualCopyDialog` | When the browser refuses a clipboard write, for example on plain HTTP on a LAN address. Use it whenever `copyToClipboard` returns `manual-required` |
| `TakeOverDialog`, `LinkContainerDialog` | Adopting a container into a stack, and linking a container to a stack |
| `McHarborRestartDialog` | Mounted once. Drive it through `useMcHarborRestartStore` |

For a dialog of your own, start from `Dialog` and `ConfirmDialog` in [Components](./components.md#overlays-and-dialogs).

## Terminal

- `useXTerm(containerId, options)` from `XTermPanel.tsx` returns a `termRef` to attach to a `div`,
  plus `connected`, `connect` and `disconnect`. xterm loads on first use.
- The terminal talks to the backend over a WebSocket with dedicated exec messages, so it works
  through the remote agent.
- `TerminalFullscreen.tsx` gives a CSS full-screen overlay (`useTerminalFullscreen`,
  `TerminalFullscreenButton`). After toggling it sends a resize, so xterm refits.

## Mounted once by the shell

You never mount these from a page. Each one has a hook or store to drive it:

| Component | Driven by |
|---|---|
| `GlobalSearch` (`mod+k`) | `useCommandRegistry` adds command-palette entries |
| `AppToaster` | `appToast` and the `MutationCache` |
| `WhatsNewOverlay` | `getReleaseItems()` in `whatsNewItems.ts` |
| `ShortcutsCheatsheet` (`?`) | `useShortcuts` |
| The resource quick-peeks (`ContainerOverview`, `ImageOverview`, `StackOverview`, `VolumeOverview`) | The header overlays |

## Domain folders

These components belong to one area but are shared because more than one page shows them.

| Folder | Contents |
|---|---|
| `host/` | Host services, OS updates and reboot panels and dialogs for an environment |
| `secrets/` | `SecretPicker`, the insert-secret flow (`SecretInsertProvider`), `PromoteToSecretDialog` |
| `security/` | `PermissionEditor`, policy and endpoint sections, ABAC and SCIM parts |
| `settings/` | `SettingsGroup`, appearance and toast fields, backup-key cards, `PasswordPolicyCard` |
| `runbooks/` | `RunbookRunner` and its step cards |
| `capacity/`, `compliance/`, `incidents/`, `support/`, `demo/` | Capacity overview, attestation dialogs, the incident narrative, pre-upgrade checks, the demo header icon |

## Lazy loading a heavy dependency

`CodeEditor`, `StatsAreaChart` and `CronSchedulePreview` are thin shells. Each lazy-imports an
`*Impl.tsx` inside `<Suspense>`, with a skeleton the size of the real element. That keeps CodeMirror,
Recharts and `cron-parser` out of every page that only imports the shell. Callers import only the
shell, which owns the prop types. Follow the same pattern for any new component built on a large
library.

## Related registries

| Folder | What it is |
|---|---|
| `resources/widgets/` | Built-in dashboard widget metadata (`getWidgetMeta`). The widgets themselves live in `widgets/` at the repository root. See the `widget` skill and [How-to → Widgets](../../how-to/widgets/README.md) |
| `resources/nodes/` | Workflow node category colours and legacy aliases. The nodes live in `nodes/` at the repository root |
| `resources/ai/` | The assistant seam. A page calls `useAIContext({...})` to tell the assistant what is on screen, and `openAssistant(prompt)` to open it |

## Related

- [Layout](./layout.md)
- [Components](./components.md)
- [Data and state](./data-and-state.md)
