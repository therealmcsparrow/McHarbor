// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconInfoCircle,
  IconDatabase,
  IconNetwork,
  IconChartBar,
} from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { DockerInfoTab } from '../components/DockerInfoTab';
import { RegistriesTab } from '../components/RegistriesTab';
import { DockerNetworksTab } from '../components/DockerNetworksTab';
import { ResourcesTab } from '../components/ResourcesTab';

const TAB_IDS = ['info', 'registries', 'networks', 'resources'] as const;
type TabId = (typeof TAB_IDS)[number];

const TAB_ICONS: Record<TabId, TablerIcon> = {
  info: IconInfoCircle,
  registries: IconDatabase,
  networks: IconNetwork,
  resources: IconChartBar,
};

export default function DockerPage() {
  const { t } = useTranslation('docker');
  const [activeTab, setActiveTab] = useState<TabId>('info');

  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} />

      <div className="flex gap-6">
        <nav className="flex w-fit bg-muted rounded-lg p-1 gap-x-1">
          {TAB_IDS.map((tabId) => {
            const Icon = TAB_ICONS[tabId];
            return (
              <Button
                key={tabId}
                variant="ghost"
                onClick={() => setActiveTab(tabId)}
                className={cn(
                  'py-2.5 px-4 inline-flex items-center gap-x-2 text-sm font-medium rounded-lg transition-colors',
                  activeTab === tabId
                    ? 'bg-card text-foreground shadow-sm'
                    : 'bg-transparent text-muted-foreground hover:text-primary'
                )}
              >
                <Icon className="size-4" />
                {t(`tabs.${tabId}`)}
              </Button>
            );
          })}
        </nav>
      </div>

      <div className="flex gap-6">
        <div className="min-w-0 flex-1 rounded-xl border border-border bg-card p-6">
          {activeTab === 'info' && <DockerInfoTab />}
          {activeTab === 'registries' && <RegistriesTab />}
          {activeTab === 'networks' && <DockerNetworksTab />}
          {activeTab === 'resources' && <ResourcesTab />}
        </div>
      </div>
    </div>
  );
}
