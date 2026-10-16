// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The group detail rail.
//
// One level, like the user detail rail it mirrors: three destinations fit
// on a single `SubNav`. It renders inside the hero card so the card reads
// as one object — this is the group, this is its state, these are the
// faces of it.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import {
  groupDetailTabIDs,
  groupDetailTabIcons,
  groupDetailTabLabelKeys,
  type GroupDetailTabId,
} from './group-detail-tabs';

type GroupDetailTabsProps = {
  activeTab: GroupDetailTabId;
  onTabChange: (tab: GroupDetailTabId) => void;
  /** Accessible name for the rail. Already translated. */
  ariaLabel: string;
  /** Counts rendered beside a label. Zero or missing renders nothing. */
  tabBadges?: Partial<Record<GroupDetailTabId, number>>;
};

export function GroupDetailTabs({ activeTab, onTabChange, ariaLabel, tabBadges }: GroupDetailTabsProps) {
  const { t } = useTranslation('security');

  const labelFor = useCallback((tab: GroupDetailTabId) => t(groupDetailTabLabelKeys[tab]), [t]);

  const items: SubNavItem<GroupDetailTabId>[] = useMemo(
    () =>
      groupDetailTabIDs.map((tab) => {
        const count = tabBadges?.[tab] ?? 0;
        return {
          id: tab,
          label: labelFor(tab),
          icon: groupDetailTabIcons[tab],
          adornment:
            count > 0 ? (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px] tabular-nums">
                {count}
              </Badge>
            ) : undefined,
        };
      }),
    [labelFor, tabBadges],
  );

  const tabContextMenuItems = useTabContextMenu({
    tabs: items.map((item) => ({
      id: item.id,
      label: item.label,
      icon: groupDetailTabIcons[item.id],
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
        idPrefix="group-tab"
        controls="group-detail-panel"
        itemWrapper={(tab, node) => (
          <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
        )}
      />
    </div>
  );
}
