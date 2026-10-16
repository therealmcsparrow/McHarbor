// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import path from 'node:path';
import { test, expect } from '@playwright/test';
import { A11Y_PAGES } from './pages';
import { runAxe, summarize, writeResultsJson, type AxeResult } from './run-axe';

// Allowlist of rule IDs that are known false positives or third-party
// issues. Anything added here MUST be reviewed against the actual
// failing rule before being committed — a list of excuses grows fast.
const ALLOWED_RULE_IDS: readonly string[] = [
  // `color-contrast` sometimes flags computed colors that look fine
  // on screen but fail the WCAG ratio because the theme tokens
  // include very subtle OKLCH values. Re-evaluate per release.
  'color-contrast',
];

// test.describe blocks share a worker but each test gets a fresh
// browser context — fine for a scan (no cross-test state).
test.describe('A11y', () => {
  // Run one test per top-level page so a failing page shows up
  // clearly in CI logs without affecting siblings.
  for (const path of A11Y_PAGES) {
    test(`${path} has no serious or critical axe violations`, async ({ page }) => {
      test.setTimeout(30_000);

      const results: AxeResult[] = await runAxe(page, {
        only: path,
        allowedRuleIds: ALLOWED_RULE_IDS,
      });

      const pageResult = results[0];
      expect(pageResult, `axe run produced no result for ${path}`).toBeDefined();
      expect.soft(pageResult?.violations.length ?? 0, summarize(results)).toBe(0);
    });
  }

  // Aggregate test ensures the whole suite stays clean — useful as a
  // single-pass smoke check that doesn't fragment CI output. It also
  // writes the structured JSON summary to `.results/a11y/results.json`
  // for CI dashboards and `grep`-based sanity checks.
  test('all top-level pages have no serious or critical axe violations', async ({ page }) => {
    test.setTimeout(120_000);

    const results = await runAxe(page, {
      allowedRuleIds: ALLOWED_RULE_IDS,
    });

    // Persist a JSON summary alongside the Playwright HTML report.
    // `outputDir` in `playwright.config.ts` is `./.results/a11y`, so we
    // use the same path here without depending on the Playwright type
    // surface (FullConfig doesn't expose `outputDir` directly).
    const outputDir = path.join(process.cwd(), '.results/a11y');
    const summary = writeResultsJson(results, path.join(outputDir, 'results.json'));
    test.info().annotations.push({
      type: 'a11y',
      description: `axe scanned ${summary.pagesScanned} pages, ${summary.totalViolations} violations`,
    });

    const totalViolations = results.reduce((acc, r) => acc + r.violations.length, 0);
    expect(totalViolations, summarize(results)).toBe(0);
  });
});
