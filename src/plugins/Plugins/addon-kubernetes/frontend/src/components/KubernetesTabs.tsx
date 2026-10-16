// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// KubernetesTabs — the group's destinations, on the app's segmented rail.
//
// Rendered inside the `ClusterHeader` card, in the same `px-3 py-2` band
// `NetworkDetailTabs` uses inside the network hero — which is why there is no
// negative gutter here any more: it existed to pull the rail flush with the
// page edge back when the rail sat loose on the page background.
//
// `SubNav` is a host component reached through `@resources`, and it is pure
// presentation: no router, no store, nothing that needs a context this bundle
// cannot provide. It comes across unchanged, so the plugin's rail is the same
// sliding pill — and the same roving-tabindex and arrow-key behaviour — as
// Security, container detail and volume detail.
//
// The per-tab context menu comes from `useTabMenu` rather than the host's
// `useTabContextMenu`: that hook reads react-router's `useLocation()`, which
// this bundle has no Router context for. `TabContextMenu` itself is pure
// presentation and comes across unchanged.

import { type ReactElement, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { TablerIcon } from '@tabler/icons-react';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu, type TabContextMenuItem } from '@resources/components/ui/Tabs';
import type { TabId } from '../nav';

export type KubernetesTabCommand = {
  id: TabId;
  label: string;
  icon: TablerIcon;
};

type KubernetesTabsProps = {
  tabs: KubernetesTabCommand[];
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
  /** Builds the right-click menu for one tab. */
  contextMenuItems: (tab: TabId) => TabContextMenuItem[];
};

export function KubernetesTabs({ tabs, activeTab, onTabChange, contextMenuItems }: KubernetesTabsProps) {
  const { t } = useTranslation('kubernetes');

  const items = useMemo<SubNavItem<TabId>[]>(
    () => tabs.map((tab) => ({ id: tab.id, label: tab.label, icon: tab.icon })),
    [tabs],
  );

  return (
    <SubNav
      items={items}
      value={activeTab}
      onChange={onTabChange}
      ariaLabel={t('tabs.label')}
      idPrefix="kubernetes-tab"
      controls="kubernetes-panel"
      itemWrapper={(tab, node) => <TabContextMenu items={contextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>}
    />
  );
}
