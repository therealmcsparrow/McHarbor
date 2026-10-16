// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The six faces of one user account.
//
// Same model as the container detail page, one level shallower: six
// destinations fit on a single rail, so there is no group step to walk
// through. The *tab id* is the unit of navigation, so `?tab=<id>` on
// `/security/users/:id` resolves directly.
//
//   1. Who is this?               -> Overview
//   2. What groups is it in?      -> Groups
//   3. What roles does it hold?   -> Roles
//   4. What was granted by hand?  -> Permissions
//   5. Where is it signed in?     -> Sessions
//   6. What can an admin do?      -> Account

import {
  IconInfoCircle,
  IconUsersGroup,
  IconShieldCheck,
  IconShieldLock,
  IconDeviceLaptop,
  IconUserCog,
  type TablerIcon,
} from '@tabler/icons-react';

export const userDetailTabIDs = ['overview', 'groups', 'roles', 'permissions', 'sessions', 'account'] as const;

export type UserDetailTabId = (typeof userDetailTabIDs)[number];

export function isUserDetailTabId(value: string | null): value is UserDetailTabId {
  return userDetailTabIDs.some((tabID) => tabID === value);
}

export const userDetailTabIcons: Record<UserDetailTabId, TablerIcon> = {
  overview: IconInfoCircle,
  groups: IconUsersGroup,
  roles: IconShieldCheck,
  permissions: IconShieldLock,
  sessions: IconDeviceLaptop,
  account: IconUserCog,
};

/** i18n keys in the `security` namespace. */
export const userDetailTabLabelKeys: Record<UserDetailTabId, string> = {
  overview: 'users.detail.tabs.overview',
  groups: 'users.detail.tabs.groups',
  roles: 'users.detail.tabs.roles',
  permissions: 'users.detail.tabs.permissions',
  sessions: 'users.detail.tabs.sessions',
  account: 'users.detail.tabs.account',
};
