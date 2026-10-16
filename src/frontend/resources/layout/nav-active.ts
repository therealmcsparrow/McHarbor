// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NavItem } from './sidebar-data';

/**
 * Every path that may light this item: its declared `matchPaths` when it owns a
 * group of sibling routes, otherwise its own `to`.
 */
export function navItemMatchPaths(item: NavItem): string[] {
  return item.matchPaths?.length ? item.matchPaths : [item.to];
}

function matchLength(pathname: string, candidate: string): number {
  if (candidate === '/dashboard') {
    // Home is exact-only — every URL is a descendant of nothing, but `/` is
    // rendered as the dashboard so it counts as a hit.
    return pathname === '/dashboard' || pathname === '/' ? candidate.length : -1;
  }
  if (candidate === '/') return pathname === '/' ? candidate.length : -1;
  if (pathname === candidate) return candidate.length;
  return pathname.startsWith(`${candidate}/`) ? candidate.length : -1;
}

/**
 * Resolve which nav item owns the current route, longest match wins.
 *
 * A plain prefix test lights every ancestor: `/plugins/addon-kubernetes` (Cluster
 * overview) stays lit on `/plugins/addon-kubernetes/pods`, and `/plugins/addon-reporting`
 * (Report Studio) stays lit on
 * `/plugins/addon-reporting/library`. Scoring the candidates and keeping only the most
 * specific one means a parent hands the highlight to its child instead of
 * sharing it. Ties are impossible — two items cannot declare the same path.
 *
 * Returns the winning item's `to`, or `null` when the route is outside the nav.
 */
export function resolveActiveNavPath(pathname: string, items: NavItem[]): string | null {
  let bestTo: string | null = null;
  let bestLen = -1;

  for (const item of items) {
    for (const candidate of navItemMatchPaths(item)) {
      const len = matchLength(pathname, candidate);
      if (len > bestLen) {
        bestLen = len;
        bestTo = item.to;
      }
    }
  }

  return bestTo;
}
