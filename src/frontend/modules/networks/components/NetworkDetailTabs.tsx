// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// NetworkDetailTabs — the network's faces, on the container page's rail.
//
// Same `SubNav` sliding pill the container detail page uses, sitting inside
// the hero card so the card reads as one object: this is the network, this
// is its state, these are the faces of it.
//
// The container page needs a second level because it carries 21 tabs across
// five groups. A network has three faces, so it stops at one rail.

import { IconAdjustments, IconBox, IconInfoCircle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';

export const networkTabIDs = ['overview', 'containers', 'configuration'] as const;

export type NetworkTabId = (typeof networkTabIDs)[number];

export function isNetworkTabId(value: string | null): value is NetworkTabId {
  return networkTabIDs.some((tabID) => tabID === value);
}

const TAB_ICONS: Record<NetworkTabId, SubNavItem<NetworkTabId>['icon']> = {
  overview: IconInfoCircle,
  containers: IconBox,
  configuration: IconAdjustments,
};

type NetworkDetailTabsProps = {
  activeTab: NetworkTabId;
  onTabChange: (tab: NetworkTabId) => void;
  /** Already translated. */
  labelFor: (tab: NetworkTabId) => string;
  /** Already translated. */
  ariaLabel: string;
  /** Count shown beside a tab. Zero renders no badge. */
  tabBadges?: Partial<Record<NetworkTabId, number>>;
};

export function NetworkDetailTabs({ activeTab, onTabChange, labelFor, ariaLabel, tabBadges }: NetworkDetailTabsProps) {
  const items: SubNavItem<NetworkTabId>[] = networkTabIDs.map((tab) => {
    const count = tabBadges?.[tab] ?? 0;
    return {
      id: tab,
      label: labelFor(tab),
      icon: TAB_ICONS[tab],
      adornment:
        count > 0 ? (
          <Badge variant="secondary" className="ml-0.5 px-1.5 py-0 text-[10px]">
            {count}
          </Badge>
        ) : undefined,
    };
  });

  return (
    <div className="border-t border-border bg-transparent px-3 py-2">
      <SubNav
        items={items}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={ariaLabel}
        idPrefix="network-tab"
        controls="network-detail-panel"
      />
    </div>
  );
}
