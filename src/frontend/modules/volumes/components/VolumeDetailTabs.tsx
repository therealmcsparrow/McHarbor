// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// VolumeDetailTabs — the volume's faces, on the container page's rail.
//
// Three questions a volume answers: what is it and where does it live, what
// is mounting it, and how was it configured. One rail, three tabs — the
// container page's two-level grouping earns its keep at 21 tabs, not at 3.

import { IconAdjustments, IconBox, IconInfoCircle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';

export const volumeTabIDs = ['overview', 'containers', 'configuration'] as const;

export type VolumeTabId = (typeof volumeTabIDs)[number];

export function isVolumeTabId(value: string | null): value is VolumeTabId {
  return volumeTabIDs.some((tabID) => tabID === value);
}

const TAB_ICONS: Record<VolumeTabId, SubNavItem<VolumeTabId>['icon']> = {
  overview: IconInfoCircle,
  containers: IconBox,
  configuration: IconAdjustments,
};

type VolumeDetailTabsProps = {
  activeTab: VolumeTabId;
  onTabChange: (tab: VolumeTabId) => void;
  /** Already translated. */
  labelFor: (tab: VolumeTabId) => string;
  /** Already translated. */
  ariaLabel: string;
  /** Count shown beside a tab. Zero renders no badge. */
  tabBadges?: Partial<Record<VolumeTabId, number>>;
};

export function VolumeDetailTabs({ activeTab, onTabChange, labelFor, ariaLabel, tabBadges }: VolumeDetailTabsProps) {
  const items: SubNavItem<VolumeTabId>[] = volumeTabIDs.map((tab) => {
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
        idPrefix="volume-tab"
        controls="volume-detail-panel"
      />
    </div>
  );
}
