// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { PluginCard } from './PluginCard';
import { StoreSearchGrid } from './StoreSearchGrid';
import { usePluginCatalog } from '../hooks/usePlugins';
import { useInstalledPlugins } from '../hooks/usePlugins';
import type { PluginEntry } from '../types/plugins';
import { isNewerArtifact } from '../pages/pluginStoreCategories';

// ponytail: ponytail ceiling — handles MAJOR.MINOR.PATCH only. Upgrade to
// real semver (Masterminds/semver) when a plugin ships a pre-release.

type PluginCategoryGridProps = {
  slugPrefix: string | null; // null = all add-ons (no prefix filter)
  /**
   * Prefix list to exclude. Lets the catch-all "addons" tab hide items
   * that already have their own category tab (comm-, storage-, etc.) so
   * dedicated Docker apps don't flood the general add-ons bucket.
   */
  excludePrefixes?: string[];
  searchPlaceholderKey: string;
  // Opens the install modal in picker mode. The PluginInstallDialog
  // owns the version selector; we just forward the click. Version is
  // chosen inside the modal, not here.
  onInstall: (app: PluginEntry) => void;
  // Wired through to PluginCard. The parent page owns the dialog +
  // useUninstallPlugin; the grid just forwards the click.
  onUninstall?: (slug: string) => void;
  onViewDetail?: (app: PluginEntry) => void;
  // Update path: hits /api/appstore/plugins/{slug}/update/stream.
  // The card renders an Update button when both `installed` and
  // `hasUpdate` are true, and calls this instead of onInstall.
  onUpdate?: (app: PluginEntry) => void;
  installedOnly?: boolean;
  /**
   * When true, restrict the list to catalog entries whose version is
   * strictly newer than what's currently installed for that slug.
   * Parent (PluginStorePage) sets this only for the "updates" tab.
   * Per-card `hasUpdate` is derived from the same comparison.
   */
  updatesMode?: boolean;
  /**
   * slug → installed version map. Required when `updatesMode` is on;
   * passed through PluginStorePage from useInstalledPlugins().
   */
  installedVersions?: Record<string, string>;
  /**
   * Optional lifted search state. When provided, the grid hides its
   * own search input (the parent renders a global one) and uses the
   * supplied value + setter for filtering. Falling back to local
   * state keeps the grid usable on its own.
   */
  search?: string;
  onSearchChange?: (value: string) => void;
};

export function PluginCategoryGrid({
  slugPrefix,
  excludePrefixes,
  searchPlaceholderKey,
  onInstall,
  onUninstall,
  onViewDetail,
  onUpdate,
  installedOnly = false,
  updatesMode = false,
  installedVersions,
  search: searchProp,
  onSearchChange,
}: PluginCategoryGridProps) {
  const { t } = useTranslation('common');
  const [localSearch, setLocalSearch] = useState('');
  const isControlled = searchProp !== undefined && onSearchChange !== undefined;
  const search = isControlled ? (searchProp as string) : localSearch;
  const setSearch = isControlled ? (onSearchChange as (v: string) => void) : setLocalSearch;

  const query = usePluginCatalog();
  // /api/appstore/plugins/installed. Without this, every installed
  // plugin renders without an "Installed" badge and without an
  // Uninstall button — the cards think nothing has been installed yet.
  const installed = useInstalledPlugins();
  const installedSlugs = useMemo(() => new Set((installed.data ?? []).map((p) => p.slug)), [installed.data]);
  const unsignedSlugs = useMemo(
    () => new Set((installed.data ?? []).filter((p) => !p.signed).map((p) => p.slug)),
    [installed.data],
  );
  const disabledSlugs = useMemo(
    () => new Set((installed.data ?? []).filter((p) => !p.enabled).map((p) => p.slug)),
    [installed.data],
  );
  const installedBuilds = useMemo(() => {
    const map: Record<string, string | undefined> = {};
    for (const row of installed.data ?? []) map[row.slug] = row.build;
    return map;
  }, [installed.data]);
  const repairState = useMemo(() => {
    const map: Record<string, { builtFor: string; error: string }> = {};
    for (const row of installed.data ?? []) {
      if (row.repair_error) {
        map[row.slug] = { builtFor: row.built_for, error: row.repair_error };
      }
    }
    return map;
  }, [installed.data]);

  const all = useMemo(() => query.data ?? [], [query.data]);

  const filtered = useMemo(() => {
    let list = all;
    if (slugPrefix) {
      list = list.filter((p) => p.slug.startsWith(slugPrefix));
    } else if (excludePrefixes && excludePrefixes.length > 0) {
      list = list.filter((p) => !excludePrefixes.some((prefix) => p.slug.startsWith(prefix)));
    }
    if (installedOnly) {
      list = list.filter((p) => installedSlugs.has(p.slug));
    }
    if (updatesMode) {
      list = list.filter((p) => {
        const installedVersion = installedVersions?.[p.slug];
        return (
          installedVersion !== undefined &&
          isNewerArtifact(p.version, p.build, installedVersion, installedBuilds[p.slug])
        );
      });
    }
    if (search) {
      const q = search.toLowerCase();
      list = list.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.description.toLowerCase().includes(q) ||
          p.slug.toLowerCase().includes(q),
      );
    }
    return list;
  }, [
    all,
    slugPrefix,
    excludePrefixes,
    search,
    installedOnly,
    installedSlugs,
    updatesMode,
    installedVersions,
    installedBuilds,
  ]);

  return (
    <StoreSearchGrid
      search={search}
      onSearchChange={setSearch}
      // When the parent page owns the search field (rendered inside the
      // vertical-tab sidebar), suppress the grid's own search input so
      // the same control does not render twice per category tab.
      hideSearch={isControlled}
      searchPlaceholder={t(searchPlaceholderKey, {
        // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: the search placeholder key is supplied by the category descriptor, which may be plugin-supplied
        defaultValue: t('store.searchPlugins'),
      })}
      count={filtered.length}
      emptyMessage={t('store.emptyCategory')}
      noResultsMessage={t('store.noResults')}
      hasItems={all.length > 0}
    >
      {query.isLoading ? (
        <div className="col-span-full flex h-48 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : (
        filtered.map((p) => {
          const installedVersion = installedVersions?.[p.slug];
          const cardHasUpdate =
            installedVersion !== undefined &&
            isNewerArtifact(p.version, p.build, installedVersion, installedBuilds[p.slug]);
          return (
            <PluginCard
              key={p.slug}
              plugin={{ ...p, installed: installedSlugs.has(p.slug) }}
              onInstall={onInstall}
              onUninstall={onUninstall}
              onViewDetail={onViewDetail}
              onUpdate={onUpdate}
              hasUpdate={cardHasUpdate}
              installedVersion={installedVersion}
              installedUnsigned={unsignedSlugs.has(p.slug)}
              installedEnabled={!disabledSlugs.has(p.slug)}
              needsRepair={repairState[p.slug] !== undefined}
              builtFor={repairState[p.slug]?.builtFor}
              repairError={repairState[p.slug]?.error}
            />
          );
        })
      )}
    </StoreSearchGrid>
  );
}
