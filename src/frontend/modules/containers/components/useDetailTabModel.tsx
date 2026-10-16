// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Derived state for the container detail navigation: which groups still
// have a visible tab, the sub-rail entries for the active group, and the
// context-menu entries (built over every visible tab, not just the active
// group, so "switch to tab" still reaches anywhere from anywhere).

import { useCallback, useMemo } from 'react';
import { Badge } from '@resources/components/ui/Badge';
import { type SubNavItem } from '@resources/components/SubNav';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { DETAIL_GROUPS, detailGroupIDs, groupLabelKey, type DetailGroupId } from './detail-tab-groups';
import { detailTabIcons, type DetailTabId } from './detail-tab-registry';

type DetailTabModelArgs = {
  activeTab: DetailTabId;
  activeGroup: DetailGroupId;
  onTabChange: (tab: DetailTabId) => void;
  labelFor: (tab: DetailTabId) => string;
  groupLabelFor?: (key: string) => string;
  visibleTabs: DetailTabId[];
  tabBadges?: Partial<Record<DetailTabId, number>>;
};

export function useDetailTabModel({
  activeTab,
  activeGroup,
  onTabChange,
  labelFor,
  groupLabelFor,
  visibleTabs,
  tabBadges,
}: DetailTabModelArgs) {
  const visibleTabSet = useMemo(() => new Set(visibleTabs), [visibleTabs]);

  /** Groups that still have at least one visible tab on this container. */
  const availableGroups = useMemo(
    () => detailGroupIDs.filter((group) => DETAIL_GROUPS[group].some((tab) => visibleTabSet.has(tab))),
    [visibleTabSet],
  );

  const groupLabel = useCallback(
    (group: DetailGroupId) => (groupLabelFor ? groupLabelFor(groupLabelKey(group)) : group),
    [groupLabelFor],
  );

  // The context menu offers switch / reload / copy link / open in new tab.
  // It is built over *every* visible tab, not just the active group, so
  // "switch to tab" still reaches anywhere from anywhere — exactly as it
  // did when the bar was flat.
  const tabContextTabs = useMemo(
    () =>
      detailGroupIDs
        .flatMap((group) => DETAIL_GROUPS[group])
        .filter((tab) => visibleTabSet.has(tab))
        .map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: detailTabIcons[tab],
        })),
    [labelFor, visibleTabSet],
  );
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabContextTabs,
    activeTab,
    onTabChange,
  });

  const subTabs: SubNavItem<DetailTabId>[] = useMemo(
    () =>
      DETAIL_GROUPS[activeGroup]
        .filter((tab) => visibleTabSet.has(tab))
        .map((tab) => {
          const count = tabBadges?.[tab] ?? 0;
          return {
            id: tab,
            label: labelFor(tab),
            icon: detailTabIcons[tab],
            adornment:
              count > 0 ? (
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px] tabular-nums">
                  {count}
                </Badge>
              ) : undefined,
          };
        }),
    [activeGroup, labelFor, tabBadges, visibleTabSet],
  );

  /** Selecting a group lands on its first visible tab. */
  const selectGroup = useCallback(
    (group: DetailGroupId) => {
      if (group === activeGroup) return;
      const first = DETAIL_GROUPS[group].find((tab) => visibleTabSet.has(tab));
      if (first) onTabChange(first);
    },
    [activeGroup, onTabChange, visibleTabSet],
  );

  return {
    availableGroups,
    groupLabel,
    tabContextMenuItems,
    subTabs,
    selectGroup,
  };
}
