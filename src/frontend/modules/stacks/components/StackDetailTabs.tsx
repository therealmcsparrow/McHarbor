// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StackDetailTabs — the stack's faces, on the container page's rail.
//
// This replaces a hand-rolled `bg-muted` pill nav that predated `SubNav`: it
// drew the same idea with a different set of tokens, so the stack page's tab
// bar and the container page's tab bar looked like two different products.
// The pill now slides between items rather than cross-fading, and the tab
// context menu rides along through `itemWrapper` — `SubNav` does not need to
// know what a context menu is.

import type { ComponentType, ReactElement, ReactNode } from 'react';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';

type TabContextMenuItem = Parameters<typeof TabContextMenu>[0]['items'];

type StackDetailTabsProps<TabId extends string> = {
  tabs: readonly TabId[];
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
  iconFor: (tab: TabId) => ComponentType<{ className?: string }>;
  /** Already translated. */
  labelFor: (tab: TabId) => string;
  /** Already translated. */
  ariaLabel: string;
  /** Right-click menu for one tab, from `useTabContextMenu`. */
  contextMenuFor?: (tab: TabId) => TabContextMenuItem;
};

export function StackDetailTabs<TabId extends string>({
  tabs,
  activeTab,
  onTabChange,
  iconFor,
  labelFor,
  ariaLabel,
  contextMenuFor,
}: StackDetailTabsProps<TabId>) {
  const items: SubNavItem<TabId>[] = tabs.map((tab) => ({
    id: tab,
    label: labelFor(tab),
    icon: iconFor(tab),
  }));

  // `SubNav` hands the wrapper a `ReactNode`; `TabContextMenu` needs a single
  // element to attach its trigger to. SubNav always passes exactly one button,
  // so the narrowing is safe — it just cannot be expressed in the callback's
  // signature without SubNav knowing what a context menu is.
  const itemWrapper = contextMenuFor
    ? (tab: TabId, node: ReactNode) => (
        <TabContextMenu items={contextMenuFor(tab)}>{node as ReactElement}</TabContextMenu>
      )
    : undefined;

  return (
    <div className="border-t border-border bg-transparent px-3 py-2">
      <SubNav
        items={items}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={ariaLabel}
        idPrefix="stack-tab"
        controls="stack-detail-panel"
        itemWrapper={itemWrapper}
      />
    </div>
  );
}
