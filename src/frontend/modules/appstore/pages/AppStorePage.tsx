// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { useAppCatalog, useAppCategories } from '../hooks/useAppStore';
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

  const { data: catalogData, isLoading } = useAppCatalog(
    category || undefined,
    search || undefined
  );
  const { data: categories } = useAppCategories();

  const apps = catalogData?.items ?? [];
  const visibleApps = installedOnly
    ? apps.filter((app) => app.installed || app.installations.length > 0)
    : apps;
  const totalCount = categories?.reduce((sum, c) => sum + c.count, 0) ?? 0;

  const handleInstall = (app: AppTemplate) => {
    setDetailApp(null);
    setInstallApp(app);
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      {/* Actions bar */}
      <div className="flex shrink-0 items-center gap-3">
        <div className="relative max-w-md flex-1">
          <IconSearch className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            placeholder={t('appStore.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
          />
        </div>
        <label className="flex h-8 shrink-0 items-center gap-2 rounded-lg border border-border bg-card px-3 text-xs font-medium text-foreground shadow-sm">
          <Switch
            checked={installedOnly}
            onCheckedChange={setInstalledOnly}
            aria-label={t('appStore.installedOnly')}
          />
          <span>{t('appStore.installedOnly')}</span>
        </label>
      </div>

      {/* Content */}
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-hidden lg:flex-row">
        {categories && (
          <CategorySidebar
            categories={categories}
            selected={category}
            onSelect={setCategory}
            totalCount={totalCount}
          />
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
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {visibleApps.map((app) => (
                <AppCard
                  key={app.slug}
                  app={app}
                  onInstall={handleInstall}
                  onViewDetail={setDetailApp}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      <AppDetailDialog
        app={detailApp}
        open={!!detailApp}
        onOpenChange={(open) => !open && setDetailApp(null)}
        onInstall={handleInstall}
      />
      <InstallDialog
        app={installApp}
        open={!!installApp}
        onOpenChange={(open) => !open && setInstallApp(null)}
      />
    </div>
  );
}

