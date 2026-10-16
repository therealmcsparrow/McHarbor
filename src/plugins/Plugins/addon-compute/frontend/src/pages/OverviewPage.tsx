// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { COMPUTE_PERM } from '../permissions';
import { useOverview } from '../hooks/useCompute';
import { useHostEnvironmentNames } from '../hooks/useHostEnvironments';
import { useComputeView } from '../hooks/useComputeView';
import { hostStatus } from '../overview-status';
import { CardGrid } from '../components/CardGrid';
import { HostCard } from '../components/HostCard';
import { ViewToggle } from '../components/ViewToggle';
import { useComputeOverviewColumns } from '../components/ComputeOverviewColumns';
import type { OverviewEntry } from '../types';

/**
 * The compute landing page: every host, as a table or as cards.
 *
 * Hosts with compute switched off stay in the same list rather than moving to a
 * footnote — an operator looking for a machine they have not set up yet should
 * find it where they looked, with the status column saying why it is not ready.
 */

interface OverviewPageProps {
  onNavigate: (to: string) => void;
  onSetup: (environmentId: string) => void;
}

export function OverviewPage({ onNavigate, onSetup }: OverviewPageProps) {
  const { t } = useTranslation('compute');
  const { data, isLoading, isFetching, isError, refetch } = useOverview();
  const { viewMode, setViewMode } = useComputeView('overview');
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  // The backend can only enumerate environment ids — its DB handle is scoped to
  // this plugin's own tables — so a host's name comes across the bundle
  // boundary from the host bridge. Without this a card reads as a raw xid.
  const hostNames = useHostEnvironmentNames();

  const entries = useMemo<OverviewEntry[]>(
    () =>
      (data?.environments ?? []).map((entry) => ({
        ...entry,
        name: entry.name || hostNames.get(entry.environmentId) || entry.environmentId,
      })),
    [data, hostNames],
  );

  // One field list for both views. TablePage narrows `data` itself for the
  // table; the card grid is handed rows that are already narrowed, so the one
  // filter bar above them governs both instead of each view owning a search.
  const searchFields = useCallback(
    (entry: OverviewEntry) => [
      entry.name,
      entry.environmentId,
      entry.capability?.os,
      entry.capability?.arch,
      t(`overview.status.${hostStatus(entry).key}`),
    ],
    [t],
  );

  const cardEntries = useMemo(() => {
    if (!query.trim()) return entries;
    return entries.filter((entry) => matchesSearchFields(query, searchMode, searchFields(entry)).matched);
  }, [entries, query, searchMode, searchFields]);

  const columns = useComputeOverviewColumns({ onNavigate, onSetup });

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  return (
    <TablePage
      id="compute-overview"
      title={t('overview.title')}
      description={t('overview.subtitle')}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('overview.searchPlaceholder'),
        fields: searchFields,
      }}
      actions={<ViewToggle viewMode={viewMode} setViewMode={setViewMode} />}
      toolbar={{
        actions: (
          <Button size="sm" permission={COMPUTE_PERM.MANAGE} onClick={() => onSetup('')}>
            <IconPlus className="h-4 w-4" /> {t('actions.runWizard')}
          </Button>
        ),
        columns: viewMode === 'table',
        density: viewMode === 'table',
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: { filename: 'mcharbor-compute-hosts' },
      }}
      columns={columns}
      data={entries}
      loading={isLoading}
      empty={{ title: t('overview.empty.title'), description: t('overview.empty.body') }}
      getRowId={(entry) => entry.environmentId}
      body={
        viewMode === 'table' ? undefined : (
          <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
            <CardGrid
              count={cardEntries.length}
              isLoading={isLoading}
              filtered={entries.length > 0}
              emptyLabel={t('overview.empty.title')}
              minHeight="min-h-[350px]"
            >
              {cardEntries.map((entry) => (
                <HostCard key={entry.environmentId} entry={entry} onNavigate={onNavigate} onSetup={onSetup} />
              ))}
            </CardGrid>
          </div>
        )
      }
    />
  );
}
