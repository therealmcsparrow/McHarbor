// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// BackupsTabs — the same two-level nav the Security page uses.
//
// Level 1 is the four groups; level 2 is the destinations inside the
// active group, on the `SubNav` segmented rail. Only Activity has more
// than one destination, so only Activity shows a rail.
//
// Selecting a group lands on its first destination, so a group button is
// always a destination and never a dead end; returning to Activity lands
// on whichever of its two you were last on, which is what `entryTabFor`
// carries.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { cn } from '@resources/utils/cn';
import {
  BACKUPS_GROUPS,
  backupsGroupHasSubRail as hasSubRail,
  backupsGroupIDs,
  backupsGroupIcons,
  backupsGroupLabelKey,
  backupsTabIcons,
  backupsTabLabelKeys,
  groupForBackupsTab,
  type BackupsGroupId,
  type BackupsTabId,
} from './backups-tab-groups';

type BackupsTabsProps = {
  activeTab: BackupsTabId;
  onTabChange: (tab: BackupsTabId) => void;
  /** Where a click on a group button should land. */
  entryTabFor?: (group: BackupsGroupId) => BackupsTabId;
  /** Accessible name for the group rail. Already translated. */
  ariaLabel: string;
  /** Invalidate the queries behind a destination, for "reload tab". */
  onReloadTab?: (tab: BackupsTabId) => void;
};

export function BackupsTabs({ activeTab, onTabChange, entryTabFor, ariaLabel, onReloadTab }: BackupsTabsProps) {
  const { t } = useTranslation('containers');
  const activeGroup = groupForBackupsTab(activeTab);

  const labelFor = useCallback((tab: BackupsTabId) => t(backupsTabLabelKeys[tab]), [t]);
  const groupLabel = useCallback((group: BackupsGroupId) => t(backupsGroupLabelKey(group)), [t]);
  const entryTab = useCallback(
    (group: BackupsGroupId): BackupsTabId => entryTabFor?.(group) ?? BACKUPS_GROUPS[group][0]!,
    [entryTabFor],
  );

  // Built over every destination rather than just the active group, so
  // "switch to tab" still reaches anywhere from anywhere.
  const tabContextTabs = useMemo(
    () =>
      backupsGroupIDs.flatMap((group) =>
        BACKUPS_GROUPS[group].map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: backupsTabIcons[tab],
        })),
      ),
    [labelFor],
  );
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabContextTabs,
    activeTab,
    onTabChange,
    includeLinkActions: false,
    onReloadTab,
  });

  const subTabs: SubNavItem<BackupsTabId>[] = useMemo(
    () =>
      BACKUPS_GROUPS[activeGroup].map((tab) => ({
        id: tab,
        label: labelFor(tab),
        icon: backupsTabIcons[tab],
      })),
    [activeGroup, labelFor],
  );

  const selectGroup = (group: BackupsGroupId) => {
    if (group === activeGroup) return;
    onTabChange(entryTab(group));
  };

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <div className="flex flex-col gap-2">
        {/* Level 1 — the four groups. */}
        <div role="tablist" aria-label={ariaLabel} className="flex flex-wrap items-center gap-1">
          {backupsGroupIDs.map((group) => {
            const Icon = backupsGroupIcons[group];
            const active = group === activeGroup;
            return (
              <TabContextMenu key={group} items={tabContextMenuItems(entryTab(group))}>
                <Button
                  // A single-destination group renders no rail, so this
                  // button is the only element the panel's
                  // `aria-labelledby` can point at. It borrows the rail's
                  // id scheme so the page has one rule rather than two.
                  id={hasSubRail(group) ? undefined : `backups-tab-${BACKUPS_GROUPS[group][0]}`}
                  role="tab"
                  aria-selected={active}
                  tabIndex={active ? 0 : -1}
                  variant={active ? 'secondary' : 'ghost'}
                  size="sm"
                  className={cn(
                    'gap-1.5',
                    active ? 'font-semibold text-foreground' : 'font-normal text-muted-foreground',
                  )}
                  onClick={() => selectGroup(group)}
                >
                  <Icon className="size-4" />
                  {groupLabel(group)}
                </Button>
              </TabContextMenu>
            );
          })}
        </div>

        {/* Level 2 — the two under Activity. The other three groups are
            each their own destination and render no rail. */}
        {subTabs.length > 1 && (
          <SubNav
            items={subTabs}
            value={activeTab}
            onChange={onTabChange}
            ariaLabel={groupLabel(activeGroup)}
            idPrefix="backups-tab"
            controls="backups-detail-panel"
            itemWrapper={(tab, node) => (
              <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
            )}
          />
        )}
      </div>
    </div>
  );
}
