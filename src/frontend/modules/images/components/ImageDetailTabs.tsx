// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ImageDetailTabs — the image's faces, on the container page's rail.
//
// The image page had no tabs at all: four cards in one grid, with the layer
// history — often forty rows — sharing a screen with the eight-line summary
// you actually came for. Four faces, one rail.

import { IconHistory, IconInfoCircle, IconSettings, IconTag } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';

export const imageTabIDs = ['overview', 'config', 'labels', 'history'] as const;

export type ImageTabId = (typeof imageTabIDs)[number];

export function isImageTabId(value: string | null): value is ImageTabId {
  return imageTabIDs.some((tabID) => tabID === value);
}

const TAB_ICONS: Record<ImageTabId, SubNavItem<ImageTabId>['icon']> = {
  overview: IconInfoCircle,
  config: IconSettings,
  labels: IconTag,
  history: IconHistory,
};

type ImageDetailTabsProps = {
  activeTab: ImageTabId;
  onTabChange: (tab: ImageTabId) => void;
  /** Already translated. */
  labelFor: (tab: ImageTabId) => string;
  /** Already translated. */
  ariaLabel: string;
  /** Count shown beside a tab. Zero renders no badge. */
  tabBadges?: Partial<Record<ImageTabId, number>>;
};

export function ImageDetailTabs({ activeTab, onTabChange, labelFor, ariaLabel, tabBadges }: ImageDetailTabsProps) {
  const items: SubNavItem<ImageTabId>[] = imageTabIDs.map((tab) => {
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
        idPrefix="image-tab"
        controls="image-detail-panel"
      />
    </div>
  );
}
