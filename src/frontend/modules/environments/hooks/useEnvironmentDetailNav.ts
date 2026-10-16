// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { isPluginTabId, usePluginPageTabs, type PluginTabId } from '@core/plugins/usePluginPageTabs';
import { environmentDetailTabs, type EnvironmentDetailTab } from './useEnvironmentDetailState';
import type { EnvironmentInfo } from './useEnvironments';

export function useEnvironmentDetailNav(id: string, env?: EnvironmentInfo) {
  const { t } = useTranslation('environments');
  const [urlTab, setUrlTab] = useTabFromUrl<EnvironmentDetailTab | PluginTabId>(environmentDetailTabs, 'overview', {
    storageKey: `mcharbor-tabs:environment:${id}`,
    accepts: isPluginTabId,
  });
  const pluginTabs = usePluginPageTabs('environment');
  // A plugin tab owns the whole panel while it is open; the built-in
  // destinations are hidden rather than rendered behind it. Only a tab the
  // installed plugins still declare is honoured, so a stale link lands on the
  // built-in view instead of an empty panel.
  const pluginTab = isPluginTabId(urlTab) && pluginTabs.some((tab) => tab.id === urlTab) ? urlTab : null;
  // Keeps the reader's place behind an open plugin tab.
  const [lastBuiltInTab, setLastBuiltInTab] = useState<EnvironmentDetailTab>('overview');
  if (!isPluginTabId(urlTab) && urlTab !== lastBuiltInTab) setLastBuiltInTab(urlTab);
  const activeTab: EnvironmentDetailTab = isPluginTabId(urlTab) ? lastBuiltInTab : urlTab;
  const setActiveTab = (tab: EnvironmentDetailTab) => setUrlTab(tab);
  const nativeAgent = env?.agentInstallKind === 'native';
  const hiddenTabs = useMemo<EnvironmentDetailTab[]>(() => (nativeAgent ? [] : ['services']), [nativeAgent]);
  const setPluginTab = (tabId: string) => setUrlTab(tabId as PluginTabId);

  const tabCommands = useMemo(
    () =>
      environmentDetailTabs
        .filter((tabId) => !hiddenTabs.includes(tabId))
        .map((tabId) => ({
          id: tabId,
          label: t(`detail.tabs.${tabId}`),
        })),
    [t, hiddenTabs],
  );

  useTabCommands({
    pageId: `environment:${id}`,
    pageLabel: env?.name ?? t('title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['environment', env?.name ?? id],
  });

  return { pluginTabs, pluginTab, activeTab, setActiveTab, setPluginTab, nativeAgent, hiddenTabs };
}
