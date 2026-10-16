// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconAdjustments,
  IconArchive,
  IconCamera,
  IconDeviceDesktop,
  IconFileText,
  IconPlugConnected,
  IconTerminal2,
} from '@tabler/icons-react';
import { SubNav } from '@resources/components/SubNav';

export type DetailTabId = 'overview' | 'console' | 'logs' | 'hardware' | 'snapshots' | 'backups' | 'settings';

const TABS: { id: DetailTabId; icon: typeof IconDeviceDesktop }[] = [
  { id: 'overview', icon: IconDeviceDesktop },
  { id: 'console', icon: IconTerminal2 },
  { id: 'logs', icon: IconFileText },
  { id: 'hardware', icon: IconPlugConnected },
  { id: 'snapshots', icon: IconCamera },
  { id: 'backups', icon: IconArchive },
  { id: 'settings', icon: IconAdjustments },
];

export function VMDetailTabs({
  activeTab,
  onTabChange,
}: {
  activeTab: DetailTabId;
  onTabChange: (tab: DetailTabId) => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <div className="border-b border-border px-5 py-3">
      <SubNav
        items={TABS.map((tab) => ({
          id: tab.id,
          label: t(`detail.tabs.${tab.id}`),
          icon: tab.icon,
        }))}
        value={activeTab}
        onChange={onTabChange}
        ariaLabel={t('detail.title')}
        idPrefix="vm-tab"
        controls="vm-detail-panel"
      />
    </div>
  );
}
