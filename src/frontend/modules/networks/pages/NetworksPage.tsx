// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLayoutGrid, IconLayoutList, IconPlus } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { getNetworkContainerCount, isBuiltInNetwork } from '@core/utils/network';
import { DataGrid } from '@resources/components/DataGrid';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Button } from '@resources/components/ui/Button';
import { PageHeader } from '@resources/layout/PageHeader';
import { BulkRemoveNetworkDialog } from '../components/BulkRemoveNetworkDialog';
import { NetworkCardGrid } from '../components/NetworkCardGrid';
import { NetworkCreateDialog } from '../components/NetworkCreateDialog';
import { useNetworks } from '../hooks/useNetworks';
import { useNetworksPageConfig } from '../hooks/useNetworksPageConfig';
import { useRemoveNetwork } from '../hooks/useNetworks';
import { useNetworksViewStore } from '../stores/networks-view';

type UsageFilter = 'all' | 'used' | 'unused';

export default function NetworksPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const { data: networks = [], isLoading } = useNetworks();
  const removeNetwork = useRemoveNetwork();
  const { viewMode, setViewMode } = useNetworksViewStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [bulkRemoveNetworks, setBulkRemoveNetworks] = useState<NetworkInfo[]>([]);
  const [usageFilter, setUsageFilter] = useState<UsageFilter>('all');
  const { columns, batchActions } = useNetworksPageConfig(
    setConfirmTarget,
    (rows) => setBulkRemoveNetworks(rows.filter((n) => !isBuiltInNetwork(n))),
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

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        title={t('title')}
        description={t('description', { count: networks.length })}
        actions={
          <>
            <Button onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('create.title')}
            </Button>
            <div className="h-6 w-px bg-border" />
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
          </>
        }
      />

      <div className="flex items-center gap-1 self-start rounded-lg border border-border p-0.5">
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

      <div className="flex min-h-0 flex-1 flex-col">
        {viewMode === 'table' ? (
          <DataGrid
            data={filteredNetworks}
            columns={columns}
            searchKey="Name"
            searchPlaceholder={t('searchPlaceholder')}
            loading={isLoading}
            emptyMessage={t('emptyMessage')}
            onRowClick={(row) => navigate(`/networks/${row.Id}`)}
            tableFixed
            selectable
            batchActions={batchActions}
            getRowId={(row) => row.Id}
            fillHeight
            isRowDisabled={(row) => isBuiltInNetwork(row)}
            getRowClassName={(row) =>
              isBuiltInNetwork(row) ? 'opacity-70' : undefined
            }
          />
        ) : (
          <div className="min-h-0 flex-1 overflow-auto">
            <NetworkCardGrid
              networks={filteredNetworks}
              isLoading={isLoading}
              onClick={(network) => navigate(`/networks/${network.Id}`)}
              onRemove={(id) => {
                const target = networks.find((n) => n.Id === id);
                if (target && isBuiltInNetwork(target)) return;
                setConfirmTarget(id);
              }}
            />
          </div>
        )}
      </div>

      <NetworkCreateDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) {
            removeNetwork.mutate(confirmTarget);
          }
          setConfirmTarget(null);
        }}
        loading={removeNetwork.isPending}
      />

      <BulkRemoveNetworkDialog
        networks={bulkRemoveNetworks}
        open={bulkRemoveNetworks.length > 0}
        onOpenChange={(open) => !open && setBulkRemoveNetworks([])}
      />
    </div>
  );
}

type FilterButtonProps = {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
  tone?: 'success' | 'default';
};

function FilterButton({ active, onClick, label, count, tone = 'default' }: FilterButtonProps) {
  const activeTone =
    tone === 'success' && active
      ? 'bg-teal-500/20 text-teal-700 dark:text-teal-300'
      : active
        ? 'bg-primary/15 text-primary'
        : 'text-muted-foreground hover:text-foreground';
  return (
    <Button
      variant={active ? 'default' : 'ghost'}
      size="sm"
      onClick={onClick}
      className={`gap-1.5 ${activeTone}`}
    >
      <span>{label}</span>
      <span className="rounded-full bg-background/60 px-1.5 text-[10px] tabular-nums text-muted-foreground">
        {count}
      </span>
    </Button>
  );
}
