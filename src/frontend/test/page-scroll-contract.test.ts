// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// PageShell's contract (resources/layout/PageShell.tsx): a route declaring
// `scroll: "data"` does NOT scroll itself — the shell is overflow-hidden and the
// page supplies the one scroller, normally PageBody or TablePage.
//
// A data page that supplies neither is silently unreachable past the fold: no
// error, no overflow, the content is simply cut off. That is what happened to the
// workflows overview page, and this test is the guard that would have caught it.
//
// The failure mode this replaces was the reason PageShell exists at all: the old
// `isFlatCanvas` chain of nine pathname tests in AppLayout meant every new route
// had to remember a list in a file it had no reason to open, and forgetting was
// silent.

const FRONTEND = resolve(__dirname, '..');
const ROUTER_FILES = [resolve(FRONTEND, 'core/router.tsx'), resolve(FRONTEND, 'core/router/appRoutes.tsx')];
const LAZY_PAGES = resolve(FRONTEND, 'core/router/lazyPages.ts');

/** Anything that establishes a scroll region inside the page. */
const SCROLLERS = ['PageBody', 'TablePage', 'data-scroll-exempt', 'overflow-y-auto', 'overflow-auto'];

/**
 * Pages that legitimately fill the shell without scrolling. A full-bleed canvas
 * owns its own viewport: the node graph pans, the terminal scrolls its own
 * buffer. Both declare `padded: false` as well.
 */
const FULL_BLEED = new Set([
  'modules/workflows/pages/WorkflowEditorPage.tsx',
  'modules/terminal/pages/TerminalPage.tsx',
]);

/**
 * Pages whose scroller lives in a child component, so it is correct here and
 * this file cannot see it. Each was read before being listed — a guess belongs
 * in BASELINE, not here.
 */
const CHILD_OWNS_SCROLLER = new Set([
  // LogViewer is `h-full overflow-auto` (resources/components/LogViewer.tsx:88).
  'modules/logs/pages/LogsPage.tsx',
  // NotificationsTable claims the remaining height and scrolls its own rows, so
  // its toolbar and pagination band stay put. The page says so in a comment.
  'modules/notifications/pages/NotificationsPage.tsx',
]);

/**
 * Known-missing scrollers, pre-existing and not yet audited. **Shrink-only.**
 * Delete a line when the page gains a scroller; never add one to make a run
 * pass — a new data page without a scroller is a bug, not a baseline entry.
 *
 * Recorded 2026-09-28 alongside the workflows overview fix. Tracked in
 * .references/todo/2026-09-28-data-pages-without-a-scroller.md
 */
const BASELINE = new Set([
  // A centred `min-h-screen` setup card, not a list — a different shape, and it
  // clips only when the card outgrows the viewport.
  'modules/settings/database/pages/DatabaseSetupPage.tsx',
  // Nested full-height Tabs whose panels are `overflow-hidden`, and neither
  // NodesTab nor WidgetsTab introduces a scroller. Genuinely suspect, but the
  // fix belongs inside the tab content rather than at the page root.
  'modules/appstore/pages/StorePage.tsx',
]);

type DataPage = { rel: string; file: string; source: string };

function dataScrollPages(): DataPage[] {
  const lazyPages = readFileSync(LAZY_PAGES, 'utf8');
  const routeSources = ROUTER_FILES.map((file) => readFileSync(file, 'utf8')).join('\n');

  const modules = new Map<string, string>();
  // The lazy() call wraps onto a second line for longer paths, so the pattern has
  // to cross newlines rather than stop at one.
  for (const m of lazyPages.matchAll(/(\w+)\s*=[\s\S]{0,120}?import\(['"]([^'"]+)['"]\)/g)) {
    if (!modules.has(m[1]!)) modules.set(m[1]!, m[2]!);
  }

  // A route's handle is either an inline object or a const declared beside the table.
  const handles = new Map<string, string>();
  for (const m of routeSources.matchAll(/const (\w+)\s*=\s*(\{[^}]*\})\s*as const/g)) {
    handles.set(m[1]!, m[2]!);
  }

  const pages: DataPage[] = [];
  const seen = new Set<string>();
  for (const m of routeSources.matchAll(/pageRoute\(\s*['"][^'"]*['"],\s*P\.(\w+)\s*(?:,\s*(\w+|\{[^}]*\}))?\s*\)/g)) {
    const handle = m[2] ? (handles.get(m[2]) ?? m[2]) : '';
    if (!/scroll:\s*['"]data['"]/.test(handle)) continue;
    const spec = modules.get(m[1]!);
    if (!spec) continue;

    const rel = spec.replace(/^@modules\//, 'modules/').replace(/^@resources\//, 'resources/');
    const candidates = [`${rel}.tsx`, `${rel}.ts`];
    const found = candidates.find((c) => existsSync(resolve(FRONTEND, c)));
    if (!found || seen.has(found)) continue;
    seen.add(found);
    pages.push({
      rel: found,
      file: resolve(FRONTEND, found),
      source: readFileSync(resolve(FRONTEND, found), 'utf8'),
    });
  }
  return pages;
}

describe('page scroll contract', () => {
  it('finds the data-scroll routes it is meant to police', () => {
    // A regex that stops matching would make every assertion below vacuous.
    expect(dataScrollPages().length).toBeGreaterThan(20);
  });

  it('gives every data-scroll page a scroller, or records it as known-missing', () => {
    const offenders = dataScrollPages()
      .filter((page) => !FULL_BLEED.has(page.rel) && !CHILD_OWNS_SCROLLER.has(page.rel) && !BASELINE.has(page.rel))
      .filter((page) => !SCROLLERS.some((token) => page.source.includes(token)))
      .map((page) => page.rel);

    expect(
      offenders,
      'a "data" shell is overflow-hidden, so these pages are cut off past the fold. ' +
        'Wrap the scrollable content in <PageBody> (or use TablePage).',
    ).toEqual([]);
  });

  it('keeps the known-missing baseline shrink-only', () => {
    const pages = dataScrollPages();
    const stillMissing = [...BASELINE].filter((rel) => {
      const page = pages.find((p) => p.rel === rel);
      if (!page) return false;
      return !SCROLLERS.some((token) => page.source.includes(token));
    });

    expect(
      [...BASELINE].filter((rel) => !stillMissing.includes(rel)),
      'these pages now have a scroller (or no longer exist): delete them from BASELINE',
    ).toEqual([]);
  });

  it('holds the workflows pages this contract was written for', () => {
    const pages = dataScrollPages();
    for (const rel of ['modules/workflows/pages/WorkflowsPage.tsx', 'modules/workflows/pages/CustomNodesPage.tsx']) {
      const page = pages.find((p) => p.rel === rel);
      expect(page, `${rel} should be a data-scroll route`).toBeDefined();
      expect(page!.source, `${rel} must render PageBody`).toContain('<PageBody');
      // The shell is a flex column; a page that does not claim its height leaves
      // PageBody with nothing to be flex-1 of.
      expect(page!.source, `${rel} root must be a min-h-0 flex column`).toContain('min-h-0 flex-col');
    }
  });
});
