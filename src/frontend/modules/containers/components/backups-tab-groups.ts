// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The four groups the five backup destinations belong to.
//
// This mirrors `modules/security/components/security-tab-groups.ts` and
// `modules/settings/components/settings-tab-groups.ts`. The Backups page
// grew a fifth tab and became the flat strip those two pages already left
// behind: five labels in a row, none of them saying which are about what
// happened and which are about what will happen.
//
// The questions the four groups answer:
//
//   1. Is my data protected?      -> Overview
//   2. What has happened?         -> Activity (runs, logs)
//   3. What is scheduled?         -> Plans
//   4. How is it configured?      -> Settings
//
// The *tab id* stays the unit of navigation, so every `?tab=<id>` keeps
// resolving — the three ids that predate the grouping included. A group is
// derived from the tab, never the reverse.

import {
  IconCalendarClock,
  IconFileText,
  IconHistory,
  IconLayoutDashboard,
  IconSettings,
  IconShieldCheck,
  type TablerIcon,
} from '@tabler/icons-react';

/** Canonical id list, in group reading order. */
export const backupsTabIDs = [
  // 1. Is my data protected?
  'overview',
  // 2. What has happened?
  'runs',
  'logs',
  // 3. What is scheduled?
  'plans',
  // 4. How is it configured?
  'settings',
] as const;

export type BackupsTabId = (typeof backupsTabIDs)[number];

export const backupsGroupIDs = ['overview', 'activity', 'plans', 'settings'] as const;

export type BackupsGroupId = (typeof backupsGroupIDs)[number];

/** Group -> its destinations, in reading order. The union is `backupsTabIDs`. */
export const BACKUPS_GROUPS: Record<BackupsGroupId, readonly BackupsTabId[]> = {
  overview: ['overview'],
  activity: ['runs', 'logs'],
  plans: ['plans'],
  settings: ['settings'],
};

const TAB_TO_GROUP: Record<string, BackupsGroupId> = Object.fromEntries(
  backupsGroupIDs.flatMap((group) => BACKUPS_GROUPS[group].map((tab) => [tab, group] as const)),
);

/** Which group does this destination live in? Total — every tab has one. */
export function groupForBackupsTab(tab: BackupsTabId): BackupsGroupId {
  return TAB_TO_GROUP[tab] ?? 'overview';
}

/** i18n key for a group's label, in the `containers` namespace. */
export function backupsGroupLabelKey(group: BackupsGroupId): string {
  return `backups.sections.${group}`;
}

/**
 * A group with a single destination renders no rail — the group button
 * *is* the destination. Only Activity has two, which is the whole reason
 * the second level exists at all.
 */
export function backupsGroupHasSubRail(group: BackupsGroupId): boolean {
  return BACKUPS_GROUPS[group].length > 1;
}

/**
 * i18n key for a destination's label. Runs, Plans and Settings keep the
 * keys they already had, so no translation is re-typed to add the grouping.
 */
export const backupsTabLabelKeys: Record<BackupsTabId, string> = {
  overview: 'backups.tabOverview',
  runs: 'backups.tabRuns',
  logs: 'backups.logs.title',
  plans: 'backups.tabPlans',
  settings: 'backups.tabSettings',
};

/** i18n key for the one-line description under the hero title. */
export const backupsTabDescriptionKeys: Record<BackupsTabId, string> = {
  overview: 'backups.descriptions.overview',
  runs: 'backups.descriptions.runs',
  logs: 'backups.descriptions.logs',
  plans: 'backups.descriptions.plans',
  settings: 'backups.descriptions.settings',
};

export const backupsTabIcons: Record<BackupsTabId, TablerIcon> = {
  overview: IconLayoutDashboard,
  runs: IconHistory,
  logs: IconFileText,
  plans: IconCalendarClock,
  settings: IconSettings,
};

export const backupsGroupIcons: Record<BackupsGroupId, TablerIcon> = {
  overview: IconShieldCheck,
  activity: IconHistory,
  plans: IconCalendarClock,
  settings: IconSettings,
};

if (import.meta.env.DEV) {
  const covered = backupsGroupIDs.flatMap((group) => BACKUPS_GROUPS[group]);
  const missing = backupsTabIDs.filter((tab) => !covered.includes(tab));
  if (missing.length > 0) {
    console.warn(`[backups-tab-groups] destinations outside every group: ${missing.join(', ')}`);
  }
}
