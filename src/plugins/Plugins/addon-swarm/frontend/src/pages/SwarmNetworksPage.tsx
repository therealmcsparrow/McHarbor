// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { Badge } from '@resources/components/ui/Badge';
import type { SearchMode } from '@resources/utils/search-filter';
import { SwarmGuard } from '../components/SwarmGuard';
import { useSwarmNetworks } from '../hooks/useSwarmStacks';
import { shortId } from '../format';
import type { SwarmOverlayNetwork } from '../types';

export default function SwarmNetworksPage() {
  return (
    <SwarmGuard>
      <SwarmNetworks />
    </SwarmGuard>
  );
}

/**
 * The swarm-scoped networks.
 *
 * Read-only on purpose: an overlay network is created by the stack that needs
 * it and removed with that stack, so a create button here would produce
 * networks nothing owns. The Docker networks page still manages local ones.
 */
function SwarmNetworks() {
  const { t } = useTranslation('swarm');
  const { data: networks = [], isLoading, isFetching, refetch } = useSwarmNetworks();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const columns = useMemo<ColumnDef<SwarmOverlayNetwork, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{row.original.name}</span>
            {row.original.ingress && (
              <Badge variant="secondary" className="shrink-0 px-1.5 py-0 text-[10px]">
                {t('networks.ingress')}
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'id',
        header: t('columns.id'),
        size: 120,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{shortId(row.original.id)}</span>,
      },
      {
        accessorKey: 'driver',
        header: t('columns.driver'),
        size: 100,
        cell: ({ row }) => (
          <Badge variant="default" className="px-1.5 py-0 text-[10px]">
            {row.original.driver}
          </Badge>
        ),
      },
      {
        id: 'subnets',
        header: t('columns.subnets'),
        size: 180,
        meta: { sortValue: (network: SwarmOverlayNetwork) => (network.subnets ?? []).join(', ') },
        cell: ({ row }) =>
          row.original.subnets && row.original.subnets.length > 0 ? (
            <span className="font-mono text-xs text-muted-foreground">{row.original.subnets.join(', ')}</span>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          ),
      },
      {
        accessorKey: 'stackName',
        header: t('columns.stack'),
        size: 140,
        cell: ({ row }) =>
          row.original.stackName ? (
            <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
              {row.original.stackName}
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          ),
      },
      {
        id: 'flags',
        header: t('columns.flags'),
        size: 150,
        meta: {
          sortValue: (network: SwarmOverlayNetwork) => (network.attachable ? 1 : 0) + (network.internal ? 2 : 0),
        },
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {row.original.attachable && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {t('networks.attachable')}
              </Badge>
            )}
            {row.original.internal && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {t('networks.internal')}
              </Badge>
            )}
          </div>
        ),
      },
    ],
    [t],
  );

  return (
    <TablePage
      id="swarm-networks"
      title={t('networks.title')}
      description={t('networks.description', { count: networks.length })}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('networks.searchPlaceholder'),
        fields: (n: SwarmOverlayNetwork) => [n.name, n.driver, n.stackName, ...(n.subnets ?? [])],
      }}
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: { filename: 'mcharbor-swarm-networks' },
      }}
      columns={columns}
      data={networks}
      loading={isLoading}
      empty={{ title: t('networks.empty') }}
      getRowId={(row) => row.id}
    />
  );
}
