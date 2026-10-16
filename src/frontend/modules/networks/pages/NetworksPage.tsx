// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLayoutGrid, IconLayoutList, IconPlus } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { getNetworkContainerCount, isBuiltInNetwork } from '@core/utils/network';
import { TablePage } from '@resources/layout/TablePage';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { BulkRemoveNetworkDialog } from '../components/BulkRemoveNetworkDialog';
import { FilterButton } from '../components/FilterButton';
import { NetworkCardGrid } from '../components/NetworkCardGrid';
import { NetworkCreateDialog } from '../components/NetworkCreateDialog';
import { useNetworks } from '../hooks/useNetworks';
import { useNetworksPageConfig } from '../hooks/useNetworksPageConfig';
import { RemoveNetworkDialog } from '../components/RemoveNetworkDialog';
import { useNetworksViewStore } from '../stores/networks-view';
import { PERM } from '@core/auth/permissionKeys';

type UsageFilter = 'all' | 'used' | 'unused';

export default function NetworksPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const { data: networks = [], isLoading, isFetching, refetch } = useNetworks();
  const { viewMode, setViewMode } = useNetworksViewStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [bulkRemoveNetworks, setBulkRemoveNetworks] = useState<NetworkInfo[]>([]);
  const [usageFilter, setUsageFilter] = useState<UsageFilter>('all');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const { columns, batchActions } = useNetworksPageConfig(setConfirmTarget, (rows) =>
    setBulkRemoveNetworks(rows.filter((n) => !isBuiltInNetwork(n))),
  );

  const filteredNetworks = useMemo(() => {
    if (usageFilter === 'all') return networks;
    return networks.filter((n) => {
      const inUse = getNetworkContainerCount(n) > 0;
      return usageFilter === 'used' ? inUse : !inUse;
    });
  }, [networks, usageFilter]);

  const counts = useMemo(() => {
    let used = 0;
    let unused = 0;
    for (const n of networks) {
      if (getNetworkContainerCount(n) > 0) used++;
      else unused++;
    }
    return { all: networks.length, used, unused };
  }, [networks]);

  const usageFilterButtons = (
    <div className="flex items-center gap-1 rounded-lg border border-border p-0.5">
      <FilterButton
        active={usageFilter === 'all'}
        onClick={() => setUsageFilter('all')}
        label={t('filters.all')}
        count={counts.all}
      />
      <FilterButton
        active={usageFilter === 'used'}
        onClick={() => setUsageFilter('used')}
        label={t('filters.used')}
        count={counts.used}
        tone="success"
      />
      <FilterButton
        active={usageFilter === 'unused'}
        onClick={() => setUsageFilter('unused')}
        label={t('filters.unused')}
        count={counts.unused}
      />
    </div>
  );

  // The card view shares the table's query, modes and fields.
  const matcher = useMemo(() => createSearchMatcher(query, searchMode), [query, searchMode]);
  const cardNetworks = useMemo(
    () =>
      query.trim() === ''
        ? filteredNetworks
        : filteredNetworks.filter(
            (network) => matchesSearchFields(query, searchMode, [network.Name, network.Driver, network.Scope]).matched,
          ),
    [filteredNetworks, query, searchMode],
  );

  return (
    <>
      <TablePage
        id="networks"
        title={t('title')}
        description={t('description', { count: networks.length })}
        actions={
          <div className="flex items-center rounded-lg border border-border">
            <Button
              variant={viewMode === 'table' ? 'default' : 'ghost'}
              size="icon-sm"
              onClick={() => setViewMode('table')}
              aria-label={t('tableView')}
            >
              <IconLayoutList className="h-4 w-4" />
            </Button>
            <Button
              variant={viewMode === 'card' ? 'default' : 'ghost'}
              size="icon-sm"
              onClick={() => setViewMode('card')}
              aria-label={t('cardView')}
            >
              <IconLayoutGrid className="h-4 w-4" />
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('searchPlaceholder'),
          error: matcher.error !== null,
          fields: (network) => [network.Name, network.Driver, network.Scope],
        }}
        // The usage facet was a `searchAdornment` on the grid and a duplicate
        // row in the card view; it is one control in the filter band now.
        filters={usageFilterButtons}
        savedFilters={{
          state: { query, mode: searchMode, usage: usageFilter },
          onRestore: (s) => {
            setQuery(typeof s.query === 'string' ? s.query : '');
            if (s.mode === 'contains' || s.mode === 'exact' || s.mode === 'regex') setSearchMode(s.mode);
            if (s.usage === 'all' || s.usage === 'used' || s.usage === 'unused') setUsageFilter(s.usage);
          },
          pristine: query === '' && usageFilter === 'all',
          defaults: { query: '', mode: 'contains', usage: 'all' },
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-networks' },
          actions: (
            <Button permission={PERM.NETWORKS_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('create.title')}
            </Button>
          ),
        }}
        columns={columns}
        data={filteredNetworks}
        loading={isLoading}
        empty={{ title: t('emptyMessage') }}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.Id}
        onRowClick={(row) => navigate(`/networks/${row.Id}`)}
        isRowDisabled={(row) => isBuiltInNetwork(row)}
        getRowClassName={(row) => (isBuiltInNetwork(row) ? 'opacity-70' : undefined)}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <NetworkCardGrid
                networks={cardNetworks}
                isLoading={isLoading}
                onClick={(network) => navigate(`/networks/${network.Id}`)}
                onRemove={(id) => {
                  const target = networks.find((n) => n.Id === id);
                  if (target && isBuiltInNetwork(target)) return;
                  setConfirmTarget(id);
                }}
              />
            </div>
          )
        }
      />

      <NetworkCreateDialog open={createOpen} onOpenChange={setCreateOpen} />

      <RemoveNetworkDialog networkId={confirmTarget} onOpenChange={(next) => !next && setConfirmTarget(null)} />

      <BulkRemoveNetworkDialog
        networks={bulkRemoveNetworks}
        open={bulkRemoveNetworks.length > 0}
        onOpenChange={(next) => !next && setBulkRemoveNetworks([])}
      />
    </>
  );
}
