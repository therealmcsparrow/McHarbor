// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { InstalledPlugin, PluginEntry } from '../types/plugins';
import {
  ADDONS_EXCLUDE_PREFIXES,
  ALL_CATEGORY,
  CATEGORIES,
  CATEGORY_FALLBACK_LABELS,
  isNewerArtifact,
} from './pluginStoreCategories';

interface UsePluginStoreCategoriesArgs {
  catalogItems: PluginEntry[];
  installed: InstalledPlugin[] | undefined;
  category: string;
  setCategory: (category: string) => void;
}

// Derives everything the category sidebar needs from the catalog and the
// installed list: the slug → version map, whether the Updates tab has
// anything to show, per-tab counts and labels, and the active tab.
export function usePluginStoreCategories({
  catalogItems,
  installed,
  category,
  setCategory,
}: UsePluginStoreCategoriesArgs) {
  const { t } = useTranslation(['common', 'plugins']);

  // slug → installed version. The Updates tab and the per-card update
  // badge both compare `catalog.version > installed.version`, so the
  // page needs the version map alongside the slug set. Without this,
  // `hasUpdates` only fires when SOMETHING is installed for a slug —
  // it cannot distinguish "1.0.1 catalog vs 1.0.0 installed" (real
  // update) from "1.0.1 catalog vs 1.0.1 installed" (already current).
  const installedVersions = useMemo<Record<string, string>>(() => {
    const map: Record<string, string> = {};
    for (const p of installed ?? []) {
      if (p.slug) map[p.slug] = p.version;
    }
    return map;
  }, [installed]);

  const installedBuilds = useMemo<Record<string, string | undefined>>(() => {
    const map: Record<string, string | undefined> = {};
    for (const p of installed ?? []) {
      if (p.slug) map[p.slug] = p.build;
    }
    return map;
  }, [installed]);

  // The entries themselves, not just a boolean: the Updates tab needs the
  // count to offer "update all", and running that needs the list.
  const updatable = useMemo(
    () =>
      catalogItems.filter((c) => {
        const installedVersion = installedVersions[c.slug];
        return (
          installedVersion !== undefined &&
          isNewerArtifact(c.version, c.build, installedVersion, installedBuilds[c.slug])
        );
      }),
    [catalogItems, installedVersions, installedBuilds],
  );

  const hasUpdates = updatable.length > 0;

  const visibleCategories = useMemo(() => CATEGORIES.filter((c) => c.key !== 'updates' || hasUpdates), [hasUpdates]);

  const categoryCounts = useMemo(
    () =>
      visibleCategories
        .filter((cat) => cat.key !== 'all')
        .map((cat) => {
          let count = catalogItems.length;
          if (cat.key === 'updates') {
            count = catalogItems.filter((entry) => {
              const installedVersion = installedVersions[entry.slug];
              return (
                installedVersion !== undefined &&
                isNewerArtifact(entry.version, entry.build, installedVersion, installedBuilds[entry.slug])
              );
            }).length;
          } else if (cat.key === 'addons') {
            count = catalogItems.filter(
              (entry) => !ADDONS_EXCLUDE_PREFIXES.some((prefix) => entry.slug.startsWith(prefix)),
            ).length;
          } else if (cat.prefix) {
            const prefix = cat.prefix;
            count = catalogItems.filter((entry) => entry.slug.startsWith(prefix)).length;
          }
          return { category: cat.key, count };
        }),
    [catalogItems, installedVersions, installedBuilds, visibleCategories],
  );

  const categoryLabelMap = useMemo(
    () =>
      Object.fromEntries(
        visibleCategories.map((cat) => [
          cat.key,
          t(`tabs.${cat.key}`, {
            ns: 'plugins',
            // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: plugin store categories are open-ended and fall back to a literal label
            defaultValue: CATEGORY_FALLBACK_LABELS[cat.key],
          }),
        ]),
      ),
    [t, visibleCategories],
  );

  const activeCategory = visibleCategories.find((cat) => cat.key === (category || 'all')) ?? ALL_CATEGORY;

  useEffect(() => {
    if (category === 'updates' && !hasUpdates) setCategory('');
  }, [category, hasUpdates, setCategory]);

  return {
    installedVersions,
    categoryCounts,
    categoryLabelMap,
    activeCategory,
    updatable,
  };
}
