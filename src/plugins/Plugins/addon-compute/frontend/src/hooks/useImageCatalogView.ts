// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { SearchMode } from '@resources/utils/search-filter';
import { distroLabel } from '../distroLogo';
import type { ComputeImage, ImageKind } from '../types';
import { useImageFetchWatcher, useImages } from './useCompute';

export type KindFacet = ImageKind | 'all';

/** Fetches the catalog and owns the search, OS and kind filters the Images page narrows it with. */
export function useImageCatalogView(environmentId: string) {
  const { t } = useTranslation('compute');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [kind, setKind] = useState<KindFacet>('all');
  const [distro, setDistro] = useState('all');

  const { data, isLoading, isFetching, isError, refetch } = useImages(environmentId || undefined, undefined);
  useImageFetchWatcher(data);

  const all = useMemo(() => data ?? [], [data]);

  const operatingSystems = useMemo(() => {
    const counts = new Map<string, number>();
    for (const image of all) {
      counts.set(image.distro, (counts.get(image.distro) ?? 0) + 1);
    }
    return [...counts.entries()]
      .map(([value, count]) => ({ value, label: `${distroLabel(value)} (${count})` }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [all]);

  const byDistro = useMemo(
    () => (distro === 'all' ? all : all.filter((image) => image.distro === distro)),
    [all, distro],
  );

  const images = useMemo(
    () => (kind === 'all' ? byDistro : byDistro.filter((image) => image.kind === kind)),
    [byDistro, kind],
  );

  const searchFields = useCallback(
    (image: ComputeImage) => [
      image.displayName,
      image.distro,
      image.release,
      image.variant,
      image.arch,
      t(`images.kind.${image.kind}`),
    ],
    [t],
  );

  return {
    isLoading,
    isFetching,
    isError,
    refetch,
    query,
    setQuery,
    searchMode,
    setSearchMode,
    kind,
    setKind,
    distro,
    setDistro,
    operatingSystems,
    byDistro,
    images,
    searchFields,
  };
}
