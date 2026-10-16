// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconLayoutDashboard,
  IconBox,
  IconPhoto,
  IconDeviceFloppy,
  IconNetwork,
  IconStack2,
  IconTerminal,
  IconArchive,
  IconFileText,
  IconWorld,
  IconGitBranch,
  IconGitMerge,
  IconRocket,
  IconActivity,
  IconClipboardList,
  IconSettings,
  IconRefresh,
  IconInfoCircle,
  IconCommand,
  IconBell,
  IconShield,
  IconBrandDocker,
} from '@tabler/icons-react';
import type { ContainerInfo, ContainerState, ImageInfo } from '@core/types/docker';
import { ENTERPRISE_NAV_ENABLED, enterpriseNavItem, type NavItem } from '@resources/layout/sidebar-data';

// ── Navigation commands ─────────────────────────────────────────────

export const NAV_COMMANDS: NavItem[] = [
  { labelKey: 'nav.dashboard', to: '/dashboard', icon: IconLayoutDashboard },
  { labelKey: 'nav.containers', to: '/containers', icon: IconBox },
  { labelKey: 'nav.images', to: '/images', icon: IconPhoto },
  { labelKey: 'nav.volumes', to: '/volumes', icon: IconDeviceFloppy },
  { labelKey: 'nav.networks', to: '/networks', icon: IconNetwork },
  { labelKey: 'nav.stacks', to: '/stacks', icon: IconStack2 },
  { labelKey: 'nav.terminal', to: '/terminal', icon: IconTerminal },
  { labelKey: 'nav.logs', to: '/logs', icon: IconFileText },
  { labelKey: 'nav.logging', to: '/lifecycle', icon: IconFileText },
  { labelKey: 'nav.activity', to: '/activity', icon: IconActivity },
  { labelKey: 'nav.backups', to: '/backups', icon: IconArchive },
  { labelKey: 'nav.reconciler', to: '/reconciler', icon: IconRefresh },
  { labelKey: 'nav.workflows', to: '/workflows', icon: IconGitMerge },
  { labelKey: 'nav.environments', to: '/environments', icon: IconWorld },
  { labelKey: 'nav.git', to: '/git', icon: IconGitBranch },
  { labelKey: 'nav.gitops', to: '/gitops', icon: IconRocket },
  { labelKey: 'nav.store', to: '/store', icon: IconCommand },
  { labelKey: 'nav.audit', to: '/audit', icon: IconClipboardList },
  {
    labelKey: 'nav.securityScans',
    to: '/security/scans',
    icon: IconShield,
    permission: 'scans.view',
  },
  // Withheld with the sidebar item it mirrors — a palette that still
  // offers a destination the nav has withdrawn is the same leak, just
  // one keystroke further away. See ENTERPRISE_NAV_ENABLED.
  ...(ENTERPRISE_NAV_ENABLED ? [enterpriseNavItem] : []),
  // The cluster is a sidebar section now, so each of its groups is its own
  // destination here too — searching "storage" should reach the cluster one.
  { labelKey: 'nav.docker', to: '/docker', icon: IconBrandDocker },
  { labelKey: 'nav.notifications', to: '/notifications', icon: IconBell },
  { labelKey: 'nav.security', to: '/security', icon: IconShield },
  { labelKey: 'nav.settings', to: '/settings', icon: IconSettings },
  {
    labelKey: 'about.menuItem',
    to: '/settings?tab=about',
    icon: IconInfoCircle,
  },
];

// ── State dot colors ────────────────────────────────────────────────

export const CONTAINER_DOT: Record<ContainerState, string> = {
  running: 'bg-success',
  paused: 'bg-warning',
  restarting: 'bg-warning',
  created: 'bg-muted',
  exited: 'bg-danger',
  removing: 'bg-danger',
  dead: 'bg-danger',
};

export const DEFAULT_DOT = 'bg-muted';

export const STACK_DOT: Record<string, string> = {
  running: 'bg-success',
  partial: 'bg-warning',
  stopped: 'bg-danger',
};

// ── Types ───────────────────────────────────────────────────────────

export type StackSvc = {
  name: string;
  containerId?: string;
  status: string;
  image: string;
};
export type StackInfo = {
  id: string;
  name: string;
  status: string;
  services: StackSvc[];
  description?: string;
  type: 'managed' | 'discovered';
};

// ── Helpers ─────────────────────────────────────────────────────────

export function containerName(c: ContainerInfo): string {
  const itemName = c.Names?.[0] ?? c.Id.slice(0, 12);
  return itemName.startsWith('/') ? itemName.slice(1) : itemName;
}

export function imageLabel(img: ImageInfo): string {
  const firstTag = img.RepoTags?.[0];
  if (firstTag && firstTag !== '<none>:<none>') return firstTag;
  return img.Id.replace('sha256:', '').slice(0, 12);
}
