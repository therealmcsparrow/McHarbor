# Visual Regression

Playwright-based screenshot diffing for McHarbor's primary surfaces,
in both **light** and **dark** themes. Captured snapshots are compared
against committed PNG baselines (Playwright's `*.spec.ts-snapshots/`
directories).

## Why this exists

Pixel-level visual regression is the only reliable way to catch:

- Layout drift (a new wrapper shrinks a card by 4px)
- Theme regressions (an OKLCH token accidentally uses the wrong tone)
- Cross-page consistency bugs (a section accent flips to amber when it
  should be cyan)

These are the bugs the axe-core / TypeScript / vitest suites miss.

## How it works

1. **List** — `tests/visual/pages.ts` enumerates the routes we capture.
2. **Capture** — `tests/visual/run-visual.spec.ts` opens each route in
   both themes, disables animations, and screenshots the full page.
3. **Diff** — Playwright's built-in `toHaveScreenshot()` matcher compares
   each capture against its committed baseline with a **0.5%** pixel
   diff budget.
4. **Baseline update** — `npm run test:visual:update` re-runs the suite
   with `--update-snapshots`, regenerating the committed baselines.

## Running locally

```bash
# Start the dev server (in one terminal)
cd src/frontend
npm run dev

# Capture + diff against the committed baselines
npm run test:visual

# Regenerate baselines after an intentional change
npm run test:visual:update
git add tests/visual/run-visual.spec.ts-snapshots/
git commit -m "test:visual: update baselines for <reason>"
```

## Auth

The suite reads `/api/auth/status` and signs in via the login page when
auth is enabled. Set `MC_ADMIN_USER` and `MC_ADMIN_PASSWORD` (the same
env vars the a11y suite consumes).

When `AUTH_DISABLE=true` (dev mode) the suite skips the login step.

## CI

`npm run test:visual` is wired into the GitHub Actions workflow
(`.github/workflows/test.yml`) as a non-blocking check. Promotion
pipelines (`alpha`, `beta`, `release`) gate on it once the golden set
stabilizes — flip `continue-on-error: false` in the workflow after
the first committed baseline set has been signed off.

## Adding a page

1. Add the route to `VISUAL_PAGES` in `tests/visual/pages.ts`.
2. Re-run `npm run test:visual` (it fails with "no baseline yet").
3. Run `npm run test:visual:update` and review the new PNG.
4. Commit the new baseline alongside your code change.

## File layout

```
tests/visual/
├── pages.ts                # route list
├── run-visual.spec.ts      # capture + diff
├── update-golden.ts        # baseline regeneration CLI
├── README.md               # this file
└── run-visual.spec.ts-snapshots/
    ├── dark-containers.png
    ├── light-containers.png
    └── ...

playwright.visual.config.ts # separate config so visual + a11y suites
                            # can iterate independently
```
