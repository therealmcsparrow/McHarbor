// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The user detail rail.
//
// One level, not two: six destinations fit on a single `SubNav`, so there
// is no group step to walk through before you reach anything. It renders
// inside the hero card for the same reason the container detail page does
// — the card then reads as one object: this is the account, this is its
// state, these are the faces of it.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { userDetailTabIDs, userDetailTabIcons, userDetailTabLabelKeys, type UserDetailTabId } from './user-detail-tabs';

type UserDetailTabsProps = {
  activeTab: UserDetailTabId;
  onTabChange: (tab: UserDetailTabId) => void;
  /** Accessible name for the rail. Already translated. */
  ariaLabel: string;
  /** Counts rendered beside a label. Zero or missing renders nothing. */
  tabBadges?: Partial<Record<UserDetailTabId, number>>;
  /** Destinations to hide entirely — a tab the caller cannot read is a dead end. */
  visibleTabs?: readonly UserDetailTabId[];
};

export function UserDetailTabs({
  activeTab,
  onTabChange,
  ariaLabel,
  tabBadges,
  visibleTabs = userDetailTabIDs,
}: UserDetailTabsProps) {
  const { t } = useTranslation('security');

  const labelFor = useCallback((tab: UserDetailTabId) => t(userDetailTabLabelKeys[tab]), [t]);

  const items: SubNavItem<UserDetailTabId>[] = useMemo(
    () =>
      userDetailTabIDs
        .filter((tab) => visibleTabs.includes(tab))
        .map((tab) => {
          const count = tabBadges?.[tab] ?? 0;
          return {
            id: tab,
            label: labelFor(tab),
            icon: userDetailTabIcons[tab],
            adornment:
              count > 0 ? (
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px] tabular-nums">
                  {count}
                </Badge>
              ) : undefined,
          };
        }),
    [labelFor, tabBadges, visibleTabs],
  );

  const tabContextMenuItems = useTabContextMenu({
    tabs: items.map((item) => ({
      id: item.id,
      label: item.label,
      icon: userDetailTabIcons[item.id],
    })),
    activeTab,
    onTabChange,
  });

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <SubNav
        items={items}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={ariaLabel}
        idPrefix="user-tab"
        controls="user-detail-panel"
        itemWrapper={(tab, node) => (
          <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
        )}
      />
    </div>
  );
}
