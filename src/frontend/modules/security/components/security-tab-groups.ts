// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The five groups the ten security destinations belong to.
//
// Unlike settings, this page already had the model — five sections, one of
// which (Management) carries six sub-tabs. What it did not have was one
// place to state it: the section/sub-tab branching was hand-written five
// times over in `SecurityPage`, and the two levels were drawn as two
// stacked pill strips rather than the hero-and-rail the container detail
// page uses.
//
// The questions the five groups answer:
//
//   1. How is the app served?        -> HTTPS
//   2. Who can get in, and as what?  -> Management
//   3. Where do identities come from?-> Identity
//   4. What is scanned?              -> Scanning
//   5. How strong must secrets be?   -> Password
//   6. What credentials does it hold?-> Secrets
//
// The *tab id* stays the unit of navigation, so every `?tab=<id>` keeps
// resolving — `api-keys` included. A group is derived from the tab, never
// the reverse.

import {
  IconAdjustmentsHorizontal,
  IconAlarm,
  IconBug,
  IconChecklist,
  IconFlask,
  IconGavel,
  IconScale,
  IconFingerprint,
  IconKey,
  IconKeyFilled,
  IconLock,
  IconShieldCheck,
  IconShieldLock,
  IconShieldPlus,
  IconUserCheck,
  IconUsers,
  IconUsersGroup,
  IconUserShield,
  type TablerIcon,
} from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';

/** Canonical id list, in group reading order. */
export const securityTabIDs = [
  // 1. How is the app served?
  'https',
  // 2. Who can get in, and as what?
  'users',
  'groups',
  'roles',
  'policies',
  'scim',
  'api-keys',
  'elevations',
  'access',
  'approvals',
  'approval-rules',
  'simulator',
  'lifecycle',
  'break-glass',
  // 3. Where do identities come from?
  'identity',
  // 4. What is scanned?
  'scanning',
  // 5. How strong must secrets be?
  'password',
  // 6. What credentials does the application itself hold?
  'secrets',
] as const;

export type SecurityTabId = (typeof securityTabIDs)[number];

/**
 * The permission each section's own routes are gated on. Same contract as
 * SETTINGS_TAB_PERMISSIONS: a section the viewer cannot call renders disabled
 * with a tooltip naming the key, rather than opening onto an empty list.
 *
 * `https`, `scanning` and `password` have no dedicated key today; they are
 * covered by settings.manage on the backend.
 */
export const SECURITY_TAB_PERMISSIONS: Partial<Record<SecurityTabId, string>> = {
  users: PERM.USERS_VIEW,
  groups: PERM.GROUPS_VIEW,
  roles: PERM.ROLES_VIEW,
  policies: PERM.RBAC_POLICIES_VIEW,
  scim: PERM.SCIM_VIEW,
  access: PERM.PERMISSIONS_VIEW,
  'approval-rules': PERM.APPROVALS_VIEW,
  simulator: PERM.PERMISSIONS_VIEW,
  lifecycle: PERM.USERS_VIEW,
  'api-keys': PERM.API_KEYS_VIEW,
  identity: PERM.IDENTITY_PROVIDERS_VIEW,
  scanning: PERM.SCANS_VIEW,
  secrets: PERM.SECRETS_VIEW,
};

export const securityGroupIDs = [
  'https',
  'management',
  'governance',
  'identity',
  'scanning',
  'password',
  'secrets',
] as const;

export type SecurityGroupId = (typeof securityGroupIDs)[number];

/**
 * Group -> its destinations, in reading order. The union is exactly
 * `securityTabIDs`.
 */
export const SECURITY_GROUPS: Record<SecurityGroupId, readonly SecurityTabId[]> = {
  https: ['https'],
  management: ['users', 'groups', 'roles', 'policies', 'scim', 'api-keys', 'elevations', 'access'],
  governance: ['approvals', 'approval-rules', 'simulator', 'lifecycle', 'break-glass'],
  identity: ['identity'],
  scanning: ['scanning'],
  password: ['password'],
  secrets: ['secrets'],
};

const TAB_TO_GROUP: Record<string, SecurityGroupId> = Object.fromEntries(
  securityGroupIDs.flatMap((group) => SECURITY_GROUPS[group].map((tab) => [tab, group] as const)),
);

/** Which group does this destination live in? Total — every tab has one. */
export function groupForSecurityTab(tab: SecurityTabId): SecurityGroupId {
  return TAB_TO_GROUP[tab] ?? 'https';
}

/** i18n key for a group's label, in the `security` namespace. */
export function securityGroupLabelKey(group: SecurityGroupId): string {
  return `sections.${group}`;
}

/**
 * A group with a single destination renders no sub-rail — the group button
 * *is* the destination. Four of the five are single; only Management has a
 * rail, which is the whole reason the two levels exist.
 */
export function securityGroupHasSubRail(group: SecurityGroupId): boolean {
  return SECURITY_GROUPS[group].length > 1;
}

/**
 * i18n key for a destination's label. The four single-destination groups
 * are named by their section key; Management's six have their own, and
 * `api-keys` is `tabs.apiKeys` rather than `tabs.api-keys`.
 */
export const securityTabLabelKeys: Record<SecurityTabId, string> = {
  https: 'sections.https',
  users: 'tabs.users',
  groups: 'tabs.groups',
  roles: 'tabs.roles',
  policies: 'tabs.policies',
  scim: 'tabs.scim',
  'api-keys': 'tabs.apiKeys',
  elevations: 'tabs.elevations',
  access: 'tabs.access',
  approvals: 'governance.tabs.approvals',
  'approval-rules': 'governance.tabs.approvalRules',
  simulator: 'governance.tabs.simulator',
  lifecycle: 'governance.tabs.lifecycle',
  'break-glass': 'governance.tabs.breakGlass',
  identity: 'sections.identity',
  scanning: 'sections.scanning',
  password: 'sections.password',
  secrets: 'sections.secrets',
};

/**
 * i18n key for the one-line description under the hero title.
 *
 * Eight of these already existed as the section's own `description` and are
 * reused rather than duplicated — one string, one place to translate it.
 * Only SCIM (whose copy lives in the `scim` namespace, which this page does
 * not load) and Password (which had none) needed a new key.
 */
export const securityTabDescriptionKeys: Record<SecurityTabId, string> = {
  https: 'https.description',
  users: 'users.description',
  groups: 'groups.description',
  roles: 'roles.description',
  policies: 'abac.description',
  scim: 'sectionDescription.scim',
  'api-keys': 'apiKeys.description',
  elevations: 'elevations.requestDescription',
  access: 'sectionDescription.access',
  approvals: 'governance.approvals.description',
  'approval-rules': 'governance.rules.description',
  simulator: 'governance.simulator.description',
  lifecycle: 'governance.lifecycle.description',
  'break-glass': 'governance.breakGlass.selfDescription',
  identity: 'identity.description',
  scanning: 'scanning.description',
  password: 'sectionDescription.password',
  secrets: 'sectionDescription.secrets',
};

export const securityTabIcons: Record<SecurityTabId, TablerIcon> = {
  https: IconLock,
  users: IconUsers,
  groups: IconUsersGroup,
  roles: IconShieldCheck,
  policies: IconAdjustmentsHorizontal,
  scim: IconUsersGroup,
  'api-keys': IconKey,
  elevations: IconShieldPlus,
  access: IconUserShield,
  approvals: IconChecklist,
  'approval-rules': IconGavel,
  simulator: IconFlask,
  lifecycle: IconUserCheck,
  'break-glass': IconAlarm,
  identity: IconFingerprint,
  scanning: IconBug,
  password: IconShieldLock,
  secrets: IconKeyFilled,
};

export const securityGroupIcons: Record<SecurityGroupId, TablerIcon> = {
  https: IconLock,
  management: IconUsersGroup,
  governance: IconScale,
  identity: IconFingerprint,
  scanning: IconBug,
  password: IconShieldLock,
  secrets: IconKeyFilled,
};

if (import.meta.env.DEV) {
  const covered = securityGroupIDs.flatMap((group) => SECURITY_GROUPS[group]);
  const missing = securityTabIDs.filter((tab) => !covered.includes(tab));
  if (missing.length > 0) {
    console.warn(`[security-tab-groups] destinations outside every group: ${missing.join(', ')}`);
  }
}
