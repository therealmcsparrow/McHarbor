# Frontend: UI primitives

The primitives in `src/frontend/resources/components/ui/`. Build from these before writing your own
markup. Each one already handles focus, permissions, i18n and theming the same way as the rest of
McHarbor.

Import a primitive by its file path, for example
`import { Button } from '@resources/components/ui/Button'`. The barrel at
`@resources/components/ui` re-exports most of them. It does **not** re-export `ActionButton`,
`HeaderActionButton`, `PlainButton`, `MutationConfirmDialog`, `Avatar`, `StatusIndicator`,
`StatusStrip`, the `Skeleton` variants, the `*Variants` helpers, `TABLE_CELL_PADDING` or the
`*.parts` files.

## Pick the right one

| You need | Use | Not |
|---|---|---|
| An action button | `Button` | A raw `<button>`, which is a lint error |
| An icon action in a table row or card | `ActionButton` | `Button size="icon"` with a hand-made tooltip |
| An icon action in a detail page's header band | `HeaderActionButton` | `ActionButton` |
| A clickable chip, swatch or tree row with your own look | `PlainButton` | A `div` with `onClick` |
| A yes/no confirmation | `ConfirmDialog` | `window.confirm`, which is a lint error |
| A confirmation that runs one mutation | `MutationConfirmDialog` | `ConfirmDialog` plus your own error state |
| A query or page that failed | `ErrorState` | A blank panel or an empty table |
| A table row's failure message | `StatusError` | Text truncated into the cell |
| Context inside the page (a warning, a note) | `Alert` | A toast |
| The result of a mutation | The global toast, which is automatic | A per-mutation `onError` toast |
| A loading placeholder | `Skeleton`, `CardSkeleton`, `TableSkeleton`, `ChartSkeleton` | A centred spinner for a whole page |
| A small in-place wait | `Spinner` | |
| Nothing to show | `EmptyState`, `InlineEmpty`, `NoResults` | A bare "No data" string |
| A dropdown (a "selector") | `Select`. For a free-text escape, add an "Enter manually" option | An input with a datalist, which reads as a plain text field |
| Several values from a list | `MultiSelect` | A row of checkboxes |
| A label, help text and error around a control | `Field` (or `FieldGroup` for a group) | A hand-wired `<label>` |
| A settings form with Save and Cancel | `SettingsShell` with `footer` | Your own sticky footer |
| A list of rows with Add and Remove | `RepeaterRows` | Your own add/remove list |
| A status shown as a dot, badge or pill | `StatusIndicator` | A coloured `span` |
| A status string mapped to a colour | `StatusBadge` with a ready-made map | Your own switch statement |
| Text with a colour tone | `Text`, `Heading`, `Caption` with `tone` | `text-red-500` |
| The full value behind truncated text | `Hint` | The native `title` attribute |

## Buttons and actions

**`Button`**
- `variant`: `default`, `destructive`, `caution`, `outline`, `secondary`, `ghost` or `link`.
- `size`: `default`, `sm`, `lg`, `icon` or `icon-sm`. Every size except `lg` takes its height from
  `.mc-control`, so a button lines up with the inputs and selects next to it.
- `asChild` renders through Radix `Slot`, for example to wrap a router `Link`.
- `permission`: a key or an any-of list.
  - A denied button renders as a disabled `<button>` with a tooltip naming the missing key, from
    `usePermissionGate`.
  - The `asChild` link target is dropped when denied, so a denied link cannot be followed.
  - A button without `permission` uses no hooks.
- `tooltip`: icon sizes only. It defaults to the `aria-label`. Pass `false` to switch it off.
- There is no `loading` prop. Swap the label (`actions.saving`) and set `disabled` instead.

**`ActionButton`**
- An icon-only ghost button with a tooltip, for dense row and card actions.
- Props: `label`, `icon`, `onClick`, `disabled` and `permission`.
- The click calls `stopPropagation()`, so it does not trigger the row's `onRowClick`.
- A denied button's tooltip reads "<label> — <reason>".

**`HeaderActionButton`**
- The equivalent for a detail page's header band.
- `tooltip` is both the tooltip and the accessible name, and can explain a reason other than a
  permission, for example "Built-in networks cannot be removed".
- Takes a `variant` (default `outline`), `disabled` and `permission`.

**`PlainButton`**
- A `type="button"` with a visible focus ring and no other styling.
- For chips, tabs, tree rows and swatches whose look you draw yourself.

**`Tag`**
- A chip with a `tone`, a `size` and an optional `dismissible` close button.

## Forms and inputs

| Primitive | What it gives you |
|---|---|
| `Input`, `Textarea` | Native controls styled with `.mc-field`. `variant` is `default` or `outline` |
| `Select` | One value, on Radix Popover. Options take `label`, `icon`, `description` (a second line in the list) and `group` |
| `MultiSelect` | Several values. `emptyLabel` names a meaningful empty selection ("All environments"). `searchable`, `allowCustom` (type a value and press Enter), and a `disabledReason` per option. The list stays open while you toggle |
| `NumberInput` | A stepper with `min`, `max`, `step` and `suffix`. With `nullable`, `value` may be `null` |
| `PasswordInput` | A show/hide toggle |
| `PasswordRequirements` | A live checklist against the password policy. It stays neutral until the first keystroke. The server validates again |
| `Checkbox`, `Switch`, `SwitchGroup`, `Slider`, `Label` | Radix-based controls. `Slider` takes `zones` coloured `safe`, `caution` or `unsafe` |
| `FileUpload` | Drag-and-drop with `accept`, `maxSize`, `maxFiles` and a status per file |
| `Field` | Label, control, description and **error** in one wrapper. It generates the id, wires `htmlFor`, and points `aria-describedby` at the description and error. `FieldGroup` is the `<fieldset>` version for radios and checkbox groups |
| `FormField` | A lighter wrapper with no `error`. Prefer `Field` |
| `Section` | A titled group of fields with an icon and help text |
| `RepeaterRows` | Homogeneous rows plus an Add button, for env vars, ports and mounts. You own the data and the component keeps stable row ids |
| `FormActions` | The save/cancel bar: `canSave`, `isSaving`, `warning` (for notes like "this recreates the container"), `extraActions` and `sticky` |
| `SettingsShell` | A header, a scrolling body and an optional `footer` with the same fields as `FormActions`. The shell for any settings-style page or tab |

A cron or schedule input is never a plain input. Use `ReadableScheduleField`, described in
[Composites → Editors](./composites.md#editors-and-inputs).

## Feedback and status

**Errors**
- `ErrorState` is for a failed query.
  - `variant` is `banner`, `card`, `inline` or `page`.
  - Pass `error` and its message becomes the description.
  - `onRetry` adds a retry button. An `errorId` gets a copy button.
  - Every page with a query renders it on `isError`.
- `StatusError` is for a row-level failure. It shows a small red icon in the cell, and the full
  message opens in a dialog with a copy button. It renders nothing for an empty string.
- `PluginMountBoundary` (`@resources/components/PluginMountBoundary`) wraps a lazily loaded plugin or
  widget bundle. A render error inside it, which a plugin's own React root would otherwise turn into
  a blank area, becomes an `ErrorState` titled "This plugin view failed to render." with a Retry
  button that remounts the subtree. The host wraps each `Suspense` that mounts plugin content
  (header overlays, the workflow editor, the dashboard, the store). A chunk that failed to load stays
  failed until the page reloads. The plugin side of the same guarantee is `guardedRoot` in the
  plugin's own `mountGuard.tsx`; see [Plugin frontend](../../how-to/plugin/frontend.md#the-entry-point).
- `Alert` is for in-page context, with `variant` `info`, `success`, `warning` or `destructive`.
  `destructive` and `warning` announce as `role="alert"`.

**Loading and empty**
- `Skeleton` and its variants set `aria-busy`.
- `TableSkeleton` takes `rows`, `columns` and `density`. `CardSkeleton` takes `lines`.
  `ChartSkeleton` takes `height`.
- `EmptyState` takes `icon`, `title`, `description`, `primaryAction` and `secondaryAction`.
  `variant` adds `warning`, `destructive`, `info` and `compact`.
- `InlineEmpty` is a one-line empty message.
- `NoResults` is for a filter that matched nothing, and offers a Clear button.

**Status**

| Primitive | Use it for |
|---|---|
| `StatusIndicator` | The one status presentation. `variant` is `dot`, `badge` or `pill`. Use `pulse` only for a state that is continuously true. A bare dot needs `ariaLabel` |
| `Badge` | A label with a variant: `default`, `secondary`, `neutral`, `destructive`, `outline`, `success`, `warning` or `info` |
| `StatusBadge` | A `Badge` keyed by status. It ships `CONTAINER_STATUS`, `STACK_STATUS`, `ENVIRONMENT_STATUS` and `RECONCILER_STATUS` |
| `StatusStrip` | A history strip of `pass`, `fail`, `warn` and `idle` blocks. Failures are also shorter, so it does not depend on colour alone |
| `SeverityBadge`, `SeverityIcon` | The five-level severity scale, labelled from the `severity` namespace |
| `ProgressBar`, `RowProgress` | Overall progress, and progress per row in a batch |
| `status-card.ts` | Class constants that keep a row of health and uptime cards at equal height |

## Overlays and dialogs

**`Dialog` and its parts** (`DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`,
`DialogBody`, `DialogFooter`)
- `DialogContent` ignores clicks outside it, so a stray click never discards a half-filled form.
  Only Cancel, the close button or Escape close it.
- `hideCloseButton` removes the close button.

**`ConfirmDialog`**
- `variant` is `destructive` (the default), `caution` or `default`.
- `loading` swaps the label to "Processing" and disables the button.
- `permission` gates the confirm button.
- Focus returns to where it was when the dialog closes.

**`MutationConfirmDialog`**
- Pass the `mutation` and its `variables`.
- It resets the mutation when it opens and closes when the mutation succeeds.
- On failure it stays open and shows the server's message inline, unchanged.
- If the change was held for approval, it shows that and disables confirm.
- This is the shape the destructive-control test suite expects.

**Tooltips and popovers**
- `Tooltip`, `TooltipTrigger`, `TooltipContent` and `TooltipProvider` wrap Radix Tooltip.
- `Hint` wraps one child in a tooltip that opens on focus and on hover. It replaces `title`.
- `Popover`, `PopoverTrigger`, `PopoverContent` and `PopoverAnchor` wrap Radix Popover.

## Layout and structure

| Primitive | What it gives you |
|---|---|
| `Box`, `Flex`, `Grid`, `Stack` | Wrappers whose `gap` (0 to 12, half steps included), `align`, `justify` and `columns` props map to static classes, so Tailwind always generates them. `columns` also accepts a responsive class string such as `"grid-cols-1 md:grid-cols-3"` |
| `Card` and its parts | The standard panel |
| `SectionHeader` | A block heading with a description and an action |
| `ListItem` | `title`, `subtitle`, `badge` and `actions` in one row |
| `Disclosure` | A collapsible section, controlled or uncontrolled, with `aria-expanded` wired |
| `Tabs`, `TabsList`, `TabsTrigger`, `TabsContent` | Radix Tabs. `TabsTrigger` takes `contextMenuItems` for a right-click menu. Keep the active tab in the URL with `useTabFromUrl` |

## Data display

| Primitive | What it gives you |
|---|---|
| `Table` and its parts | A plain native table for small, page-owned tables. For a list page use `TablePage` ([Layout](./layout.md#list-pages-tablepage)) |
| `TABLE_CELL_PADDING` | `compact`, `default` and `comfortable` cell padding. `default` matches the data grid's rows |
| `InfoRow` | A label and value. `fixed` gives a 144 px label column, and `spread` puts the value on the right with an ellipsis |
| `Text`, `Heading`, `Subheading`, `Title`, `Caption`, `Code` | Typography with `tone`, `size`, `weight`, `truncate` and `monospace` |
| `Avatar`, `Kbd` | A user avatar with initials, and a keyboard key |

## Accessibility helpers

- `VisuallyHidden` is text only a screen reader reads. Try `aria-label` first.
- `FocusTrap` is a custom focus trap. Radix dialogs, popovers and tooltips already trap focus.
- `SkipLink` is mounted by the shell. A module never adds one.

Every icon-only control needs an `aria-label`, and every `<img>` needs an `alt`.

## What the linter enforces

Oxlint, run through Vite+, applies these rules to the host, `widgets/`, `nodes/` and the plugin
frontends. The configuration is `src/frontend/lint/config.ts`, read by the `lint` block of
`vite.config.ts`. See [Lint and format](./tooling.md) for the commands and the rule set.

| Rule | Message |
|---|---|
| Raw `<button>` | "Use &lt;Button&gt; or &lt;PlainButton&gt; from @resources/components/ui" |
| `style={{…}}` | "No inline style objects; Tailwind, or `style={cssVars({...})}` + `w-(--w)`" |
| `useTranslation()` with no namespace | "Name the namespace" |
| `defaultValue` on `t(...)` | Not allowed. See [Data and state → i18n](./data-and-state.md#i18n) |
| A `queryFn` ending in `.then(r => r.data)` | "queryFn must end in .then(assertSuccess)" |
| An `onError` with no arguments | "The global MutationCache already toasts the server message" |
| `toLocaleString` and similar | "Use formatDate / formatDateCustom from @resources/utils/format" |
| Raw palette classes (`bg-white`, `text-gray-500`) and hex values | "Use semantic OKLCH tokens" |
| `window.alert` / `window.confirm` | `no-alert` |
| More than 300 lines | `max-lines`, which skips blank lines and comments. Extract a hook |
| `any`, literal JSX strings, inaccessible click handlers | `no-explicit-any`, `i18next/no-literal-string`, `jsx-a11y` |
| A plugin slug in host code | "Host code must not name a plugin slug" |
| `setState` inside an effect, `ref.current` read during render, impure calls during render | The React Compiler rules (`react/set-state-in-effect`, `react/refs`, `react/purity`) |
| `role="button"` / `role="status"` / `role="group"` on a `div`, `autoFocus` | `jsx-a11y/prefer-tag-over-role`, `jsx-a11y/no-autofocus`. Use `PlainButton`, `<output>`, `<fieldset>` |
| A promise that is neither awaited nor `void`-ed | `typescript/no-floating-promises` (type-aware) |

A rule that is right to break in one place takes a directive on the line before, with a reason:
`// eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: <reason>`. The
`no-restricted-syntax` rules come from the local plugin `lint/mcharbor-plugin.js`, so the
directive names `mcharbor/no-restricted-syntax`, not the bare ESLint name.

## Related

- [Layout](./layout.md)
- [Composites](./composites.md)
- [Styling](./styling.md)
- `.claude/reference/ui-pr-checklist.md`, the checklist a UI change is reviewed against
