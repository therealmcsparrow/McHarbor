// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { defineConfig, devices } from '@playwright/test';
import { AUTH_STATE } from './tests/playwright-auth';

// Playwright config for the McHarbor a11y (axe-core) suite.
//
// Why a separate config from the visual-regression suite?
//   - The a11y suite uses `expect()` with hard assertions — a single
//     failing page aborts the test and surfaces clearly in CI logs.
//   - The visual suite uses full-page screenshots + PNG diffs and
//     benefits from a different timeout + output directory.
//   - Sharing a single worker (workers: 1) avoids racing axe-core
//     injections on the same Vite dev server.
//
// `check:a11y` is the canonical npm script and is wired into CI.
export default defineConfig({
  globalSetup: './tests/playwright-auth.ts',
  testDir: './tests/a11y',
  // Per-page budget. The aggregate test sets a longer timeout via
  // test.setTimeout() because it scans the whole page set in series.
  timeout: 30_000,
  // Sequential: axe scans every page in a single worker, which keeps
  // the dev-server load flat and the report output deterministic.
  fullyParallel: false,
  workers: 1,
  // No retries on a11y — the spec already records the per-page JSON
  // summary, so a flake shows up as a hard failure rather than a
  // silent retry-then-pass.
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  outputDir: './.results/a11y',
  use: {
    storageState: AUTH_STATE,
    baseURL: 'http://127.0.0.1:8173',
    headless: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Reuse an already-running `npm run dev` so the visual suite and
  // the a11y suite can share a dev server when run sequentially.
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1',
    url: 'http://127.0.0.1:8173',
    reuseExistingServer: true,
    timeout: 120_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
});
