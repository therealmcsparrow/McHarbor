// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch, IconRefresh } from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useAppCatalog, useAppCategories, useSyncCatalog, useSyncStatus } from '../hooks/useAppStore';
import { useLoadMoreSentinel } from '../hooks/useLoadMoreSentinel';
import { AppCard } from '../components/AppCard';
import { CategorySidebar } from '../components/CategorySidebar';
import { AppDetailDialog } from '../components/AppDetailDialog';
import { InstallDialog } from '../components/InstallDialog';
import type { AppTemplate } from '../types';

export default function AppsTab() {
  const { t } = useTranslation('common');
  const [category, setCategory] = useState('');
  const [search, setSearch] = useState('');
  const [installedOnly, setInstalledOnly] = useState(false);
  const [detailApp, setDetailApp] = useState<AppTemplate | null>(null);
  const [installApp, setInstallApp] = useState<AppTemplate | null>(null);

  const {
    data: catalogData,
    isLoading,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useAppCatalog(category || undefined, search || undefined);
  const { data: categories } = useAppCategories();
  const syncCatalog = useSyncCatalog();
  const { data: syncStatus } = useSyncStatus();

  const apps = catalogData?.pages.flatMap((p) => p.items) ?? [];
  const matchingCount = catalogData?.pages[0]?.total ?? 0;
  const canLoadMore = !!hasNextPage && !isFetchingNextPage;
  const sentinelRef = useLoadMoreSentinel<HTMLDivElement>(canLoadMore, apps.length, () => void fetchNextPage());
  const visibleApps = installedOnly ? apps.filter((app) => app.installed || app.installations.length > 0) : apps;
  const totalCount = categories?.reduce((sum, c) => sum + c.count, 0) ?? 0;

  const handleInstall = (app: AppTemplate) => {
    setDetailApp(null);
    setInstallApp(app);
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      {/* Actions bar */}
      <div className="flex shrink-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center">
        <div className="relative max-w-md min-w-0 flex-1">
          <IconSearch className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            placeholder={t('appStore.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="mc-control w-full rounded-md border border-border bg-background pr-3 pl-9 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
          />
        </div>
        {/* Nothing schedules a catalog refresh - no ticker, no cron, no
            background job - so without this button the catalog only ever held
            whatever it was last seeded with. The route and both hooks already
            existed; only the control was missing. */}
        <Button
          variant="outline"
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => syncCatalog.mutate()}
          disabled={syncCatalog.isPending}
          title={syncStatus?.lastSyncedAt ?? undefined}
        >
          <IconRefresh className="size-4" aria-hidden="true" />
          {syncCatalog.isPending ? t('appStore.syncing') : t('appStore.sync')}
        </Button>
        <label className="mc-control mc-glass flex shrink-0 items-center gap-2 rounded-lg px-3 text-xs font-medium text-foreground shadow-sm">
          <Switch checked={installedOnly} onCheckedChange={setInstalledOnly} aria-label={t('appStore.installedOnly')} />
          <span>{t('appStore.installedOnly')}</span>
        </label>
      </div>

      {/* Content */}
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-hidden lg:flex-row">
        {categories && (
          <CategorySidebar categories={categories} selected={category} onSelect={setCategory} totalCount={totalCount} />
        )}

        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          {isLoading ? (
            <div className="flex h-full min-h-48 items-center justify-center">
              <Spinner size="lg" />
            </div>
          ) : visibleApps.length === 0 ? (
            <div className="flex h-full min-h-48 items-center justify-center text-sm text-muted-foreground">
              {installedOnly ? t('appStore.noInstalledApps') : t('appStore.noApps')}
            </div>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                {visibleApps.map((app) => (
                  <AppCard key={app.slug} app={app} onInstall={handleInstall} onViewDetail={setDetailApp} />
                ))}
              </div>
              <div ref={sentinelRef} className="flex flex-col items-center gap-2 py-6 text-xs text-muted-foreground">
                <span>
                  {t('appStore.showingCount', {
                    shown: apps.length,
                    total: matchingCount,
                  })}
                </span>
                {hasNextPage && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void fetchNextPage()}
                    disabled={isFetchingNextPage}
                  >
                    {isFetchingNextPage ? <Spinner size="sm" /> : null}
                    {t('appStore.loadMore')}
                  </Button>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      <AppDetailDialog
        app={detailApp}
        open={!!detailApp}
        onOpenChange={(openState) => !openState && setDetailApp(null)}
        onInstall={handleInstall}
      />
      <InstallDialog app={installApp} open={!!installApp} onOpenChange={(isOpen) => !isOpen && setInstallApp(null)} />
    </div>
  );
}
