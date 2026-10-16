// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconSearch } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { CategorySidebar } from '../components/CategorySidebar';
import { PluginCategoryGrid } from '../components/PluginCategoryGrid';
import { PluginDetailDialog } from '../components/PluginDetailDialog';
import { PluginInstallFlowDialog } from '../components/PluginInstallFlowDialog';
import { UninstallConfirmDialog } from '../components/UninstallConfirmDialog';
import { OperationProgressDialog } from '@resources/components/OperationProgressDialog';
import { usePluginUpdateAll } from '../hooks/usePluginUpdateAll';
import { usePluginCatalog } from '../hooks/usePlugins';
import { useInstalledPlugins } from '../hooks/usePlugins';
import type { InstalledPlugin, PluginEntry, PluginEntry as StrictPluginEntry } from '../types/plugins';
import { ADDONS_EXCLUDE_PREFIXES } from './pluginStoreCategories';
import { usePluginInstallFlow } from './usePluginInstallFlow';
import { usePluginStoreCategories } from './usePluginStoreCategories';

export default function PluginStorePage() {
  // Load both the common and plugins namespaces so we can reference
  // page-level chrome and the
  // plugin-specific tab labels (tabs.updates, tabs.addons, …) from a
  // single t() call. The plugins.json keys are added in 6 locales.
  const { t } = useTranslation(['common', 'plugins']);
  const [detailPlugin, setDetailPlugin] = useState<PluginEntry | null>(null);
  // Held alongside the confirm dialog. We resolve slug -> InstalledPlugin
  // here (not in PluginCard) so the row's metadata (version, enabled
  // flag, permissions_referenced) reaches the dialog straight from the
  // installed-list cache the page already polls.
  const [uninstallSlug, setUninstallSlug] = useState<string | null>(null);

  // Global page-level filters — apply to every category tab. The
  // per-tab search field inside StoreSearchGrid is hidden when this
  // page-level search has a value, so we don't render the same
  // control twice.
  const [search, setSearch] = useState('');
  const [installedOnly, setInstalledOnly] = useState(false);
  const [category, setCategory] = useState('');

  // Reads /api/appstore/plugins/installed — the PluginCategoryGrid
  // flips cards to "Installed" based on this set, so a wrong endpoint
  // means every freshly installed plugin shows no badge and no
  // Uninstall button.
  const installed = useInstalledPlugins();
  const catalog = usePluginCatalog();
  const catalogItems = catalog.data ?? [];
  const { installedVersions, categoryCounts, categoryLabelMap, activeCategory, updatable } = usePluginStoreCategories({
    catalogItems,
    installed: installed.data,
    category,
    setCategory,
  });

  const flow = usePluginInstallFlow(() => setDetailPlugin(null));
  const updateAll = usePluginUpdateAll();

  // Installed list is the source of truth for InstalledPlugin rows.
  // If the user clicks Uninstall on a card whose installed badge is
  // stale (e.g. row already gone between render and click) the lookup
  // returns undefined and the dialog never opens — better than crashing
  // a missing row into the confirm path.
  const uninstallTarget: InstalledPlugin | null = uninstallSlug
    ? ((installed.data ?? []).find((p) => p.slug === uninstallSlug) ?? null)
    : null;
  const uninstallCatalogEntry = uninstallSlug ? (catalogItems.find((c) => c.slug === uninstallSlug) ?? null) : null;

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      <div className="flex shrink-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center">
        <div className="relative max-w-md min-w-0 flex-1">
          <IconSearch className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            variant="outline"
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('store.search', { ns: 'plugins' })}
            aria-label={t('store.search', { ns: 'plugins' })}
            className="pl-9"
          />
        </div>
        {/* Only past one: a single pending update is what the card's own
            button is for, and a second control beside it would just be two
            ways to do the same thing. */}
        {updatable.length > 1 && (
          <Button
            size="sm"
            variant="secondary"
            className="shrink-0"
            permission={PERM.PLUGINS_MANAGE}
            disabled={updateAll.isRunning || flow.dialogOpen}
            onClick={() => {
              setDetailPlugin(null);
              void updateAll.updateAll(updatable);
            }}
          >
            <IconRefresh className="size-4" />
            {t('store.updateAllAction', {
              ns: 'plugins',
              count: updatable.length,
            })}
          </Button>
        )}

        <label className="mc-control mc-glass flex shrink-0 items-center gap-2 rounded-lg px-3 text-xs font-medium text-foreground shadow-sm">
          <Switch
            checked={installedOnly}
            onCheckedChange={setInstalledOnly}
            aria-label={t('catalog.installedOnly', { ns: 'plugins' })}
          />
          <span>{t('catalog.installedOnly', { ns: 'plugins' })}</span>
        </label>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-hidden lg:flex-row">
        <CategorySidebar
          categories={categoryCounts}
          selected={category}
          onSelect={setCategory}
          totalCount={catalogItems.length}
          labelMap={categoryLabelMap}
        />

        <div className="min-h-0 flex-1">
          <PluginCategoryGrid
            slugPrefix={activeCategory.prefix}
            excludePrefixes={activeCategory.key === 'addons' ? ADDONS_EXCLUDE_PREFIXES : undefined}
            searchPlaceholderKey={`store.searchTab${activeCategory.key.charAt(0).toUpperCase() + activeCategory.key.slice(1)}`}
            onInstall={flow.dispatchInstall}
            onUninstall={setUninstallSlug}
            onUpdate={flow.dispatchUpdate}
            onViewDetail={setDetailPlugin}
            installedOnly={installedOnly}
            updatesMode={activeCategory.key === 'updates'}
            installedVersions={installedVersions}
            search={search}
            onSearchChange={setSearch}
          />
        </div>
      </div>

      <PluginDetailDialog
        plugin={detailPlugin}
        open={!!detailPlugin}
        installed={detailPlugin ? installedVersions[detailPlugin.slug] !== undefined : false}
        onOpenChange={(openState) => {
          if (!openState) setDetailPlugin(null);
        }}
        onInstall={flow.dispatchInstall}
      />
      <UninstallConfirmDialog
        plugin={uninstallTarget}
        catalogEntry={uninstallCatalogEntry as StrictPluginEntry | null}
        open={!!uninstallSlug}
        onOpenChange={(isOpen) => {
          if (!isOpen) setUninstallSlug(null);
        }}
      />
      <PluginInstallFlowDialog flow={flow} />
      <OperationProgressDialog state={updateAll.dialogState} onClose={updateAll.closeDialog} />
    </div>
  );
}
