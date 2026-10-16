// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.
//
// Tiny CLI that reads the structured JSON summary written by the
// axe-core suite and prints either a markdown table (for PR
// comments) or a one-line summary (for CI logs).
//
// Usage (from `src/frontend/`):
//   tsx tests/a11y/report.ts <results.json> [--format=markdown|oneliner] [--exit-code]
//
// `--exit-code` exits non-zero when violations exist, which makes the
// script usable as a custom CI step independent of Playwright.

import fs from 'node:fs';
import path from 'node:path';

type AxeImpact = 'minor' | 'moderate' | 'serious' | 'critical';

type AxeSummary = {
  schemaVersion: number;
  generatedAt: string;
  pagesScanned: number;
  totalViolations: number;
  byImpact: Record<AxeImpact, number>;
  byPage: Array<{ page: string; violations: number }>;
  results: Array<{
    page: string;
    violations: Array<{
      id: string;
      impact: AxeImpact | null;
      description: string;
      helpUrl: string;
      nodes: Array<{ html: string; target: string[] }>;
    }>;
  }>;
};

function readSummary(filePath: string): AxeSummary {
  const abs = path.resolve(filePath);
  const text = fs.readFileSync(abs, 'utf8');
  const parsed = JSON.parse(text) as AxeSummary;
  if (parsed.schemaVersion !== 1) {
    throw new Error(`Unsupported schemaVersion ${parsed.schemaVersion}; expected 1`);
  }
  return parsed;
}

function formatOneliner(summary: AxeSummary): string {
  const impact = Object.entries(summary.byImpact)
    .filter(([, n]) => n > 0)
    .map(([k, n]) => `${n} ${k}`)
    .join(', ');
  const impactSuffix = impact ? ` (${impact})` : '';
  return `axe: ${summary.totalViolations} violations across ${summary.pagesScanned} pages${impactSuffix}`;
}

function formatMarkdown(summary: AxeSummary): string {
  const lines: string[] = [];
  lines.push(`### Accessibility scan`);
  lines.push('');
  lines.push(`- Pages scanned: **${summary.pagesScanned}**`);
  lines.push(`- Total violations (≥ serious): **${summary.totalViolations}**`);
  if (summary.totalViolations > 0) {
    lines.push(
      `- By impact: ${Object.entries(summary.byImpact)
        .filter(([, n]) => n > 0)
        .map(([k, n]) => `${n} ${k}`)
        .join(', ')}`,
    );
    lines.push('');
    lines.push(`| Page | Rule | Impact | Description |`);
    lines.push(`|------|------|--------|-------------|`);
    for (const r of summary.results) {
      for (const v of r.violations) {
        const desc = v.description.replace(/\|/g, '\\|');
        lines.push(`| \`${r.page}\` | \`${v.id}\` | ${v.impact ?? 'n/a'} | ${desc} |`);
      }
    }
  } else {
    lines.push('');
    lines.push('No serious or critical accessibility violations detected. ✅');
  }
  lines.push('');
  lines.push(`Generated at ${summary.generatedAt}.`);
  return lines.join('\n');
}

function main(): void {
  const argv = process.argv.slice(2);
  if (argv.length === 0 || argv.includes('--help') || argv.includes('-h')) {
    console.error('Usage: tsx tests/a11y/report.ts <results.json> [--format=markdown|oneliner] [--exit-code]');
    process.exit(argv.length === 0 ? 2 : 0);
  }
  const filePath = argv[0];
  if (!filePath) {
    console.error('Usage: tsx tests/a11y/report.ts <results.json> [--format=markdown|oneliner] [--exit-code]');
    process.exit(2);
  }
  const fmtArg = argv.find((a) => a.startsWith('--format='));
  const format = (fmtArg?.split('=')[1] ?? 'oneliner') as 'markdown' | 'oneliner';
  const exitCode = argv.includes('--exit-code');

  const summary = readSummary(filePath);
  const out = format === 'markdown' ? formatMarkdown(summary) : formatOneliner(summary);
  process.stdout.write(out + '\n');
  if (exitCode && summary.totalViolations > 0) {
    process.exit(1);
  }
}

main();
