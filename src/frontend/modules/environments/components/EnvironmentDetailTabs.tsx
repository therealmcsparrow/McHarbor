// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// EnvironmentDetailTabs — the two-level tab bar inside the environment hero.
//
// Same anatomy as `ContainerDetailTabs`: a row of group buttons, and under
// it a `SubNav` rail with the destinations of the active group. Drag-to-
// reorder and pinned quick-jumps are left out — nine tabs do not need them.

import {
  IconActivity,
  IconAdjustments,
  IconBrandDocker,
  IconClock,
  IconFileText,
  IconKey,
  IconPackage,
  IconInfoCircle,
  IconListDetails,
  IconRobot,
  IconPuzzle,
  IconServer,
  IconServerCog,
  IconTerminal,
} from '@tabler/icons-react';
import { useMemo, type ReactElement } from 'react';
import { Button } from '@resources/components/ui/Button';
import { useTranslation } from 'react-i18next';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import type { PluginPageTab } from '@core/plugins/usePluginPageTabs';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { cn } from '@resources/utils/cn';
import { environmentDetailTabs, type EnvironmentDetailTab } from '../hooks/useEnvironmentDetailState';
import {
  ENVIRONMENT_DETAIL_GROUPS,
  environmentDetailGroupIDs,
  groupForTab,
  groupHasSubRail,
  groupLabelKey,
  type EnvironmentDetailGroupId,
} from './environment-detail-tab-groups';

export const environmentTabIcons: Record<EnvironmentDetailTab, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  activity: IconActivity,
  automation: IconRobot,
  retention: IconClock,
  access: IconKey,
  host: IconServer,
  docker: IconBrandDocker,
  processes: IconListDetails,
  terminal: IconTerminal,
  logs: IconFileText,
  updates: IconPackage,
  services: IconServerCog,
};

const groupIcons: Record<EnvironmentDetailGroupId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  settings: IconAdjustments,
  host: IconServer,
};

type EnvironmentDetailTabsProps = {
  activeTab: EnvironmentDetailTab;
  onTabChange: (tab: EnvironmentDetailTab) => void;
  /** Tabs contributed by installed plugins for this page, already resolved. */
  pluginTabs?: PluginPageTab[];
  /** The plugin tab currently open, or null when a built-in tab is. */
  activePluginTab?: string | null;
  onPluginTabChange?: (id: string) => void;
  /** Already namespace-scoped by the caller. */
  labelFor: (tab: EnvironmentDetailTab) => string;
  groupLabelFor: (key: string) => string;
  ariaLabel: string;
  hiddenTabs?: readonly EnvironmentDetailTab[];
};

export function EnvironmentDetailTabs({
  activeTab,
  onTabChange,
  pluginTabs = [],
  activePluginTab = null,
  onPluginTabChange,
  labelFor,
  groupLabelFor,
  ariaLabel,
  hiddenTabs = [],
}: EnvironmentDetailTabsProps) {
  const activeGroup = groupForTab(activeTab);
  const { t } = useTranslation('common');

  // Built over every tab, not just the active group, so "switch to tab"
  // in the context menu still reaches anywhere from anywhere.
  const contextTabs = useMemo(
    () =>
      environmentDetailTabs
        .filter((tab) => !hiddenTabs.includes(tab))
        .map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: environmentTabIcons[tab],
        })),
    [labelFor, hiddenTabs],
  );
  const tabContextMenuItems = useTabContextMenu({
    tabs: contextTabs,
    activeTab,
    onTabChange,
  });

  const subTabs: SubNavItem<EnvironmentDetailTab>[] = useMemo(
    () =>
      ENVIRONMENT_DETAIL_GROUPS[activeGroup]
        .filter((tab) => !hiddenTabs.includes(tab))
        .map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: environmentTabIcons[tab],
        })),
    [activeGroup, labelFor, hiddenTabs],
  );

  const selectGroup = (group: EnvironmentDetailGroupId) => {
    if (group === activeGroup) return;
    onTabChange(ENVIRONMENT_DETAIL_GROUPS[group][0]);
  };

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <div className="flex flex-col gap-2">
        <div role="tablist" aria-label={ariaLabel} className="flex flex-wrap items-center gap-1">
          {environmentDetailGroupIDs.map((group) => {
            const Icon = groupIcons[group];
            const active = group === activeGroup && activePluginTab === null;
            return (
              <Button
                key={group}
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
                {groupLabelFor(groupLabelKey(group))}
              </Button>
            );
          })}

          {pluginTabs.length > 0 && (
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
          )}
        </div>

        {activePluginTab !== null && pluginTabs.length > 0 && (
          <SubNav
            items={pluginTabs}
            value={activePluginTab}
            onChange={(id) => onPluginTabChange?.(id)}
            ariaLabel={t('plugins.addons')}
            idPrefix="environment-tab"
            controls="environment-detail-panel"
          />
        )}

        {activePluginTab === null && groupHasSubRail(activeGroup) && (
          <SubNav
            items={subTabs}
            value={activeTab}
            onChange={onTabChange}
            ariaLabel={groupLabelFor(groupLabelKey(activeGroup))}
            idPrefix="environment-tab"
            controls="environment-detail-panel"
            itemWrapper={(tab, node) => (
              <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
            )}
          />
        )}
      </div>
    </div>
  );
}
