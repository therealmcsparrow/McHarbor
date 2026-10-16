// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SettingsTabs — the two-level nav from the container detail page, applied
// to settings.
//
// Level 1 is the five groups; level 2 is the sections inside the active
// group, on the same `SubNav` segmented rail the container detail page
// uses. The flat `ScrollableTabBar` this replaces put thirteen equal-weight
// labels in a horizontally scrolling strip: everything past "Storage" was
// off-screen at 1280px, and nothing on the strip said which of the thirteen
// were related to each other.
//
// Selecting a group lands on its first section, so a group button is
// always a destination and never a dead end.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { IconPuzzle } from '@tabler/icons-react';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import type { PluginPageTab } from '@core/plugins/usePluginPageTabs';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { cn } from '@resources/utils/cn';
import { useTabPermissionGate } from '@resources/hooks/useTabPermissionGate';
import {
  SETTINGS_GROUPS,
  SETTINGS_TAB_PERMISSIONS,
  groupForSettingsTab,
  settingsGroupIDs,
  settingsGroupIcons,
  settingsGroupLabelKey,
  settingsTabIcons,
  type SettingsGroupId,
  type SettingsTabId,
} from './settings-tab-groups';

type SettingsTabsProps = {
  activeTab: SettingsTabId;
  onTabChange: (tab: SettingsTabId) => void;
  /** Sections this instance does not offer; left out of the rail entirely. */
  hiddenTabs?: ReadonlySet<SettingsTabId>;
  /** Tabs contributed by installed plugins for this page, already resolved. */
  pluginTabs?: PluginPageTab[];
  /** The plugin tab currently open, or null when a built-in one is. */
  activePluginTab?: string | null;
  onPluginTabChange?: (id: string) => void;
  /** Translator for a section label. Namespace-scoped by the caller. */
  labelFor: (tab: SettingsTabId) => string;
  /** Translator for a group label. Namespace-scoped by the caller. */
  groupLabelFor: (key: string) => string;
  /** Accessible name for the group rail. Already translated. */
  ariaLabel: string;
};

export function SettingsTabs({
  activeTab,
  onTabChange,
  hiddenTabs,
  labelFor,
  groupLabelFor,
  ariaLabel,
  pluginTabs = [],
  activePluginTab = null,
  onPluginTabChange,
}: SettingsTabsProps) {
  // The group follows the section, never the reverse — so
  // `?tab=logging` opens Integrations with Logging selected, with no
  // redirect and no second source of truth to keep in sync.
  const activeGroup = groupForSettingsTab(activeTab);

  // A section whose own routes the viewer cannot call renders disabled with a
  // tooltip naming the key, rather than opening onto an empty panel or a 403
  // toast. The backend check is still the boundary; this is the honesty.
  const { t } = useTranslation('common');
  const gateFor = useTabPermissionGate<SettingsTabId>(SETTINGS_TAB_PERMISSIONS);

  const sectionsOf = useCallback(
    (group: SettingsGroupId) => SETTINGS_GROUPS[group].filter((tab) => !hiddenTabs?.has(tab)),
    [hiddenTabs],
  );

  const groupLabel = useCallback(
    (group: SettingsGroupId) => groupLabelFor(settingsGroupLabelKey(group)),
    [groupLabelFor],
  );

  // The context menu offers switch / reload / copy link / open in new tab,
  // and is built over every section rather than just the active group, so
  // "switch to tab" still reaches anywhere from anywhere.
  const tabContextTabs = useMemo(
    () =>
      settingsGroupIDs.flatMap((group) =>
        sectionsOf(group).map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: settingsTabIcons[tab],
        })),
      ),
    [labelFor, sectionsOf],
  );
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabContextTabs,
    activeTab,
    onTabChange,
  });

  const subTabs: SubNavItem<SettingsTabId>[] = useMemo(
    () =>
      sectionsOf(activeGroup).map((tab) => {
        const reason = gateFor(tab);
        return {
          id: tab,
          label: labelFor(tab),
          icon: settingsTabIcons[tab],
          disabled: Boolean(reason),
          disabledReason: reason,
        };
      }),
    [activeGroup, labelFor, gateFor, sectionsOf],
  );

  /**
   * Selecting a group lands on its first section the viewer can actually
   * open — landing them on a disabled one would make the group a dead end.
   */
  const selectGroup = (group: SettingsGroupId) => {
    if (group === activeGroup) return;
    const sections = sectionsOf(group);
    const first = sections.find((tab) => !gateFor(tab)) ?? sections[0];
    if (first) onTabChange(first);
  };

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <div className="flex flex-col gap-2">
        {/* Level 1 — the five groups. All of them fit at 1280px, which is
            the whole point: thirteen sections never did. */}
        <div role="tablist" aria-label={ariaLabel} className="flex flex-wrap items-center gap-1">
          {settingsGroupIDs.map((group) => {
            const Icon = settingsGroupIcons[group];
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
                {groupLabel(group)}
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
              {t('plugins.addons', { ns: 'common' })}
            </Button>
          )}
        </div>

        {activePluginTab !== null && pluginTabs.length > 0 && (
          <SubNav
            items={pluginTabs}
            value={activePluginTab}
            onChange={(id) => onPluginTabChange?.(id)}
            ariaLabel={t('plugins.addons', { ns: 'common' })}
            idPrefix="settings-tab"
            controls="settings-detail-panel"
          />
        )}

        {/* Level 2 — the destinations inside the active group. */}
        {activePluginTab === null && subTabs.length > 1 && (
          <SubNav
            items={subTabs}
            value={activeTab}
            onChange={onTabChange}
            ariaLabel={groupLabel(activeGroup)}
            idPrefix="settings-tab"
            controls="settings-detail-panel"
            itemWrapper={(tab, node) => (
              <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
            )}
          />
        )}
      </div>
    </div>
  );
}
