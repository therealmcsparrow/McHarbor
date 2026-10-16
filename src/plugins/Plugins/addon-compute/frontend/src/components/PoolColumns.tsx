// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconFolderSymlink, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { COMPUTE_PERM } from '../permissions';
import { PoolCapacityBar, PoolSpeedBadge } from './PoolCapacityBar';
import { usedPercent } from '../format';
import type { Pool } from '../types';

interface UsePoolColumnsOptions {
  hostName: (environmentId: string) => string;
  refreshing: boolean;
  onRefresh: (id: string) => void;
  onDelete: (pool: Pool) => void;
  onMove: (pool: Pool) => void;
}

/** The pools table's columns. */
export function usePoolColumns({ hostName, refreshing, onRefresh, onDelete, onMove }: UsePoolColumnsOptions) {
  const { t } = useTranslation('compute');

  return useMemo<ColumnDef<Pool, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('pools.columns.name'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="font-medium text-foreground">{row.original.name}</span>
            {row.original.isDefault && <Badge variant="secondary">{t('badges.default')}</Badge>}
          </div>
        ),
      },
      {
        id: 'host',
        header: t('pools.columns.host'),
        accessorFn: (pool) => hostName(pool.environmentId),
        cell: ({ row }) => <span className="text-muted-foreground">{hostName(row.original.environmentId)}</span>,
      },
      {
        accessorKey: 'backend',
        header: t('pools.columns.backend'),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1.5">
            <span>{t(`pools.backend.${row.original.backend}`)}</span>
            <PoolSpeedBadge pool={row.original} />
          </div>
        ),
      },
      {
        accessorKey: 'target',
        header: t('pools.columns.target'),
        cell: ({ row }) => <code className="font-mono text-xs text-muted-foreground">{row.original.target}</code>,
      },
      {
        id: 'capacity',
        header: t('pools.columns.capacity'),
        meta: { sortValue: (pool: Pool) => usedPercent(pool.usedBytes, pool.capacityBytes) },
        cell: ({ row }) => (
          <div className="min-w-[12rem]">
            <PoolCapacityBar pool={row.original} />
          </div>
        ),
      },
      {
        accessorKey: 'volumeCount',
        header: t('pools.columns.volumes'),
        cell: ({ row }) => <span className="text-muted-foreground tabular-nums">{row.original.volumeCount}</span>,
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.refresh')}
              title={t('actions.refresh')}
              permission={COMPUTE_PERM.VIEW}
              disabled={refreshing}
              onClick={() => onRefresh(row.original.id)}
            >
              <IconRefresh className="h-4 w-4" aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('pools.move.action')}
              title={t('pools.move.action')}
              permission={COMPUTE_PERM.MANAGE}
              disabled={row.original.backend !== 'dir'}
              onClick={() => onMove(row.original)}
            >
              <IconFolderSymlink className="h-4 w-4" aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.delete')}
              title={t('actions.delete')}
              permission={COMPUTE_PERM.DELETE}
              onClick={() => onDelete(row.original)}
            >
              <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
            </Button>
          </div>
        ),
      },
    ],
    [t, hostName, refreshing, onRefresh, onDelete, onMove],
  );
}
