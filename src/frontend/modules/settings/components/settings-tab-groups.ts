// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The six groups the settings sections belong to.
//
// This mirrors `containers/components/detail-tab-groups.ts`. Settings had
// the same problem the container detail bar had before it was grouped: a
// flat, horizontally scrolling strip where finding "Logging" meant
// reading every label in sequence. The questions a settings
// section answers are not fourteen different questions, they are six:
//
//   1. How does the app look and behave?   -> General
//   2. What is it wired up to?             -> Integrations
//   3. What does it tell me about?         -> Monitoring
//   4. What does it do on its own?         -> Automation
//   5. What keeps itself current?          -> Updates
//   6. What is it running as?              -> System
//
// The *tab id* remains the unit of navigation, so every existing
// `?tab=<id>` URL keeps resolving: a group is derived from the tab, never
// the reverse. Nothing has been renamed or dropped since; `retention` was
// added when data retention moved out of General onto a section of its own,
// and `notifications` when the toast defaults did the same. `updates` was the
// one exception: it was a single System section stacking three unrelated
// audiences (container images, remote agents, McHarbor itself), so it became a
// group with one section per audience. `?tab=updates` still resolves — the
// settings page aliases it to `updates-mcharbor`.

import {
  IconAlertTriangle,
  IconAnchor,
  IconArchive,
  IconBell,
  IconBellRinging,
  IconBoxMultiple,
  IconCamera,
  IconCertificate,
  IconClock,
  IconCloudLock,
  IconDatabase,
  IconDatabaseCog,
  IconFlask2,
  IconInfoCircle,
  IconMail,
  IconMessageCircle,
  IconPackages,
  IconPalette,
  IconPlug,
  IconRefresh,
  IconRobot,
  IconServer2,
  IconServerCog,
  IconSettings,
  IconSparkles,
  IconWebhook,
  type TablerIcon,
} from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';

/**
 * Canonical id list, in group reading order. Kept exported because
 * `useTabFromUrl` and the command palette both take the flat list.
 */
export const settingsTabIDs = [
  // 1. How does the app look and behave?
  'general',
  'themes',
  'notifications',
  // 2. What is it wired up to?
  'ai',
  'communications',
  'storage',
  'snapshots',
  'logging',
  'catalogs',
  // 3. What does it tell me about?
  'alerts',
  'retention',
  // 4. What does it do on its own?
  'schedules',
  'webhooks',
  // 5. What keeps itself current?
  'updates-containers',
  'updates-agents',
  'updates-host',
  'updates-mcharbor',
  // 6. What is it running as?
  'host-services',
  'database',
  'instance-backup',
  'agent',
  'license',
  'about',
  'contact',
  'demo',
] as const;

export type SettingsTabId = (typeof settingsTabIDs)[number];

export const settingsGroupIDs = ['general', 'integrations', 'monitoring', 'automation', 'updates', 'system'] as const;

export type SettingsGroupId = (typeof settingsGroupIDs)[number];

/**
 * The permission each section's own routes are gated on.
 *
 * Every one of these keys was already enforced on the backend, so a viewer
 * without it opened the section and met an empty panel or a 403 toast. The
 * rail now renders the section disabled with a tooltip naming the key, which
 * is the difference between "nothing here" and "you cannot see this".
 *
 * Sections with no entry are readable by anyone who can reach Settings at
 * all (General, Themes, About), or have no dedicated key to gate on yet
 * (Snapshots, Catalogs, Agent) — those three want a key of their own before
 * they can be gated honestly.
 */
export const SETTINGS_TAB_PERMISSIONS: Partial<Record<SettingsTabId, string>> = {
  // Toast defaults only, backed by /settings/* - so it takes the settings key.
  // It used to be gated on notifications.view, a key none of its routes used
  // and which no longer exists: that module was the legacy channel CRUD,
  // superseded by Communications and removed.
  notifications: PERM.SETTINGS_VIEW,
  ai: PERM.AI_ASSISTANT_VIEW,
  communications: PERM.COMMUNICATIONS_VIEW,
  storage: PERM.STORAGE_LOCATIONS_VIEW,
  logging: PERM.AUDIT_VIEW,
  alerts: PERM.ALERTS_VIEW,
  retention: PERM.SETTINGS_VIEW,
  schedules: PERM.SCHEDULES_VIEW,
  webhooks: PERM.WEBHOOKS_VIEW,
  'updates-containers': PERM.UPDATES_VIEW,
  'updates-agents': PERM.UPDATES_VIEW,
  'updates-host': PERM.HOST_SELF_VIEW,
  'updates-mcharbor': PERM.UPDATES_VIEW,
  'host-services': PERM.HOST_SELF_VIEW,
  database: PERM.DATABASE_VIEW,
  'instance-backup': PERM.INSTANCE_BACKUP_VIEW,
  license: PERM.LICENSE_VIEW,
};

/**
 * Group -> its sections, in reading order. The union of these is exactly
 * `settingsTabIDs`; `assertGroupsCoverAllTabs` guards that at dev time.
 */
export const SETTINGS_GROUPS: Record<SettingsGroupId, readonly SettingsTabId[]> = {
  general: ['general', 'themes', 'notifications'],
  integrations: ['ai', 'communications', 'storage', 'snapshots', 'logging', 'catalogs'],
  monitoring: ['alerts', 'retention'],
  automation: ['schedules', 'webhooks'],
  updates: ['updates-containers', 'updates-agents', 'updates-host', 'updates-mcharbor'],
  system: ['host-services', 'database', 'instance-backup', 'agent', 'license', 'about', 'contact', 'demo'],
};

const TAB_TO_GROUP: Record<string, SettingsGroupId> = Object.fromEntries(
  settingsGroupIDs.flatMap((group) => SETTINGS_GROUPS[group].map((tab) => [tab, group] as const)),
);

/** Which group does this section live in? Total — every section has one. */
export function groupForSettingsTab(tab: SettingsTabId): SettingsGroupId {
  return TAB_TO_GROUP[tab] ?? 'general';
}

/** i18n key for a group's label, in the `settings` namespace. */
export function settingsGroupLabelKey(group: SettingsGroupId): string {
  return `groups.${group}`;
}

/**
 * A group with a single section needs no sub-rail — the group button is
 * already the destination. None of the six is single today; the helper
 * keeps that from becoming a layout bug if one ever is. (Monitoring came
 * closest, and stopped being single when Data retention moved out of
 * General and joined Alerts.)
 */
export function settingsGroupHasSubRail(group: SettingsGroupId): boolean {
  return SETTINGS_GROUPS[group].length > 1;
}

export const settingsTabIcons: Record<SettingsTabId, TablerIcon> = {
  general: IconSettings,
  themes: IconPalette,
  notifications: IconBell,
  ai: IconSparkles,
  communications: IconMessageCircle,
  catalogs: IconPackages,
  storage: IconDatabase,
  snapshots: IconCamera,
  alerts: IconAlertTriangle,
  retention: IconDatabaseCog,
  logging: IconCloudLock,
  schedules: IconClock,
  webhooks: IconWebhook,
  'updates-containers': IconBoxMultiple,
  'updates-agents': IconRobot,
  'updates-host': IconPackages,
  'updates-mcharbor': IconAnchor,
  'host-services': IconServerCog,
  database: IconServer2,
  'instance-backup': IconArchive,
  demo: IconFlask2,
  agent: IconRobot,
  license: IconCertificate,
  about: IconInfoCircle,
  contact: IconMail,
};

export const settingsGroupIcons: Record<SettingsGroupId, TablerIcon> = {
  general: IconSettings,
  integrations: IconPlug,
  monitoring: IconBellRinging,
  automation: IconClock,
  updates: IconRefresh,
  system: IconServerCog,
};

if (import.meta.env.DEV) {
  const covered = settingsGroupIDs.flatMap((group) => SETTINGS_GROUPS[group]);
  const missing = settingsTabIDs.filter((tab) => !covered.includes(tab));
  if (missing.length > 0) {
    console.warn(`[settings-tab-groups] sections outside every group: ${missing.join(', ')}`);
  }
}
