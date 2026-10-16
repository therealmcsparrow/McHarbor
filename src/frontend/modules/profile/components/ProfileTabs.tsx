// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ProfileTabs — the profile page's face selector.
//
// Same rail the container detail page uses (`SubNav`), sitting inside the
// hero card so the card reads as one object: this is you, this is your
// account state, these are the faces of it.
//
// The container page needs two levels because it carries 21 tabs across
// five groups. Profile carries five, so it stops at one rail — grouping
// five items would be ceremony without a payoff.

import { IconBell, IconPalette, IconShieldLock, IconUserCircle, IconUserEdit } from '@tabler/icons-react';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';

export const profileTabIDs = ['overview', 'account', 'appearance', 'notifications', 'security'] as const;

export type ProfileTabId = (typeof profileTabIDs)[number];

export function isProfileTabId(value: string | null): value is ProfileTabId {
  return profileTabIDs.some((tabID) => tabID === value);
}

const TAB_ICONS: Record<ProfileTabId, SubNavItem<ProfileTabId>['icon']> = {
  overview: IconUserCircle,
  account: IconUserEdit,
  appearance: IconPalette,
  notifications: IconBell,
  security: IconShieldLock,
};

type ProfileTabsProps = {
  activeTab: ProfileTabId;
  onTabChange: (tab: ProfileTabId) => void;
  /** Already translated. */
  labelFor: (tab: ProfileTabId) => string;
  /** Already translated. */
  ariaLabel: string;
};

export function ProfileTabs({ activeTab, onTabChange, labelFor, ariaLabel }: ProfileTabsProps) {
  const items: SubNavItem<ProfileTabId>[] = profileTabIDs.map((tab) => ({
    id: tab,
    label: labelFor(tab),
    icon: TAB_ICONS[tab],
  }));

  return (
    <div className="border-t border-border bg-transparent px-3 py-2">
      <SubNav
        items={items}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={ariaLabel}
        idPrefix="profile-tab"
        controls="profile-detail-panel"
      />
    </div>
  );
}
