// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SecurityTabs — the two-level nav from the container detail page.
//
// Level 1 is the five sections; level 2 is the destinations inside the
// active section, on the same `SubNav` segmented rail the container detail
// page uses. Only Management has more than one destination, so only
// Management shows a rail — which is precisely the container page's rule
// for Overview and Data, and the reason the second strip no longer has to
// be conditionally assembled by the page itself.
//
// Selecting a section lands on its first destination, so a section button
// is always a destination and never a dead end. Returning to Management
// lands on whichever of its six you were last on, which is what the
// caller's `entryTabFor` carries.

import { type ReactElement, useCallback, useMemo } from 'react';
import { IconPuzzle } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import type { PluginPageTab } from '@core/plugins/usePluginPageTabs';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { cn } from '@resources/utils/cn';
import { useTabPermissionGate } from '@resources/hooks/useTabPermissionGate';
import {
  SECURITY_GROUPS,
  SECURITY_TAB_PERMISSIONS,
  groupForSecurityTab,
  securityGroupIDs,
  securityGroupIcons,
  securityGroupHasSubRail as hasSubRail,
  securityGroupLabelKey,
  securityTabIcons,
  securityTabLabelKeys,
  type SecurityGroupId,
  type SecurityTabId,
} from './security-tab-groups';

type SecurityTabsProps = {
  activeTab: SecurityTabId;
  onTabChange: (tab: SecurityTabId) => void;
  /** Tabs contributed by installed plugins for this page, already resolved. */
  pluginTabs?: PluginPageTab[];
  /** The plugin tab currently open, or null when a built-in one is. */
  activePluginTab?: string | null;
  onPluginTabChange?: (id: string) => void;
  /**
   * Where a click on a section button should land. Lets the page remember
   * the last Management sub-tab instead of always resetting to Users.
   */
  entryTabFor?: (group: SecurityGroupId) => SecurityTabId;
  /** Accessible name for the section rail. Already translated. */
  ariaLabel: string;
};

export function SecurityTabs({
  activeTab,
  onTabChange,
  entryTabFor,
  ariaLabel,
  pluginTabs = [],
  activePluginTab = null,
  onPluginTabChange,
}: SecurityTabsProps) {
  const { t } = useTranslation('security');
  const activeGroup = groupForSecurityTab(activeTab);

  const labelFor = useCallback((tab: SecurityTabId) => t(securityTabLabelKeys[tab]), [t]);
  const groupLabel = useCallback((group: SecurityGroupId) => t(securityGroupLabelKey(group)), [t]);
  // A section whose own routes the viewer cannot call renders disabled with a
  // tooltip naming the key. The backend check is still the boundary.
  const gateFor = useTabPermissionGate<SecurityTabId>(SECURITY_TAB_PERMISSIONS);

  const entryTab = useCallback(
    (group: SecurityGroupId): SecurityTabId => {
      const preferred = entryTabFor?.(group);
      if (preferred && !gateFor(preferred)) return preferred;
      // Land on a section the viewer can open; a disabled one is a dead end.
      return SECURITY_GROUPS[group].find((tab) => !gateFor(tab)) ?? SECURITY_GROUPS[group][0]!;
    },
    [entryTabFor, gateFor],
  );

  // Built over every destination rather than just the active section, so
  // "switch to tab" still reaches anywhere from anywhere.
  const tabContextTabs = useMemo(
    () =>
      securityGroupIDs.flatMap((group) =>
        SECURITY_GROUPS[group].map((tab) => ({
          id: tab,
          label: labelFor(tab),
          icon: securityTabIcons[tab],
        })),
      ),
    [labelFor],
  );
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabContextTabs,
    activeTab,
    onTabChange,
  });

  const subTabs: SubNavItem<SecurityTabId>[] = useMemo(
    () =>
      SECURITY_GROUPS[activeGroup].map((tab) => {
        const reason = gateFor(tab);
        return {
          id: tab,
          label: labelFor(tab),
          icon: securityTabIcons[tab],
          disabled: Boolean(reason),
          disabledReason: reason,
        };
      }),
    [activeGroup, labelFor, gateFor],
  );

  const selectGroup = (group: SecurityGroupId) => {
    if (group === activeGroup) return;
    onTabChange(entryTab(group));
  };

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <div className="flex flex-col gap-2">
        {/* Level 1 — the five sections. */}
        <div role="tablist" aria-label={ariaLabel} className="flex flex-wrap items-center gap-1">
          {securityGroupIDs.map((group) => {
            const Icon = securityGroupIcons[group];
            const active = group === activeGroup && activePluginTab === null;
            return (
              <TabContextMenu key={group} items={tabContextMenuItems(entryTab(group))}>
                <Button
                  // A single-destination section renders no rail, so this
                  // button is the only element the panel's
                  // `aria-labelledby` can point at. It borrows the rail's
                  // id scheme so the page has one rule rather than two;
                  // Management skips it, because its rail owns those ids.
                  id={hasSubRail(group) ? undefined : `security-tab-${SECURITY_GROUPS[group][0]}`}
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
            idPrefix="security-tab"
            controls="security-detail-panel"
          />
        )}

        {/* Level 2 — the six under Management. The other four sections are
            each their own destination and render no rail. */}
        {activePluginTab === null && subTabs.length > 1 && (
          <SubNav
            items={subTabs}
            value={activeTab}
            onChange={onTabChange}
            ariaLabel={groupLabel(activeGroup)}
            idPrefix="security-tab"
            controls="security-detail-panel"
            itemWrapper={(tab, node) => (
              <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
            )}
          />
        )}
      </div>
    </div>
  );
}
