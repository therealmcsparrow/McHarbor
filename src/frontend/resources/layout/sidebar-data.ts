// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { IconProps } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import {
  IconLayoutDashboard,
  IconFlame,
  IconBook2,
  IconBox,
  IconPhoto,
  IconDeviceFloppy,
  IconNetwork,
  IconStack2,
  IconTerminal,
  IconArchive,
  IconFileText,
  IconHistory,
  IconWorld,
  IconGitBranch,
  IconActivity,
  IconClipboardList,
  IconRefresh,
  IconApps,
  IconGitMerge,
  IconRocket,
  IconShield,
  IconBuildingSkyscraper,
  IconBrandDocker,
  IconTool,
  IconTruckDelivery,
  IconGavel,
  IconPuzzle,
  IconClipboardCheck,
  IconServer2,
  IconVariable,
} from '@tabler/icons-react';

export type IconType = React.ComponentType<IconProps>;

/**
 * A sidebar nav entry.
 *
 * `permission` is optional — when set, the link is hidden from users
 * who lack the permission. The check uses the same RBAC catalog as
 * RequirePermission; this is a UX hide, not a security gate (the
 * backend endpoint still enforces its own check on every request).
 */
export type NavItem = {
  to: string;
  labelKey: string;
  icon: IconType;
  /** Permission key, or any-of list. Omit to show to everyone. */
  permission?: string | string[];
  /**
   * Paths that should mark this item active, when `to` alone cannot say it.
   * A group of sibling routes points every member at one item; without
   * this, NavLink's prefix rule would light the wrong one.
   * Matched exactly, or as a prefix so detail routes stay on their group.
   */
  matchPaths?: string[];
};

export type NavSection = {
  id: string;
  // When labelKey is empty the section renders no header (used for
  // the dashboard sentinel, which sits above all grouped sections).
  labelKey?: string;
  /**
   * Stands in for the whole group on the collapsed rail, where there is
   * no room for a text header and the items fold into a popup menu.
   * Sections without one fall back to a generic folder glyph.
   */
  icon?: IconType;
  items: NavItem[];
};

// ── Four groups ──────────────────────────────────────────────────────
//
// Seven sections folded to four. Every destination that existed before
// appears exactly once here — this is a regrouping, not a reduction, and
// no route became harder to reach.
//
//   Workloads   what you run
//   Operate     what you do to it while it runs
//   Govern      who may do it, and what happened
//   Deliver     how it gets there
//
// Deliver reads last because it is the only group whose destinations are
// about *getting somewhere else* — environments, git, gitops, the store.
// It is also the one group `buildSections` deliberately does not return:
// `Sidebar.tsx` appends it after the plugin sections so an installed
// plugin can never push it back up the list. See the note on
// `buildSections` below.
//
// Where things moved:
//   observability  → Operate   (terminal, logs, lifecycle, activity)
//   operations     → Operate   (backups, reconciler, workflows)
//   integrations   → Deliver   (environments, git, gitops, store)
//   security       → Govern    (audit, security scans)  [access & roles → dock]
//   enterprise     → its own Enterprise section (overview, HA, compliance, …)
//
// `/security` (access & roles) used to appear twice — once in the bottom
// dock and once implicitly as the parent of the scans page. It resolves to
// the dock, which carries Docker, Notifications, Security and Settings;
// Govern keeps the scans page. The destination is unchanged and still one
// click away.
//
// Defers to `sidebar-and-tabs-modern-ux-plan.md` Phase 1 for the
// collapse/persist behaviour, which this only supplies section ids for.

export const dockerSection: NavSection = {
  id: 'docker',
  labelKey: 'nav.section.docker',
  icon: IconBrandDocker,
  items: [
    { to: '/containers', labelKey: 'nav.containers', icon: IconBox, permission: PERM.CONTAINERS_VIEW },
    { to: '/stacks', labelKey: 'nav.stacks', icon: IconStack2, permission: PERM.STACKS_VIEW },
    { to: '/images', labelKey: 'nav.images', icon: IconPhoto, permission: PERM.IMAGES_VIEW },
    { to: '/volumes', labelKey: 'nav.volumes', icon: IconDeviceFloppy, permission: PERM.VOLUMES_VIEW },
    { to: '/networks', labelKey: 'nav.networks', icon: IconNetwork, permission: PERM.NETWORKS_VIEW },
  ],
};

export const operateSection: NavSection = {
  id: 'operate',
  labelKey: 'nav.section.operate',
  icon: IconTool,
  items: [
    { to: '/logs', labelKey: 'nav.logs', icon: IconFileText, permission: PERM.LOGS_VIEW },
    { to: '/terminal', labelKey: 'nav.terminal', icon: IconTerminal, permission: PERM.TERMINAL_ACCESS },
    { to: '/activity', labelKey: 'nav.activity', icon: IconActivity, permission: PERM.ACTIVITY_VIEW },
    { to: '/lifecycle', labelKey: 'nav.logging', icon: IconHistory, permission: PERM.EVENTS_VIEW },
    { to: '/backups', labelKey: 'nav.backups', icon: IconArchive, permission: PERM.BACKUPS_VIEW },
    { to: '/reconciler', labelKey: 'nav.reconciler', icon: IconRefresh, permission: PERM.RECONCILER_VIEW },
    { to: '/incidents', labelKey: 'nav.incidents', icon: IconFlame, permission: PERM.INCIDENTS_VIEW },
    { to: '/runbooks', labelKey: 'nav.runbooks', icon: IconBook2, permission: PERM.RUNBOOKS_VIEW },
    { to: '/workflows', labelKey: 'nav.workflows', icon: IconGitMerge, permission: PERM.WORKFLOWS_VIEW },
  ],
};

export const deliverSection: NavSection = {
  id: 'deliver',
  labelKey: 'nav.section.deliver',
  icon: IconTruckDelivery,
  items: [
    { to: '/environments', labelKey: 'nav.environments', icon: IconWorld, permission: PERM.ENVIRONMENTS_VIEW },
    { to: '/git', labelKey: 'nav.git', icon: IconGitBranch, permission: PERM.GIT_VIEW },
    { to: '/gitops', labelKey: 'nav.gitops', icon: IconRocket, permission: PERM.GITOPS_VIEW },
    { to: '/store', labelKey: 'nav.store', icon: IconApps, permission: PERM.STORE_APPS_VIEW },
  ],
};

/**
 * Release gate for the Enterprise area.
 *
 * `/enterprise` routes and renders either way — this only controls the
 * two places that *offer* it (the Enterprise sidebar section and the
 * Ctrl+K command palette). Turning it off withdraws both without touching
 * the routes, the section's definition or its labels in all six locales.
 *
 * This is a shipping decision, not a permission check: a user who lacks
 * a permission gets a disabled affordance and a tooltip naming what they
 * need, which is the opposite of what an unreleased feature wants.
 */
export const ENTERPRISE_NAV_ENABLED = true;

export const enterpriseNavItem: NavItem = {
  to: '/enterprise',
  labelKey: 'nav.enterprise',
  icon: IconBuildingSkyscraper,
  permission: PERM.SETTINGS_VIEW,
};

export const enterpriseSection: NavSection = {
  id: 'enterprise',
  labelKey: 'nav.section.enterprise',
  icon: IconBuildingSkyscraper,
  items: [
    {
      to: '/enterprise',
      labelKey: 'nav.enterpriseOverview',
      icon: IconBuildingSkyscraper,
      permission: PERM.SETTINGS_VIEW,
    },
    { to: '/enterprise/ha', labelKey: 'nav.highAvailability', icon: IconServer2, permission: PERM.CLUSTER_VIEW },
    {
      to: '/enterprise/compliance',
      labelKey: 'nav.compliance',
      icon: IconClipboardCheck,
      permission: PERM.COMPLIANCE_VIEW,
    },
    {
      to: '/enterprise/compliance/packs',
      labelKey: 'nav.compliancePacks',
      icon: IconClipboardCheck,
      permission: PERM.COMPLIANCE_VIEW,
    },
    {
      to: '/enterprise/environment-variables',
      labelKey: 'nav.environmentVariables',
      icon: IconVariable,
      permission: PERM.SETTINGS_VIEW,
    },
  ],
};

export const governSection: NavSection = {
  id: 'govern',
  labelKey: 'nav.section.govern',
  icon: IconGavel,
  items: [
    { to: '/audit', labelKey: 'nav.audit', icon: IconClipboardList, permission: PERM.AUDIT_VIEW },
    {
      to: '/security/scans',
      labelKey: 'nav.securityScans',
      icon: IconShield,
      permission: PERM.SCANS_VIEW,
    },
  ],
};

export const dashboardItem: NavItem = {
  to: '/dashboard',
  labelKey: 'nav.dashboard',
  icon: IconLayoutDashboard,
  permission: PERM.DASHBOARD_VIEW,
};

// Dashboard is a synthetic section with no `labelKey` so the section
// renderer skips its header — keeps the "home" entry visually distinct
// from the themed groups below.
export const dashboardSection: NavSection = {
  id: 'dashboard',
  items: [dashboardItem],
};

/**
 * The sections that render above whatever the plugin loader contributes.
 *
 * `deliverSection` is deliberately absent. It belongs at the very bottom
 * of the sidebar — below every plugin-owned section — and returning it
 * here would put it above them. `Sidebar.tsx`
 * appends it itself as the last entry, which is the only placement a
 * newly installed plugin cannot disturb.
 */
export function buildSections(_mode: 'docker'): NavSection[] {
  return [dockerSection, operateSection, governSection, ...(ENTERPRISE_NAV_ENABLED ? [enterpriseSection] : [])];
}

import type { PluginMenuItem } from '@core/plugins/loader';
import { resolvePluginIcon } from './pluginIcons';

function pluginMenuToItem(p: PluginMenuItem): NavItem {
  return {
    to: p.path,
    // Plugin-managed translations live in `p.label_ns:p.label_key`;
    // i18next resolves dots in keys so this is a single string key.
    labelKey: `${p.label_ns}:${p.label_key}`,
    icon: resolvePluginIcon(p.icon),
    permission: p.required_permission,
  };
}

/**
 * Map plugin manifest menu entries (the `contributes.menu` block) to
 * synthetic `NavSection`s the existing sidebar renderer can drop in
 * alongside the host-managed sections.
 *
 * A plugin may own a *named* section of its own. Migration 104 declares
 * `plugin_menu.parent` as an "optional section id"; entries that share a
 * non-empty `parent` are grouped into their own section, headed by
 * `<label_ns>:nav.section.<parent>` so the plugin keeps ownership of the
 * label in its own i18n bundle. Entries with an empty `parent` keep the
 * pre-existing behaviour and collapse into the generic "Plugins"
 * section, which always sorts last.
 *
 * Returns an empty array when nothing is contributed, so the caller can
 * spread the result unconditionally.
 */
export function pluginMenuToSections(items: PluginMenuItem[]): NavSection[] {
  if (!items.length) return [];

  const owned = new Map<string, NavSection>();
  const generic: NavItem[] = [];

  for (const p of items) {
    const group = (p.parent ?? '').trim();
    if (!group) {
      generic.push(pluginMenuToItem(p));
      continue;
    }
    const id = `plugin:${group}`;
    let section = owned.get(id);
    if (!section) {
      section = {
        id,
        labelKey: `${p.label_ns}:nav.section.${group}`,
        // A plugin-owned section has no icon of its own in the manifest,
        // so the first entry it contributes stands for the group on the
        // collapsed rail.
        icon: resolvePluginIcon(p.icon),
        items: [],
      };
      owned.set(id, section);
    }
    section.items.push(pluginMenuToItem(p));
  }

  const sections = [...owned.values()];
  if (generic.length) {
    sections.push({
      id: 'plugins',
      labelKey: 'nav.section.plugins',
      icon: IconPuzzle,
      items: generic,
    });
  }
  return sections;
}
