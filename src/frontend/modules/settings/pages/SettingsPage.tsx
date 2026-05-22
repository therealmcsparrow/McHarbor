// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import {
  IconSettings,
  IconLock,
  IconRobot,
  IconWebhook,
  IconPuzzle,
  IconClock,
  IconMail,
  IconMessageCircle,
  IconAlertTriangle,
  IconInfoCircle,
  IconRefresh,
} from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { useSettings } from '../hooks/useSettings';
import { GeneralTab } from '../components/GeneralTab';
import { HttpsTab } from '../components/HttpsTab';
import { AgentTab } from '../components/AgentTab';
import { WebhooksTab } from '../components/WebhooksTab';
import { PluginsTab } from '../components/PluginsTab';
import { SchedulesTab } from '../components/SchedulesTab';
import { EmailTab } from '../components/EmailTab';
import { CommunicationsTab } from '../components/CommunicationsTab';
import { AlertsTab } from '../components/AlertsTab';
import { AboutTab } from '../components/AboutTab';
import { UpdatesTab } from '../components/UpdatesTab';

const TAB_IDS = ['general', 'https', 'email', 'communications', 'alerts', 'agent', 'webhooks', 'plugins', 'schedules', 'updates', 'about'] as const;
type TabId = (typeof TAB_IDS)[number];

const TAB_ICONS: Record<TabId, TablerIcon> = {
  general: IconSettings,
  https: IconLock,
  email: IconMail,
  communications: IconMessageCircle,
  alerts: IconAlertTriangle,
  agent: IconRobot,
  webhooks: IconWebhook,
  plugins: IconPuzzle,
  schedules: IconClock,
  updates: IconRefresh,
  about: IconInfoCircle,
};

function isValidTab(tab: string | null): tab is TabId {
  return tab !== null && (TAB_IDS as readonly string[]).includes(tab);
}

export default function SettingsPage() {
  const { t } = useTranslation('settings');
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get('tab');
  const [activeTab, setActiveTab] = useState<TabId>(isValidTab(tabParam) ? tabParam : 'general');
  const { data: settings } = useSettings();

  useEffect(() => {
    if (isValidTab(tabParam) && tabParam !== activeTab) {
      setActiveTab(tabParam);
    }
  }, [tabParam]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleTabChange = (tabId: TabId) => {
    setActiveTab(tabId);
    if (tabId === 'general') {
      setSearchParams({});
    } else {
      setSearchParams({ tab: tabId });
    }
  };

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
                onClick={() => handleTabChange(tabId)}
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
          {activeTab === 'general' && <GeneralTab settings={settings} />}
          {activeTab === 'https' && <HttpsTab />}
          {activeTab === 'email' && <EmailTab />}
          {activeTab === 'communications' && <CommunicationsTab />}
          {activeTab === 'alerts' && <AlertsTab />}
          {activeTab === 'agent' && <AgentTab />}
          {activeTab === 'webhooks' && <WebhooksTab />}
          {activeTab === 'plugins' && <PluginsTab />}
          {activeTab === 'schedules' && <SchedulesTab />}
          {activeTab === 'updates' && <UpdatesTab />}
          {activeTab === 'about' && <AboutTab />}
        </div>
      </div>
    </div>
  );
}
