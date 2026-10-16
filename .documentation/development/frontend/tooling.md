# Lint and Format

The frontend is linted with **Oxlint** and formatted with **oxfmt**, both run through
**Vite+** (`vite-plus`). ESLint and Prettier are not used. One configuration covers the host
(`src/frontend`), `widgets/`, `nodes/` and every plugin's `frontend/src`.

## Commands

Run them from `src/frontend`.

| Command | Does |
|---|---|
| `npm run lint` | Oxlint over `src/frontend`, type-aware and with type checking |
| `npm run lint:fix` | The same, applying the fixes Oxlint can make safely |
| `npm run lint:out-of-tree` | Oxlint over `widgets/`, `nodes/` and each plugin's `frontend/src` |
| `npm run format` | oxfmt over `src/frontend` |
| `npm run format:check` | Reports files oxfmt would change, and fails if there are any |
| `npm run format:out-of-tree` / `format:out-of-tree:check` | oxfmt over the out-of-tree sources |
| `npm run check` | `vp check`: format check, lint and type check in one run |
| `npm run dev` / `build` / `preview` / `test` | `vp dev`, `tsc -b && vp build`, `vp preview`, `vp test run` |

`make lint` runs both lint commands, both format checks and knip. CI runs `npm run lint` and
`npm run format:check` in the step **Lint (Oxlint) and format check**.

## Where the configuration lives

| File | Holds |
|---|---|
| `src/frontend/vite.config.ts` | `defineConfig` from `vite-plus`, with the `lint` and `fmt` blocks |
| `src/frontend/lint/config.ts` | The Oxlint rules and the oxfmt options those blocks import |
| `src/frontend/lint/mcharbor-plugin.js` | The local Oxlint plugin: `mcharbor/no-restricted-syntax`, `mcharbor/module-boundaries`, `mcharbor/copyright-header` |
| `src/frontend/scripts/out-of-tree.mjs` | Collects the out-of-tree source files and hands them to `vp lint` / `vp fmt` |
| `widgets/tsconfig.json`, `nodes/tsconfig.json` | Extend `src/frontend/tsconfig.json` so those trees type-check |

`vite` resolves to `@voidzero-dev/vite-plus-core` through an npm alias and an `overrides` entry
in `src/frontend/package.json`, so the build, the dev server, Vitest and the lint config all
share one Vite.

## The format

oxfmt writes single quotes, semicolons, trailing commas and a print width of 120. It sorts
Tailwind classes in `className`, `cn()`, `clsx()`, `cva()` and `twMerge()` calls. Locale
bundles (`locales/**/*.json`, `i18n/**/*.json`) are not formatted: they keep their 4-space
PowerShell style, and are edited by hand.

## The rules

Oxlint runs its `correctness` category with the `typescript`, `react`, `jsx-a11y`, `unicorn` and
`oxc` plugins. Type-aware rules and type checking are on (`options.typeAware`,
`options.typeCheck`). On top of that:

| Rule | Applies to |
|---|---|
| `mcharbor/copyright-header` | Every `.ts` / `.tsx` file outside tests |
| `mcharbor/no-restricted-syntax` | The project rules listed in [Components → What the linter enforces](./components.md#what-the-linter-enforces). Host code also may not name a plugin slug; plugin frontends may not keep prose in data tables |
| `mcharbor/module-boundaries` | `modules/X` importing `modules/Y`. Only `oncall` may reach `runbooks` and `incidents` |
| `no-restricted-imports` | `core/**` and `resources/**` importing `@modules/*`, except `core/router.tsx` and `core/router/` |
| `i18next/no-literal-string` | Literal JSX text and the `title`, `placeholder`, `aria-label`, `alt` and `label` attributes |
| `better-tailwindcss/no-restricted-classes` | Palette colours (`bg-white`, `text-gray-500`) and raw hex values |
| `max-lines` | 300 lines of code per file, not counting blank lines and comments |
| React Compiler rules | `react/set-state-in-effect`, `react/refs`, `react/purity`, `react/static-components`, `react/immutability`, `react/preserve-manual-memoization`, `react/globals` |
| `jsx-a11y/*` | The full correctness set, including `prefer-tag-over-role`, `no-autofocus` and `label-has-associated-control` |

Test files (`*.test.ts(x)`, `*.spec.ts`, `tests/**`, `*.stories.tsx`) skip the header, i18n,
size and project-syntax rules.

## Exceptions

A rule that is wrong for one line takes a directive on the line before it, with the reason after
`--`. Both `eslint-disable-next-line` and `oxlint-disable-next-line` are read. Inside a JSX
attribute list the directive sits directly above the attribute Oxlint reports.

```tsx
// eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: <reason>
```

The markers in `.claude/rules/sanity-markers.md` (`unavoidable cast:`, `i18n-open-set:`,
`do-not-translate:`) go in that reason.

## Hooks and the gate

- An edit made by an agent is formatted at once by `scripts/hooks/format-edited.mjs`, which runs
  `vp fmt` on the file, including widget, node and plugin frontend sources.
- The Stop gate (`scripts/hooks/gate-touched.mjs`) and `make gate` lint the touched files with
  Oxlint, in batches of 100 files.

## What goes wrong

| Symptom | Cause |
|---|---|
| `PATH must not contain ".."` | `vp lint ../../widgets`. Use `npm run lint:out-of-tree`, which passes absolute paths |
| An out-of-tree locale JSON or README gets reformatted | oxfmt was pointed at a directory outside `src/frontend`, where `ignorePatterns` do not apply. Use the out-of-tree script |
| A directive does not silence the rule | It names `no-restricted-syntax` instead of `mcharbor/no-restricted-syntax`, or it sits above the element rather than the reported attribute |
| `Cannot find module` or a JSX error in a widget or node | The tree's `tsconfig.json` is missing; it must extend `src/frontend/tsconfig.json` |
| A plugin bundle no longer matches its source | Lint fixes and formatting change `frontend/src`; run `npm run build` in that plugin's `frontend` |

## Related

- [Components](./components.md)
- [Data and state](./data-and-state.md)
- [Development workflow](../workflow/README.md)
