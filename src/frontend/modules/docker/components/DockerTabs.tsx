// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// DockerTabs — the tab strip that lives inside the Docker hero card.
//
// This is `SettingsTabs` minus its group rail: the same bordered strip, the
// same shared `SubNav` segmented rail with a pill that slides, and the same
// per-item `TabContextMenu` offering switch / reload / copy link / open in
// new tab. What it replaces was a hand-rolled `bg-muted rounded-lg p-1` pill
// row sitting on the bare canvas, which shared no vocabulary with the rest
// of the app and left the tabs visually detached from the content they
// select.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentUserPermissions, hasPermission } from '@core/auth/useCurrentUserPermissions';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import {
  DOCKER_PANEL_ID,
  DOCKER_TAB_ID_PREFIX,
  DOCKER_TAB_PERMISSIONS,
  dockerTabIDs,
  dockerTabIcons,
  type DockerTabId,
} from './docker-tab-groups';

type DockerTabsProps = {
  activeTab: DockerTabId;
  onTabChange: (tab: DockerTabId) => void;
  /** Translator for a section label. Namespace-scoped by the caller. */
  labelFor: (tab: DockerTabId) => string;
  /** Accessible name for the rail. Already translated. */
  ariaLabel: string;
};

export function DockerTabs({ activeTab, onTabChange, labelFor, ariaLabel }: DockerTabsProps) {
  // A section whose own routes the viewer cannot call renders disabled with a
  // tooltip naming the key, rather than opening onto an empty panel or a 403
  // toast. The backend check is still the boundary; this is the honesty.
  const { t: tCommon } = useTranslation('common');
  const { data: permissions } = useCurrentUserPermissions();
  const gateFor = useCallback(
    (tab: DockerTabId) => {
      const required = DOCKER_TAB_PERMISSIONS[tab];
      if (!required || hasPermission(permissions, required)) return undefined;
      return tCommon('permission.requires', { permission: required });
    },
    [permissions, tCommon],
  );

  const tabs: SubNavItem<DockerTabId>[] = useMemo(
    () =>
      dockerTabIDs.map((tab) => {
        const reason = gateFor(tab);
        return {
          id: tab,
          label: labelFor(tab),
          icon: dockerTabIcons[tab],
          disabled: Boolean(reason),
          disabledReason: reason,
        };
      }),
    [labelFor, gateFor],
  );

  const tabContextMenuItems = useTabContextMenu({
    tabs,
    activeTab,
    onTabChange,
  });

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <SubNav
        items={tabs}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={ariaLabel}
        idPrefix={DOCKER_TAB_ID_PREFIX}
        controls={DOCKER_PANEL_ID}
        itemWrapper={(tab, node) => (
          <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
        )}
      />
    </div>
  );
}
