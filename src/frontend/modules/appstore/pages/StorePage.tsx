// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { IconApps, IconPuzzle, IconLayoutGrid } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@resources/components/ui/Tabs';
import { NodesTab } from '../components/NodesTab';
import { WidgetsTab } from '../components/WidgetsTab';

const AppsTab = lazy(() => import('./AppStorePage'));

function TabFallback() {
  return (
    <div className="flex h-48 items-center justify-center">
      <Spinner size="lg" />
    </div>
  );
}

export default function StorePage() {
  const { t } = useTranslation('common');

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader
        title={t('store.title')}
        description={t('store.description')}
      />

      <Tabs defaultValue="apps" className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden">
        <div className="shrink-0 overflow-x-auto">
          <TabsList className="min-w-max">
            <TabsTrigger value="apps" className="gap-1.5">
              <IconApps className="size-4" />
              {t('store.tabApps')}
            </TabsTrigger>
            <TabsTrigger value="nodes" className="gap-1.5">
              <IconPuzzle className="size-4" />
              {t('store.tabNodes')}
            </TabsTrigger>
            <TabsTrigger value="widgets" className="gap-1.5">
              <IconLayoutGrid className="size-4" />
              {t('store.tabWidgets')}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="apps" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <Suspense fallback={<TabFallback />}>
            <AppsTab />
          </Suspense>
        </TabsContent>

        <TabsContent value="nodes" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <NodesTab />
        </TabsContent>

        <TabsContent value="widgets" className="mt-0 min-h-0 flex-1 overflow-hidden">
          <WidgetsTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}

