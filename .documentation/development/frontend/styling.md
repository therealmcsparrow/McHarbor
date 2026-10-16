# Frontend: Styling

McHarbor is dark-first: the default theme is `dark`. Every colour is addressable by a theme. A
component never names a colour. It names a **semantic token**, and the active theme decides what
that token looks like. Styling is Tailwind 4 utilities only.

## Tokens

The tokens are defined in `src/frontend/app.css`:

- The `@theme inline` block maps each token to a CSS variable.
- `:root` holds the light values and `.dark` holds the dark values.
- The values are `oklch()`.
- The `theme` store switches the `dark` class on `<html>`.

| Group | Utilities |
|---|---|
| Surface and text | `bg-background`, `text-foreground`, `bg-card` / `text-card-foreground`, `bg-popover`, `bg-muted` / `text-muted-foreground`, `bg-accent`, `bg-primary` / `text-primary-foreground`, `bg-secondary`, `bg-destructive` |
| Lines and focus | `border-border`, `border-input`, `ring-ring` |
| Status | `success`, `warning`, `danger`, `critical`, `info`, `neutral`, each with a `-foreground` for text on a solid fill (`bg-success text-success-foreground`) |
| Charts | `chart-1` to `chart-8` |
| Glass surfaces | `structure`, `canvas-2`, `glass`, `glass-2`, `glass-solid`, `hairline`, `hairline-2`, `edge-hi`, `shine`, `scrim` |
| Sidebar | `sidebar`, `sidebar-foreground`, `sidebar-primary`, `sidebar-accent`, `sidebar-border`, `sidebar-ring` |
| Type | `font-sans` (Inter Tight), `font-display` (Bricolage Grotesque, for headings), `font-mono` (JetBrains Mono) |

Opacity modifiers work on every token (`bg-primary/12`, `hover:bg-muted/40`).

**Never use** a palette class (`bg-white`, `text-gray-500`, `border-slate-200`) or a hex value
(`text-[#3b82f6]`). The `better-tailwindcss` lint rule rejects both, with "Use semantic OKLCH
tokens".

## Status colours

Do not pick a status colour by hand. Use one of these:

| Helper | Gives you |
|---|---|
| `StatusIndicator`, `Badge`, `StatusBadge` | The finished component. See [Components → Status](./components.md#feedback-and-status) |
| `STATUS_COLORS[tone]` from `@resources/theme/status-palette` | `solid`, `soft`, `outline`, `dot` and `text` classes, plus `cssVar`, for one `StatusTone` (`success`, `warning`, `danger`, `critical`, `info` or `neutral`) |
| `statusSoft(tone)`, `statusSolid(tone)`, `statusOutline(tone)` | The same classes as functions, with extra classes appended |
| `capacityTone`, `severity`, `container-state` in `@resources/utils` | The mapping from a domain value to a tone |

Never let colour carry the meaning alone. Pair it with a label, an icon or a shape, the way
`StatusStrip` draws a failure as a shorter bar.

## Chart colours

`metricColor(metric)` and `CHART_COLORS` in `@resources/theme/chart-colors` assign each metric
(`cpu`, `memory`, `networkRx`, `networkTx`, `diskRead`, `diskWrite`, `gpu`, …) a fixed step on the
`chart-1` to `chart-8` ramp. The steps are spaced so that pairs read together, such as receive and
transmit, stay distinguishable. Use them, so the same metric has the same colour on every chart.

## Controls share one height

`.mc-control` sets the shared control height (`--mc-control-h`), and `.mc-field` gives inputs their
border, hover and focus states. `Button`, `Input`, `Select` and `NumberInput` already apply them,
so a button and an input in one row line up. Apply `.mc-field` yourself only when you build a new
input primitive.

`app.css` also defines the glass surfaces (`.mc-glass`, `.mc-glass-plain`, `.mc-glass-overlay`)
and the overlay chrome (`.mc-modal`, `.mc-menu`, `.mc-sheet`, `.mc-tooltip`, `.mc-scrim`). The
primitives use them. A module should not need to.

## No inline styles

`style={{ … }}` is a lint error. Use one of these instead:

1. **A Tailwind arbitrary value** for a fixed size: `min-h-[300px]`, `w-[42rem]`.
2. **A CSS variable** for a value computed at run time. Set it with `cssVars` from
   `@resources/utils/cssVars`, and read it in a class:

   ```tsx
   <Box className="pl-(--indent)" style={cssVars({ indent: `${indent + 8}px` })}>
   ```

   (`modules/containers/components/RestoreTreeRow.tsx`.)

## Class names

- Combine classes with `cn(...)` from `@resources/utils/cn`. It runs `clsx` and then
  `tailwind-merge`, so a caller's `className` overrides the component's default.
- A primitive with variants uses `class-variance-authority`, as `buttonVariants`, `badgeVariants`
  and `inputVariants` do. Export the variants beside the component.
- A class that depends on a prop goes through a static lookup map, as `Layout.tsx` does for `gap`,
  so Tailwind can see every class at build time. A class string assembled from pieces at run time
  (`` `gap-${n}` ``) is never generated.
- `app.css` scans `widgets/`, `nodes/` and the plugin frontends with `@source`, so a utility used
  only there still works.

## Themes

The selectable themes are CSS files in `src/frontend/public/themes/`, including
`mcharbor-default`, catppuccin, dracula, gruvbox, nord, solarized and tokyo-night. Each theme must
define every required token. `npm run theme:lint` (`scripts/theme-tokens.mjs --check` plus
`scripts/theme-lint.mjs`) fails when a token is missing. A new token therefore goes into `app.css`
and every theme in the same change.

Users can override accent, density, font and base size through the theme customiser
(`themeCustomizer` store). A component that uses tokens and `.mc-control` follows those settings
for free.

## Motion

Respect the reduced-motion preference. The `motion` store writes `data-reduce-motion` on
`<html>`, and `motionAllowed()` tells script-driven animation whether to run. A `pulse` on a
status dot is only for a state that is continuously true.

## Related

- [Components](./components.md)
- [Layout](./layout.md)
- `.claude/reference/ui-pr-checklist.md`
- `.claude/reference/accessibility.md`
