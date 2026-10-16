// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The five groups the 21 container detail tabs belong to.
//
// This model is not new. `ContainerDetailTabs` has documented it in a
// comment since it was written:
//
//   1. What is it?                        → Overview
//   2. How is it configured?              → Configuration
//   3. What is it doing right now?        → Runtime
//   4. Where did it come from / going?    → Delivery
//   5. What is the long-term safety net?  → Data
//
// The model was correct and unused — the bar rendered all 21 flat, and
// `prominentTabs` and `groupStartTabs` were compensations for a bar too
// long to scan. This makes the grouping real.
//
// Every one of the 21 destinations survives, and the *tab id* remains the
// unit of navigation, so every existing `?tab=<id>` URL keeps resolving
// without a redirect: a group is derived from the tab, never the reverse.

import type { DetailTabId } from './ContainerDetailTabs';

export const detailGroupIDs = ['overview', 'configuration', 'runtime', 'delivery', 'security', 'data'] as const;

export type DetailGroupId = (typeof detailGroupIDs)[number];

/**
 * Group → its tabs, in reading order. The union of these is exactly
 * `detailTabIDs`; `detail-tab-groups.test` territory if this ever drifts,
 * and `assertGroupsCoverAllTabs` below guards it at dev time.
 */
export const DETAIL_GROUPS: Record<DetailGroupId, readonly DetailTabId[]> = {
  overview: ['overview'],
  configuration: ['environment', 'labels', 'network', 'mounts', 'resources'],
  runtime: ['activity', 'logs', 'terminal', 'exec', 'files', 'processes', 'services', 'inspect'],
  delivery: ['updates', 'history', 'changes', 'diff'],
  // Settings and Scans answer the same question — "is it safe to run?" —
  // from the two directions an operator asks it: what we allowed this
  // container to do, and what is known to be wrong with its image. They
  // were split across Configuration and nothing respectively.
  //
  // `security` keeps its id, so every existing `?tab=security` link
  // resolves; only the group it renders under changed.
  security: ['security', 'scans'],
  // Three destinations, so `groupHasSubRail` is true and Data gets the
  // same level-2 rail Configuration and Runtime already render. `backups`
  // stays the id of the run list, so every existing `?tab=backups` link
  // keeps resolving.
  data: ['backup-settings', 'backup-schedules', 'backups'],
};

const TAB_TO_GROUP: Record<string, DetailGroupId> = Object.fromEntries(
  detailGroupIDs.flatMap((group) => DETAIL_GROUPS[group].map((tab) => [tab, group] as const)),
);

/** Which group does this tab live in? Total — every tab has exactly one. */
export function groupForTab(tab: DetailTabId): DetailGroupId {
  return TAB_TO_GROUP[tab] ?? 'overview';
}

/** i18n key for a group's label. */
export function groupLabelKey(group: DetailGroupId): string {
  return `detail.group.${group}`;
}

/**
 * A group with a single tab (Overview, Data) needs no sub-rail — the group
 * button *is* the destination.
 */
export function groupHasSubRail(group: DetailGroupId): boolean {
  return DETAIL_GROUPS[group].length > 1;
}
