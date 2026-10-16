// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The API-key detail rail.
//
// Same construction as `UserDetailTabs`, and for the same reason: it renders
// inside the hero card so the card reads as one object — this is the
// credential, this is its state, these are the faces of it.

import { type ReactElement, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';
import { TabContextMenu } from '@resources/components/ui/Tabs';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import {
  apiKeyDetailTabIDs,
  apiKeyDetailTabIcons,
  apiKeyDetailTabLabelKeys,
  type APIKeyDetailTabId,
} from './api-key-detail-tabs';

type APIKeyDetailTabsProps = {
  activeTab: APIKeyDetailTabId;
  onTabChange: (tab: APIKeyDetailTabId) => void;
  /** Accessible name for the rail. Already translated. */
  ariaLabel: string;
  /** Counts rendered beside a label. Zero or missing renders nothing. */
  tabBadges?: Partial<Record<APIKeyDetailTabId, number>>;
};

export function APIKeyDetailTabs({ activeTab, onTabChange, ariaLabel, tabBadges }: APIKeyDetailTabsProps) {
  const { t } = useTranslation('security');

  const labelFor = useCallback((tab: APIKeyDetailTabId) => t(apiKeyDetailTabLabelKeys[tab]), [t]);

  const items: SubNavItem<APIKeyDetailTabId>[] = useMemo(
    () =>
      apiKeyDetailTabIDs.map((tab) => {
        const count = tabBadges?.[tab] ?? 0;
        return {
          id: tab,
          label: labelFor(tab),
          icon: apiKeyDetailTabIcons[tab],
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
      icon: apiKeyDetailTabIcons[item.id],
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
        idPrefix="api-key-tab"
        controls="api-key-detail-panel"
        itemWrapper={(tab, node) => (
          <TabContextMenu items={tabContextMenuItems(tab)}>{node as ReactElement}</TabContextMenu>
        )}
      />
    </div>
  );
}
