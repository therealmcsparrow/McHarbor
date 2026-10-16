// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import fs from 'node:fs';
import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';
import type { Result, ImpactValue, NodeResult } from 'axe-core';
import { A11Y_PAGES } from './pages';

export type AxeImpact = NonNullable<ImpactValue>;

export type AxeViolation = {
  id: string;
  impact: AxeImpact | null;
  description: string;
  helpUrl: string;
  nodes: { html: string; target: string[] }[];
};

export type AxeResult = {
  page: string;
  violations: AxeViolation[];
};

export type RunAxeOptions = {
  /** Restrict the run to a single page (matched by path). */
  only?: string;
  /** axe-core tags. Defaults to WCAG 2.0 + 2.1 A + AA. */
  tags?: string[];
  /** Restrict the scan to a specific selector (default: full document). */
  rootSelector?: string;
  /** Allowlist of violation IDs that are known false positives. */
  allowedRuleIds?: readonly string[];
  /** Minimum impact to include in the result. Default: `'serious'`. */
  minImpact?: AxeImpact;
};

const DEFAULT_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] as const;
const DEFAULT_MIN_IMPACT: AxeImpact = 'serious';

const IMPACT_RANK: Record<AxeImpact, number> = {
  minor: 0,
  moderate: 1,
  serious: 2,
  critical: 3,
};

// shapeNode collapses a NodeResult to the small subset that
// downstream consumers (Playwright assertions, formatter) need.
function shapeNode(node: NodeResult): AxeViolation['nodes'][number] {
  return {
    html: node.html,
    target: Array.isArray(node.target) ? node.target.map(String) : [String(node.target)],
  };
}

// shapeViolation turns an axe-core Result into the project's own
// narrower violation type. Keeping this layer lets callers depend
// only on the local shape (not the full axe-core Result tree).
function shapeViolation(result: Result): AxeViolation {
  return {
    id: result.id,
    impact: result.impact ?? null,
    description: result.description,
    helpUrl: result.helpUrl,
    nodes: result.nodes.map(shapeNode),
  };
}

// formatViolation produces a single readable line per violation so
// CI logs are easier to skim than the raw JSON blob.
export function formatViolation(v: AxeViolation): string {
  const target = v.nodes[0]?.target.join(' ') ?? '(no target)';
  const impact = v.impact ?? 'unknown';
  return `[${impact}] ${v.id} — ${v.description} @ ${target} (${v.helpUrl})`;
}

// runAxe injects axe-core into the supplied Playwright page, runs
// the configured rule set, and returns the filtered violations.
//
// The function does not assert or throw — callers decide how to
// react. `tests/a11y/run-axe.spec.ts` is the canonical caller that
// turns the result into a Playwright `expect()` failure.
export async function runAxe(page: Page, options: RunAxeOptions = {}): Promise<AxeResult[]> {
  const tags = options.tags ?? [...DEFAULT_TAGS];
  const minImpact = options.minImpact ?? DEFAULT_MIN_IMPACT;
  const allowedRuleIds = new Set(options.allowedRuleIds ?? []);
  const pagesToScan = options.only ? A11Y_PAGES.filter((p) => p === options.only) : [...A11Y_PAGES];

  const results: AxeResult[] = [];

  for (const path of pagesToScan) {
    // Playwright resolves relative URLs against the `baseURL` set
    // in `use.baseURL` of playwright.config.ts. Absolute URLs are
    // passed through verbatim so the function can be re-used for
    // external checks if needed later.
    await page.goto(path, { waitUntil: 'networkidle' });

    let builder = new AxeBuilder({ page }).withTags(tags);
    if (options.rootSelector) {
      builder = builder.include(options.rootSelector);
    }
    if (allowedRuleIds.size > 0) {
      builder = builder.disableRules([...allowedRuleIds]);
    }

    const scan = await builder.analyze();

    const filtered = scan.violations.filter((v) => {
      if (!v.impact) return false;
      if (IMPACT_RANK[v.impact] < IMPACT_RANK[minImpact]) return false;
      return true;
    });

    results.push({
      page: path,
      violations: filtered.map(shapeViolation),
    });
  }

  return results;
}

// summarize is a tiny helper used by the spec to build the failure
// message — the sum of every violation across every page, formatted
// one per line.
export function summarize(results: AxeResult[]): string {
  const total = results.reduce((acc, r) => acc + r.violations.length, 0);
  if (total === 0) {
    return `axe: 0 violations across ${results.length} pages`;
  }
  const lines = [`axe: ${total} violations across ${results.length} pages`];
  for (const r of results) {
    if (r.violations.length === 0) continue;
    lines.push(`\n  ${r.page}:`);
    for (const v of r.violations) {
      lines.push(`    ${formatViolation(v)}`);
    }
  }
  return lines.join('\n');
}

// AxeRunSummary is the trimmed-down payload written to disk for CI
// dashboards and grep-based sanity checks. The full `AxeResult[]`
// shape is preserved so consumers can rebuild the failure summary
// without re-running axe.
export type AxeRunSummary = {
  schemaVersion: 1;
  generatedAt: string;
  pagesScanned: number;
  totalViolations: number;
  byImpact: Record<AxeImpact, number>;
  byPage: Array<{ page: string; violations: number }>;
  results: AxeResult[];
};

// writeResultsJson is called by the spec after every run so the
// artefacts under `.results/a11y/` include a structured file in
// addition to the Playwright HTML report. CI dashboards can then
// grep the JSON for total violations / per-page counts / impact
// histograms without parsing axe-core internals.
export function writeResultsJson(results: AxeResult[], outPath: string): AxeRunSummary {
  const byImpact: Record<AxeImpact, number> = {
    minor: 0,
    moderate: 0,
    serious: 0,
    critical: 0,
  };
  for (const r of results) {
    for (const v of r.violations) {
      if (v.impact) byImpact[v.impact] += 1;
    }
  }

  const summary: AxeRunSummary = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    pagesScanned: results.length,
    totalViolations: results.reduce((acc, r) => acc + r.violations.length, 0),
    byImpact,
    byPage: results.map((r) => ({ page: r.page, violations: r.violations.length })),
    results,
  };

  fs.mkdirSync(outPath.replace(/[^/\\]+$/, ''), { recursive: true });
  fs.writeFileSync(outPath, JSON.stringify(summary, null, 2), 'utf8');
  return summary;
}
