// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { test, expect, type Page } from '@playwright/test';
import { VISUAL_PAGES, VISUAL_THEMES } from './pages';

// Playwright visual regression suite for McHarbor's primary
// surfaces in both light and dark themes.
//
// Strategy:
//   1. Disable animations + transitions so screenshots are
//      deterministic across runs.
//   2. Force the .dark / .light class on <html> so the theme is
//      independent of the user's stored preference.
//   3. Use Playwright's built-in `toHaveScreenshot()` matcher,
//      which handles golden baseline management + pixel diff with
//      configurable tolerance. Baselines live next to the spec as
//      `run-visual.spec.ts-snapshots/<theme>-<page>.png`.
//
// Update baselines with `npm run test:visual:update` (passes
// Playwright's `--update-snapshots` flag).

// applyTheme forces the .dark / .light class on <html> via an
// init script so it takes effect before any React hydration. We
// also disable animations and transitions to keep pixel diffs
// deterministic across runs and platforms.
async function applyTheme(page: Page, theme: 'dark' | 'light'): Promise<void> {
  await page.addInitScript((mode: 'dark' | 'light') => {
    const root = document.documentElement;
    if (mode === 'dark') {
      root.classList.add('dark');
      root.classList.remove('light');
    } else {
      root.classList.remove('dark');
      root.classList.add('light');
    }
    // Disable transitions/animations for the duration of the capture.
    const style = document.createElement('style');
    style.textContent = `
      *, *::before, *::after {
        animation-duration: 0s !important;
        animation-delay: 0s !important;
        transition-duration: 0s !important;
        transition-delay: 0s !important;
      }
    `;
    document.head.appendChild(style);
  }, theme);
}

test.describe('Visual regression', () => {
  for (const theme of VISUAL_THEMES) {
    for (const pagePath of VISUAL_PAGES) {
      // Snapshot name: `${theme}-${slugified-path}.png`. The `.png`
      // extension is required by Playwright's `toHaveScreenshot`.
      const slug = pagePath.replace(/^\//, '').replace(/\//g, '-') || 'root';
      const snapshotName = `${theme}-${slug}.png`;

      test(`${theme} ${pagePath} matches the golden baseline`, async ({ page }) => {
        test.setTimeout(60_000);

        await applyTheme(page, theme);
        await page.goto(pagePath, { waitUntil: 'networkidle' });
        // Wait a beat for fonts, skeleton loaders, and any post-load
        // hydration to settle.
        await page.waitForTimeout(750);

        await expect(page).toHaveScreenshot(snapshotName, {
          fullPage: true,
          mask: [page.locator('tbody, footer, .recharts-wrapper, time, [data-live]')],
          // 0.5% pixel diff budget per page. Tighten in the future
          // if noise is lower than expected; loosen if it isn't.
          maxDiffPixelRatio: 0.005,
        });
      });
    }
  }
});
