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
import { usePools, useRefreshPool } from '../hooks/useCompute';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import { useComputeView } from '../hooks/useComputeView';
import { CardGrid } from '../components/CardGrid';
import { PoolCard } from '../components/PoolCard';
import { usePoolColumns } from '../components/PoolColumns';
import { ViewToggle } from '../components/ViewToggle';
import { CreatePoolDialog } from '../components/CreatePoolDialog';
import { DeletePoolDialog } from '../components/DeletePoolDialog';
import { MovePoolDialog } from '../components/MovePoolDialog';
import type { Pool } from '../types';

/** Storage pools across every host. */
type PoolsPageProps = {
  environments: { id: string; name: string }[];
  onOpenPool: (poolId: string) => void;
  createRequested?: boolean;
  onCreateRequestDone?: () => void;
};

export function PoolsPage({ environments, onOpenPool, createRequested = false, onCreateRequestDone }: PoolsPageProps) {
  const { t } = useTranslation('compute');
  // Scoped to the host's topbar selection, like every other page in this
  // plugin. An empty selection means "no host chosen yet" and shows all.
  const environmentId = useHostEnvironment();
  const { data, isLoading, isFetching, isError, refetch } = usePools(environmentId || undefined);
  const refresh = useRefreshPool();
  const { viewMode, setViewMode } = useComputeView('pools');

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<Pool | null>(null);
  const [moving, setMoving] = useState<Pool | null>(null);

  const hostName = useCallback(
    (envId: string) => environments.find((env) => env.id === envId)?.name ?? envId,
    [environments],
  );

  const pools = useMemo(() => data ?? [], [data]);

  const searchFields = useCallback(
    (pool: Pool) => [pool.name, hostName(pool.environmentId), pool.target, t(`pools.backend.${pool.backend}`)],
    [hostName, t],
  );

  const cardPools = useMemo(() => {
    if (!query.trim()) return pools;
    return pools.filter((pool) => matchesSearchFields(query, searchMode, searchFields(pool)).matched);
  }, [pools, query, searchMode, searchFields]);

  const columns = usePoolColumns({
    hostName,
    refreshing: refresh.isPending,
    onRefresh: (id) => refresh.mutate(id),
    onDelete: setDeleting,
    onMove: setMoving,
  });

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  return (
    <>
      <TablePage
        id="compute-pools"
        title={t('pools.title')}
        description={t('pools.subtitle')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('pools.searchPlaceholder'),
          fields: searchFields,
        }}
        actions={<ViewToggle viewMode={viewMode} setViewMode={setViewMode} />}
        toolbar={{
          actions: (
            <Button size="sm" permission={COMPUTE_PERM.MANAGE} onClick={() => setCreating(true)}>
              <IconPlus className="h-4 w-4" /> {t('actions.createPool')}
            </Button>
          ),
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-compute-pools' },
        }}
        columns={columns}
        data={pools}
        loading={isLoading}
        empty={{ title: t('pools.empty.title'), description: t('pools.empty.body') }}
        getRowId={(pool) => pool.id}
        onRowClick={(pool) => onOpenPool(pool.id)}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <CardGrid
                count={cardPools.length}
                isLoading={isLoading}
                filtered={pools.length > 0}
                emptyLabel={t('pools.empty.title')}
              >
                {cardPools.map((pool) => (
                  <PoolCard
                    key={pool.id}
                    pool={pool}
                    hostName={hostName(pool.environmentId)}
                    refreshing={refresh.isPending}
                    onRefresh={(id) => refresh.mutate(id)}
                    onDelete={setDeleting}
                    onMove={setMoving}
                    onOpen={() => onOpenPool(pool.id)}
                  />
                ))}
              </CardGrid>
            </div>
          )
        }
      />

      {(creating || createRequested) && (
        <CreatePoolDialog
          environments={environments}
          onClose={() => {
            setCreating(false);
            onCreateRequestDone?.();
          }}
        />
      )}
      {deleting && <DeletePoolDialog pool={deleting} onClose={() => setDeleting(null)} />}
      {moving && <MovePoolDialog pool={moving} onClose={() => setMoving(null)} />}
    </>
  );
}
