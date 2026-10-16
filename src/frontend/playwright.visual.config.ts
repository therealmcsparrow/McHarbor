// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { defineConfig, devices } from '@playwright/test';
import { AUTH_STATE } from './tests/playwright-auth';

// Playwright config for the McHarbor visual regression suite.
//
// Why a separate config?
//   The a11y suite is intentionally single-threaded with strict
//   `expect()` assertions so failures cascade cleanly in CI logs.
//   The visual suite is heavier (full-page screenshots + PNG diffs)
//   and benefits from independent timing and output directories.
//
// The dev server (Vite on :8173) is shared with the a11y config
// via `reuseExistingServer` so we don't fight a parallel `npm run dev`.
export default defineConfig({
  globalSetup: './tests/playwright-auth.ts',
  testDir: './tests/visual',
  // Visual regressions run a fixed per-page budget — 60s per page
  // covers network + screenshot + diff with headroom for CI jitter.
  timeout: 60_000,
  // Sequential: the suite already iterates theme × page in-band
  // and parallel diffs would muddy the output directory.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  outputDir: './.results/visual',
  use: {
    storageState: AUTH_STATE,
    baseURL: 'http://127.0.0.1:8173',
    headless: true,
    trace: 'retain-on-failure',
    // We capture screenshots ourselves (fullPage) — leave Playwright's
    // failure-only default on so unexpected crashes still get a
    // viewport-only debug frame.
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1',
    url: 'http://127.0.0.1:8173',
    reuseExistingServer: true,
    timeout: 120_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
});
