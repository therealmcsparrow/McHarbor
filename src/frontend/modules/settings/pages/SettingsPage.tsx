// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { PluginTabPanel } from '@core/plugins/PluginTabPanel';
import { isPluginTabId, parsePluginTabId, usePluginPageTabs, type PluginTabId } from '@core/plugins/usePluginPageTabs';
import { Card } from '@resources/components/ui/Card';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { settingsTabIDs, settingsTabIcons, type SettingsTabId } from '../components/settings-tab-groups';
import { SettingsSectionHero } from '../components/SettingsSectionHero';
import { SettingsTabs } from '../components/SettingsTabs';
import { useHiddenSettingsTabs } from '../components/useHiddenSettingsTabs';
import { GeneralTab } from '../components/GeneralTab';
import { NotificationsTab } from '../components/NotificationsTab';
import { AgentTab } from '../components/AgentTab';
import { DatabaseSettingsPanel } from '../database/components/DatabaseSettingsPanel';
import { InstanceBackupPanel } from '../instance-backup/components/InstanceBackupPanel';
import { DemoModePanel } from '../demo/components/DemoModePanel';
import { WebhooksTab } from '../components/WebhooksTab';
import { SchedulesTab } from '../components/SchedulesTab';
import { CommunicationsTab } from '../components/CommunicationsTab';
import { StorageTab } from '../components/StorageTab';
import { SnapshotsTab } from '../components/SnapshotsTab';
import { AlertsTab } from '../components/AlertsTab';
import { RetentionTab } from '../components/RetentionTab';
import { AboutTab } from '../components/AboutTab';
import { ContactTab } from '../components/ContactTab';
import { LicenseTab } from '../components/LicenseTab';
import { ContainerUpdatesTab } from '../components/ContainerUpdatesTab';
import { AgentUpdatesTab } from '../components/AgentUpdatesTab';
import { HostUpdatesTab } from '../components/HostUpdatesTab';
import { HostServicesTab } from '../components/HostServicesTab';
import { McHarborUpdatesTab } from '../components/McHarborUpdatesTab';
import { CatalogsTab } from '../components/CatalogsTab';
import { ThemeTab } from '../components/ThemeTab';
import { AIProvidersTab } from '../components/AIProvidersTab';
import { LoggingTab } from '../components/LoggingTab';

export default function SettingsPage() {
  const { t } = useTranslation('settings');
  const [urlTab, setUrlTab] = useTabFromUrl<SettingsTabId | PluginTabId>(settingsTabIDs, 'general', {
    // `?tab=updates` predates the Updates group; it lands on McHarbor,
    // which is where the old section's headline controls now live.
    aliases: { email: 'communications', updates: 'updates-mcharbor' },
    storageKey: 'mcharbor-tabs:settings',
    // A plugin's tab id cannot be in the list above; it exists only once
    // that plugin is installed.
    accepts: isPluginTabId,
  });
  const pluginTabs = usePluginPageTabs('settings');
  // Only honour a plugin tab the installed plugins still declare: a link to a
  // tab whose plugin was removed lands on the built-in view rather than an
  // empty panel.
  const pluginTab = isPluginTabId(urlTab) && pluginTabs.some((tab) => tab.id === urlTab) ? urlTab : null;
  const showBuiltIn = pluginTab === null;
  // Keeps the reader's place behind an open plugin tab.
  const [lastBuiltInTab, setLastBuiltInTab] = useState<SettingsTabId>('general');
  if (!isPluginTabId(urlTab) && urlTab !== lastBuiltInTab) setLastBuiltInTab(urlTab);
  const hiddenTabs = useHiddenSettingsTabs();
  const requestedTab: SettingsTabId = isPluginTabId(urlTab) ? lastBuiltInTab : urlTab;
  // A link to a section this instance does not offer opens General instead.
  const activeTab: SettingsTabId = hiddenTabs.has(requestedTab) ? 'general' : requestedTab;
  const setActiveTab = (tab: SettingsTabId) => setUrlTab(tab);
  const setPluginTab = (id: string) => setUrlTab(id as PluginTabId);

  const tabCommands = useMemo(
    () =>
      settingsTabIDs
        .filter((tabId) => !hiddenTabs.has(tabId))
        .map((tabId) => ({
          id: tabId,
          label: t(`tabs.${tabId}`),
          icon: settingsTabIcons[tabId],
        })),
    [t, hiddenTabs],
  );

  useTabCommands({
    pageId: 'settings',
    pageLabel: t('title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['settings'],
  });

  return (
    <div className="flex min-h-0 flex-col gap-4">
      {/* Orientation stays in the topbar, where it keeps the same
          coordinates on every page and does not scroll away. */}
      <PageHeader title={t('title')} description={<SettingsSectionHero activeTab={activeTab} />} />

      {/* The hero, borrowed wholesale from the container detail page:
          identity at the top of the content, the tab navigation inside the
          same card, so the card reads as one object — this is the section,
          this is the group it belongs to, these are its siblings. */}
      <Card className="flex-none overflow-visible">
        <SettingsTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          hiddenTabs={hiddenTabs}
          pluginTabs={pluginTabs}
          activePluginTab={pluginTab}
          onPluginTabChange={setPluginTab}
          labelFor={(tab) => t(`tabs.${tab}`)}
          groupLabelFor={(key) => t(key)}
          ariaLabel={t('nav.sections')}
        />
      </Card>

      {/* The body keeps a surface of its own. The container detail tabs
          each bring their own `mc-glass` sections and sit on the bare
          canvas; several settings sections are plain form stacks that
          would read as loose controls with nothing behind them. */}
      <Card
        id="settings-detail-panel"
        role="tabpanel"
        aria-labelledby={`settings-tab-${activeTab}`}
        className="min-w-0 p-5"
      >
        {pluginTab !== null &&
          (() => {
            const parsed = parsePluginTabId(pluginTab);
            if (!parsed) return null;
            return (
              <PluginTabPanel
                slug={parsed.slug}
                tabKey={parsed.tabKey}
                context={{ pageId: 'settings', tabKey: parsed.tabKey }}
              />
            );
          })()}

        {showBuiltIn && activeTab === 'general' && <GeneralTab />}
        {showBuiltIn && activeTab === 'themes' && <ThemeTab />}
        {showBuiltIn && activeTab === 'notifications' && <NotificationsTab />}
        {showBuiltIn && activeTab === 'ai' && <AIProvidersTab />}
        {showBuiltIn && activeTab === 'communications' && <CommunicationsTab />}
        {showBuiltIn && activeTab === 'catalogs' && <CatalogsTab />}
        {showBuiltIn && activeTab === 'storage' && <StorageTab />}
        {showBuiltIn && activeTab === 'snapshots' && <SnapshotsTab />}
        {showBuiltIn && activeTab === 'logging' && <LoggingTab />}
        {showBuiltIn && activeTab === 'alerts' && <AlertsTab />}
        {showBuiltIn && activeTab === 'retention' && <RetentionTab />}
        {showBuiltIn && activeTab === 'schedules' && <SchedulesTab />}
        {showBuiltIn && activeTab === 'webhooks' && <WebhooksTab />}
        {showBuiltIn && activeTab === 'updates-containers' && <ContainerUpdatesTab />}
        {showBuiltIn && activeTab === 'updates-agents' && <AgentUpdatesTab />}
        {showBuiltIn && activeTab === 'updates-host' && <HostUpdatesTab />}
        {showBuiltIn && activeTab === 'updates-mcharbor' && <McHarborUpdatesTab />}
        {showBuiltIn && activeTab === 'host-services' && <HostServicesTab />}
        {showBuiltIn && activeTab === 'database' && <DatabaseSettingsPanel />}
        {showBuiltIn && activeTab === 'instance-backup' && <InstanceBackupPanel />}
        {showBuiltIn && activeTab === 'demo' && <DemoModePanel />}
        {showBuiltIn && activeTab === 'agent' && <AgentTab />}
        {showBuiltIn && activeTab === 'license' && <LicenseTab />}
        {showBuiltIn && activeTab === 'about' && <AboutTab />}
        {showBuiltIn && activeTab === 'contact' && <ContactTab />}
      </Card>
    </div>
  );
}
