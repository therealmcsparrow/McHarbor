// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The Swarm section's shape.
 *
 * Each group owns an ordered list of tabs; the sidebar entry for a group points
 * at its first tab, and every tab below it belongs to that group.
 *
 * Two things changed when Swarm became an add-on. Paths are now relative to the
 * plugin root rather than absolute (`services`, not `/swarm/services`) — the
 * host mounts one route for the whole subtree and the plugin resolves the rest.
 * And the group→sidebar mapping lives in `plugin.yaml` and `backend/main.go`
 * rather than being derived from this file, because the sidebar is built from
 * the manifest before any of this bundle has loaded. The four groups below and
 * the four `menu:` entries in the manifest are therefore the one place in this
 * plugin that must be kept in step by hand.
 */

export type SwarmTabId = 'overview' | 'services' | 'tasks' | 'nodes' | 'secrets' | 'configs' | 'stacks' | 'networks';

export type SwarmGroupId = 'overview' | 'workloads' | 'cluster' | 'config';

export type SwarmGroup = {
  id: SwarmGroupId;
  tabs: SwarmTabId[];
};

// The cluster root. Named so the lookups below have a total fallback rather
// than an index the compiler cannot prove is there.
export const swarmOverviewGroup: SwarmGroup = {
  id: 'overview',
  tabs: ['overview'],
};

export const swarmGroups: SwarmGroup[] = [
  swarmOverviewGroup,
  { id: 'workloads', tabs: ['services', 'tasks', 'stacks'] },
  { id: 'cluster', tabs: ['nodes', 'networks'] },
  { id: 'config', tabs: ['secrets', 'configs'] },
];

export const swarmTabs: SwarmTabId[] = swarmGroups.flatMap((group) => group.tabs);

/** `overview` is the plugin root; every other tab is a segment under it. */
export function swarmTabPath(tab: SwarmTabId): string {
  return tab === 'overview' ? '' : tab;
}

export function swarmGroupOf(tab: SwarmTabId): SwarmGroup {
  return swarmGroups.find((group) => group.tabs.includes(tab)) ?? swarmOverviewGroup;
}

/**
 * Read the active tab out of the plugin's splat.
 *
 * `''` is the overview; `services/abc123` is still the services tab, so a
 * detail page keeps its group. An unrecognised first segment falls back to the
 * overview rather than rendering nothing — a stale bookmark to a tab that no
 * longer exists should land somewhere, not on a blank page.
 */
export function swarmTabFromSplat(splat: string): SwarmTabId {
  const rest = splat.replace(/^\/+/, '').split('/')[0] ?? '';
  if (!rest) return 'overview';
  return swarmTabs.find((tab) => tab === rest) ?? 'overview';
}

/** True when the splat names a detail route below a list tab. */
export function isDetailSplat(splat: string): boolean {
  return splat.replace(/^\/+/, '').split('/').filter(Boolean).length > 1;
}
