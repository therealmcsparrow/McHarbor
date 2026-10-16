// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconBraces,
  IconCpu,
  IconDatabase,
  IconDatabaseExport,
  IconFileText,
  IconFolder,
  IconInfoCircle,
  IconListDetails,
  IconNetwork,
  IconRefresh,
  IconShield,
  IconShieldSearch,
  IconStack2,
  IconTag,
  IconTerminal,
  IconTerminal2,
  IconVariable,
  IconHistory,
  IconHistoryToggle,
  IconArrowsDiff,
  IconActivity,
  IconArchive,
  IconRocket,
  IconSettings,
  IconClock,
  IconAdjustments,
} from '@tabler/icons-react';
import { type DetailGroupId } from './detail-tab-groups';

// detailTabIDs is still the canonical list and still the unit of
// navigation: `?tab=<id>` resolves exactly as it did before this bar was
// grouped, because a group is derived from the tab rather than the other
// way round. Nothing about the URL contract changed.
//
// The order below mirrors the six groups in `detail-tab-groups.ts`:
//   1. What is it?                        → Overview
//   2. How is it configured?              → Configuration
//   3. What is it doing right now?        → Runtime
//   4. Where did it come from / going?    → Delivery
//   5. Is it safe to run?                 → Security
//   6. What is the long-term safety net?  → Backup
//
// Security was the sixth member of Configuration until it earned its own
// group. Two things pulled it out: its settings are the ones an operator
// audits rather than tunes, and the image vulnerability scan that lives
// beside them is not configuration at all — it is a finding about the
// image. Grouping them together gives the scan a home that says what it
// is, instead of burying it under the sixth tab of Configuration.
export const detailTabIDs = [
  // 1. What is it?
  'overview',
  // 2. How is it configured?
  'environment',
  'labels',
  'network',
  'mounts',
  'resources',
  // 3. What is it doing right now?
  'activity',
  'logs',
  'terminal',
  'exec',
  'files',
  'processes',
  'services',
  'inspect',
  // 4. Where did it come from / where is it going?
  'updates',
  'history',
  'changes',
  'diff',
  // 6. Is it safe to run?
  'security',
  'scans',
  // 7. What is the long-term safety net?
  'backup-settings',
  'backup-schedules',
  'backups',
] as const;

export type DetailTabId = (typeof detailTabIDs)[number];

export function isDetailTabId(value: string | null): value is DetailTabId {
  return detailTabIDs.some((tabID) => tabID === value);
}

export const detailTabIcons: Record<DetailTabId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  environment: IconVariable,
  labels: IconTag,
  network: IconNetwork,
  mounts: IconDatabase,
  resources: IconCpu,
  security: IconShield,
  scans: IconShieldSearch,
  activity: IconActivity,
  logs: IconFileText,
  terminal: IconTerminal,
  processes: IconListDetails,
  files: IconFolder,
  'backup-settings': IconAdjustments,
  'backup-schedules': IconClock,
  backups: IconDatabaseExport,
  inspect: IconBraces,
  updates: IconRefresh,
  services: IconStack2,
  exec: IconTerminal2,
  history: IconHistory,
  changes: IconHistoryToggle,
  diff: IconArrowsDiff,
};

export const groupIcons: Record<DetailGroupId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  configuration: IconSettings,
  runtime: IconActivity,
  delivery: IconRocket,
  security: IconShield,
  data: IconArchive,
};
