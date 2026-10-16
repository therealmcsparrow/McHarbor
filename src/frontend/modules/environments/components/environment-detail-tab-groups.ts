// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Grouping for the environment detail tabs.
//
// The container detail page groups its 21 tabs into a two-level bar so the
// first row always fits; the environment page has nine, which fit as a flat
// strip, but a *different* bar on the sibling page is one more thing to
// learn. The same shape is used here so the two pages read as one design:
//
//   1. What is it?              → Overview
//   2. How is it configured?    → Settings   (activity, automation, retention)
//
// `access` is built and routed but deliberately absent from every group, so it
// is unreachable from the rail until it is put back in `settings`.
//   3. What is the host doing?  → Host       (host, terminal, logs, docker,
//                                             processes, services, updates —
//                                             services only for a native agent)
//
// `environmentDetailTabs` stays the canonical list and the unit of
// navigation — `?tab=<id>` resolves exactly as it did, because the group is
// derived from the tab, not the other way round.

import type { EnvironmentDetailTab } from '../hooks/useEnvironmentDetailState';

export const ENVIRONMENT_DETAIL_GROUPS = {
  overview: ['overview'],
  settings: ['activity', 'automation', 'retention'],
  host: ['host', 'terminal', 'logs', 'docker', 'processes', 'services', 'updates'],
} as const satisfies Record<string, readonly EnvironmentDetailTab[]>;

export type EnvironmentDetailGroupId = keyof typeof ENVIRONMENT_DETAIL_GROUPS;

export const environmentDetailGroupIDs = Object.keys(ENVIRONMENT_DETAIL_GROUPS) as EnvironmentDetailGroupId[];

export function groupForTab(tab: EnvironmentDetailTab): EnvironmentDetailGroupId {
  for (const group of environmentDetailGroupIDs) {
    const members: readonly EnvironmentDetailTab[] = ENVIRONMENT_DETAIL_GROUPS[group];
    if (members.includes(tab)) return group;
  }
  return 'overview';
}

export function groupLabelKey(group: EnvironmentDetailGroupId): string {
  return `detail.group.${group}`;
}

/** A single-tab group needs no sub-rail — the group button is the destination. */
export function groupHasSubRail(group: EnvironmentDetailGroupId): boolean {
  return ENVIRONMENT_DETAIL_GROUPS[group].length > 1;
}
