// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconApps, IconPuzzle, IconLayoutGrid, IconPlug } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@resources/components/ui/Tabs';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { NodesTab } from '../components/NodesTab';
import { WidgetsTab } from '../components/WidgetsTab';
import { PluginMountBoundary } from '@resources/components/PluginMountBoundary';

const AppsTab = lazy(() => import('./AppStorePage'));
const PluginsTab = lazy(() => import('./PluginStorePage'));

const TAB_IDS = ['apps', 'nodes', 'widgets', 'plugins'] as const;
type TabId = (typeof TAB_IDS)[number];

function TabFallback() {
  return (
    <div className="flex h-48 items-center justify-center">
      <Spinner size="lg" />
    </div>
  );
}

export default function StorePage() {
  const { t } = useTranslation('common');
  const [activeTab, setActiveTab] = useTabFromUrl<TabId>(TAB_IDS, 'apps', {
    replace: true,
    storageKey: 'mcharbor-tabs:store',
  });
  const tabCommands = useMemo(
    () => [
      { id: 'apps' as const, label: t('store.tabApps'), icon: IconApps },
      { id: 'nodes' as const, label: t('store.tabNodes'), icon: IconPuzzle },
      {
        id: 'widgets' as const,
        label: t('store.tabWidgets'),
        icon: IconLayoutGrid,
      },
      { id: 'plugins' as const, label: t('store.tabPlugins'), icon: IconPlug },
    ],
    [t],
  );

  useTabCommands({
    pageId: 'store',
    pageLabel: t('store.title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['store', 'catalog'],
  });
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabCommands,
    activeTab,
    onTabChange: setActiveTab,
  });

  // Each catalog is a separate read on the backend. A tab the viewer cannot
  // load renders disabled with the key it wants, rather than opening onto an
  // empty grid that looks like an empty catalog.
  const nodesGate = usePermissionGate(PERM.STORE_NODES_VIEW);
  const widgetsGate = usePermissionGate(PERM.STORE_WIDGETS_VIEW);
  const pluginsGate = usePermissionGate(PERM.PLUGINS_VIEW);

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader title={t('store.title')} description={t('store.description')} headingLeaf />

      <Tabs
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as TabId)}
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden"
      >
        <div className="shrink-0 overflow-x-auto">
          <TabsList className="min-w-max">
            <TabsTrigger value="apps" className="gap-1.5" contextMenuItems={tabContextMenuItems('apps')}>
              <IconApps className="size-4" />
              {t('store.tabApps')}
            </TabsTrigger>
            <TabsTrigger
              value="nodes"
              className="gap-1.5"
              disabled={nodesGate.denied}
              title={nodesGate.denied ? nodesGate.reason : undefined}
              contextMenuItems={tabContextMenuItems('nodes')}
            >
              <IconPuzzle className="size-4" />
              {t('store.tabNodes')}
            </TabsTrigger>
            <TabsTrigger
              value="widgets"
              className="gap-1.5"
              disabled={widgetsGate.denied}
              title={widgetsGate.denied ? widgetsGate.reason : undefined}
              contextMenuItems={tabContextMenuItems('widgets')}
            >
              <IconLayoutGrid className="size-4" />
              {t('store.tabWidgets')}
            </TabsTrigger>
            <TabsTrigger
              value="plugins"
              className="gap-1.5"
              disabled={pluginsGate.denied}
              title={pluginsGate.denied ? pluginsGate.reason : undefined}
              contextMenuItems={tabContextMenuItems('plugins')}
            >
              <IconPlug className="size-4" />
              {t('store.tabPlugins')}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="apps" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <PluginMountBoundary>
            <Suspense fallback={<TabFallback />}>
              <AppsTab />
            </Suspense>
          </PluginMountBoundary>
        </TabsContent>

        <TabsContent value="nodes" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <NodesTab />
        </TabsContent>

        <TabsContent value="widgets" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <WidgetsTab />
        </TabsContent>

        <TabsContent value="plugins" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <PluginMountBoundary>
            <Suspense fallback={<TabFallback />}>
              <PluginsTab />
            </Suspense>
          </PluginMountBoundary>
        </TabsContent>
      </Tabs>
    </div>
  );
}
