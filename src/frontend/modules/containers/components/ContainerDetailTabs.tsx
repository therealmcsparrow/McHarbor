// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPuzzle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { SubNav } from '@resources/components/SubNav';
import type { PluginPageTab } from '@core/plugins/usePluginPageTabs';
import { cn } from '@resources/utils/cn';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { DetailGroupRail } from './DetailGroupRail';
import { groupForTab, groupHasSubRail } from './detail-tab-groups';
import { detailTabIDs, type DetailTabId } from './detail-tab-registry';
import { useDetailTabModel } from './useDetailTabModel';

// The tab registry (ids, icons, persisted order) lives in
// detail-tab-registry.ts; re-exported so existing importers keep working.
export { detailTabIDs, detailTabIcons, isDetailTabId } from './detail-tab-registry';
export type { DetailTabId } from './detail-tab-registry';

type ContainerDetailTabsProps = {
  activeTab: DetailTabId;
  onTabChange: (tab: DetailTabId) => void;
  /** Tabs contributed by installed plugins for this page, already resolved. */
  pluginTabs?: PluginPageTab[];
  /** The plugin tab currently open, or null when a built-in one is. */
  activePluginTab?: string | null;
  onPluginTabChange?: (id: string) => void;
  labelFor: (tab: DetailTabId) => string;
  /** Translator for the group labels. Namespace-scoped by the caller. */
  groupLabelFor?: (key: string) => string;
  ariaLabel: string;
  visibleTabs?: DetailTabId[];
  /**
   * Per-tab counts rendered as a badge next to the label in the sub-rail.
   * A zero or missing count renders nothing — an empty badge is noise, and
   * "0 mounts" is already obvious from the empty state inside the tab.
   */
  tabBadges?: Partial<Record<DetailTabId, number>>;
};

export function ContainerDetailTabs({
  activeTab,
  onTabChange,
  labelFor,
  groupLabelFor,
  ariaLabel,
  visibleTabs = [...detailTabIDs],
  tabBadges,
  pluginTabs = [],
  activePluginTab = null,
  onPluginTabChange,
}: ContainerDetailTabsProps) {
  const { t } = useTranslation('common');

  // The group follows the tab, never the reverse — so a deep link to
  // `?tab=diff` opens Delivery with Diff selected, with no redirect and no
  // second source of truth to keep in sync.
  const activeGroup = groupForTab(activeTab);

  const { availableGroups, groupLabel, tabContextMenuItems, subTabs, selectGroup } = useDetailTabModel({
    activeTab,
    activeGroup,
    onTabChange,
    labelFor,
    groupLabelFor,
    visibleTabs,
    tabBadges,
  });

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <div className="flex flex-col gap-2">
        {/* Level 1 — the five groups. All of them fit at 1280px, which is
            the whole point: 21 tabs never did. */}
        <DetailGroupRail
          groups={availableGroups}
          activeGroup={activeGroup}
          onSelectGroup={selectGroup}
          groupLabel={groupLabel}
          ariaLabel={ariaLabel}
        />

        {pluginTabs.length > 0 && (
          <div className="flex flex-wrap items-center gap-1">
            <Button
              role="tab"
              aria-selected={activePluginTab !== null}
              tabIndex={activePluginTab !== null ? 0 : -1}
              variant={activePluginTab !== null ? 'secondary' : 'ghost'}
              size="sm"
              className={cn(
                'gap-1.5',
                activePluginTab !== null ? 'font-semibold text-foreground' : 'font-normal text-muted-foreground',
              )}
              onClick={() => onPluginTabChange?.(pluginTabs[0]!.id)}
            >
              <IconPuzzle className="size-4" />
              {t('plugins.addons')}
            </Button>
          </div>
        )}

        {activePluginTab !== null && pluginTabs.length > 0 && (
          <SubNav
            items={pluginTabs}
            value={activePluginTab}
            onChange={(id) => onPluginTabChange?.(id)}
            ariaLabel={t('plugins.addons')}
            idPrefix="container-tab"
            controls="container-detail-panel"
          />
        )}

        {/* Level 2 — the destinations inside the active group. Groups with
            one member (Overview, Data) render no rail; the group button is
            already the destination. */}
        {activePluginTab === null && groupHasSubRail(activeGroup) && subTabs.length > 1 && (
          <SubNav
            items={subTabs}
            value={activeTab}
            onChange={onTabChange}
            ariaLabel={groupLabel(activeGroup)}
            idPrefix="container-tab"
            controls="container-detail-panel"
            itemWrapper={(tab, node) => (
              <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
            )}
          />
        )}
      </div>
    </div>
  );
}
