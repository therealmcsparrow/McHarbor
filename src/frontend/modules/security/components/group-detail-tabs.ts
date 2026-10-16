// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The three faces of one group.
//
// Same model as the user detail page: the *tab id* is the unit of
// navigation, so `?tab=<id>` on `/security/groups/:id` resolves directly.
//
//   1. What is this group?        -> Overview
//   2. Who is in it?              -> Members
//   3. What does it grant?        -> Roles

import { IconInfoCircle, IconShieldCheck, IconUsers, type TablerIcon } from '@tabler/icons-react';

export const groupDetailTabIDs = ['overview', 'members', 'roles'] as const;

export type GroupDetailTabId = (typeof groupDetailTabIDs)[number];

export function isGroupDetailTabId(value: string | null): value is GroupDetailTabId {
  return groupDetailTabIDs.some((tabID) => tabID === value);
}

export const groupDetailTabIcons: Record<GroupDetailTabId, TablerIcon> = {
  overview: IconInfoCircle,
  members: IconUsers,
  roles: IconShieldCheck,
};

/** i18n keys in the `security` namespace. */
export const groupDetailTabLabelKeys: Record<GroupDetailTabId, string> = {
  overview: 'groups.detail.tabs.overview',
  members: 'groups.detail.tabs.members',
  roles: 'groups.detail.tabs.roles',
};
