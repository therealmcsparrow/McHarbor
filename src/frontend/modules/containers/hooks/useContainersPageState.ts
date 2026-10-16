// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useDeferredValue, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import type { SearchMode } from '@resources/utils/search-filter';
import { useUrlSearchState } from '@resources/hooks/useUrlSearchState';
import {
  CONTAINER_STATE_FACETS,
  matchesStateFacet,
  type ContainerStateFacet,
} from '../components/ContainerStateFilterChips';
import { filterContainers } from '../utils/container-search';

const searchModes = ['contains', 'exact', 'regex'] as const;

/** Holds the dialog targets and open flags the containers page drives. */
export function useContainersPageDialogState() {
  const [removeTarget, setRemoveTarget] = useState<ContainerInfo | null>(null);
  const [moveTarget, setMoveTarget] = useState<ContainerInfo | null>(null);
  const [renameTarget, setRenameTarget] = useState<ContainerInfo | null>(null);
  const [terminalTarget, setTerminalTarget] = useState<ContainerInfo | null>(null);
  const [logsTarget, setLogsTarget] = useState<ContainerInfo | null>(null);
  const [takeOverTarget, setTakeOverTarget] = useState<ContainerInfo | null>(null);
  const [recreateAllConfirmOpen, setRecreateAllConfirmOpen] = useState(false);
  // Single-container recreate. It runs through the same batch-progress
  // machinery as "Recreate All" — a one-item batch — so the log, the toast
  // and the cache invalidation are identical no matter how it was started.
  const [recreateTarget, setRecreateTarget] = useState<ContainerInfo | null>(null);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);
  // Create is a dialog, not a route; the toolbar button opens it in place.
  const [createOpen, setCreateOpen] = useState(false);
  const [bulkRemoveTargets, setBulkRemoveTargets] = useState<ContainerInfo[]>([]);
  return {
    removeTarget,
    setRemoveTarget,
    moveTarget,
    setMoveTarget,
    renameTarget,
    setRenameTarget,
    terminalTarget,
    setTerminalTarget,
    logsTarget,
    setLogsTarget,
    takeOverTarget,
    setTakeOverTarget,
    recreateAllConfirmOpen,
    setRecreateAllConfirmOpen,
    recreateTarget,
    setRecreateTarget,
    pruneConfirmOpen,
    setPruneConfirmOpen,
    createOpen,
    setCreateOpen,
    bulkRemoveTargets,
    setBulkRemoveTargets,
  };
}

/** Collects the ids of containers whose last update check found a newer image. */
export function useUpdateAvailableIds(
  updateResults: Map<string, { containerId: string; updateAvailable: boolean }> | undefined,
) {
  return new Set(
    updateResults
      ? [...updateResults.values()].filter((result) => result.updateAvailable).map((result) => result.containerId)
      : [],
  );
}

/** Owns the URL-backed search and state-facet filters and the rows they leave visible. */
export function useContainersPageFilters(containers: ContainerInfo[], updateAvailableIDs: Set<string>) {
  const { t } = useTranslation('containers');
  const [search, setSearch] = useUrlSearchState<string>('');
  const [searchMode, setSearchMode] = useUrlSearchState<SearchMode>('contains', {
    param: 'mode',
    allowed: searchModes,
  });
  // The state facet joins the URL alongside the query, so a filtered view is
  // a shareable link rather than something you have to describe.
  const [stateFacet, setStateFacet] = useUrlSearchState<ContainerStateFacet>('all', {
    param: 'state',
    allowed: CONTAINER_STATE_FACETS,
  });
  const deferredSearch = useDeferredValue(search);
  const searchResult = filterContainers(containers, deferredSearch, searchMode);
  // The facet composes with the query — it narrows what the search returns
  // rather than replacing it.
  const visibleContainers = searchResult.matches.filter((container) =>
    matchesStateFacet(container, stateFacet, updateAvailableIDs),
  );
  return {
    search,
    setSearch,
    searchMode,
    setSearchMode,
    stateFacet,
    setStateFacet,
    searchProps: {
      value: search,
      onChange: setSearch,
      mode: searchMode,
      onModeChange: setSearchMode,
      placeholder: t('structuredSearch.placeholder'),
      error: searchResult.error !== null,
    },
    savedFilters: {
      state: { search, mode: searchMode, state: stateFacet },
      onRestore: (saved: Record<string, unknown>) => {
        setSearch(typeof saved.search === 'string' ? saved.search : '');
        if (saved.mode === 'contains' || saved.mode === 'exact' || saved.mode === 'regex') setSearchMode(saved.mode);
        if (typeof saved.state === 'string') setStateFacet(saved.state as ContainerStateFacet);
      },
      pristine: search === '' && stateFacet === 'all',
      defaults: { search: '', mode: 'contains', state: 'all' },
    },
    visibleContainers,
  };
}
