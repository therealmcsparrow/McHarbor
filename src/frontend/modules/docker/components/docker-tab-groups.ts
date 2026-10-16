// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The three sections of the Docker page.
//
// This mirrors `settings/components/settings-tab-groups.ts`, with the group
// layer left out on purpose. Settings needs two levels because thirteen
// equal-weight labels do not fit on one rail and nothing on that rail says
// which of the thirteen are related. Three labels fit, and inventing groups
// for them would add a level of navigation that answers no question.
//
// There was a fourth, `networks`. It listed the environment's Docker
// networks, which is what `/networks` already is — a second, smaller,
// read-only copy of a page one click away. Removed rather than kept in
// sync.
//
// What is kept is everything the shared chrome reads: a canonical id list in
// reading order, the id type, and the icon map — so `DockerSectionHero`,
// `DockerTabs`, the command palette and the tab context menu all take their
// tabs from one place instead of each page-level component owning a copy.

import { IconChartBar, IconDatabase, IconInfoCircle, type TablerIcon } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';

/**
 * Canonical id list, in reading order. Exported because `useTabFromUrl` and
 * the command palette both take the flat list.
 */
export const dockerTabIDs = ['info', 'registries', 'resources'] as const;

export type DockerTabId = (typeof dockerTabIDs)[number];

export const dockerTabIcons: Record<DockerTabId, TablerIcon> = {
  info: IconInfoCircle,
  registries: IconDatabase,
  resources: IconChartBar,
};

/**
 * The permission each section's own routes are gated on.
 *
 * Both keys were already enforced on the backend and gated nothing in the UI,
 * so a viewer without them opened the section and met an empty panel or a 403
 * toast: `registries` reads `/registries` (registry.view) and `resources`
 * reads `/metrics/host` (host.view). The rail now renders such a section
 * disabled with a tooltip naming the key. `info` has no entry — it is readable
 * by anyone who can reach the page at all.
 */
export const DOCKER_TAB_PERMISSIONS: Partial<Record<DockerTabId, string>> = {
  registries: PERM.REGISTRY_VIEW,
  resources: PERM.HOST_VIEW,
};

/** id of the panel the tab rail controls, shared by the rail and the panel. */
export const DOCKER_PANEL_ID = 'docker-detail-panel';

/** Prefix for each tab button's DOM id, so the panel can be labelled by it. */
export const DOCKER_TAB_ID_PREFIX = 'docker-tab';
