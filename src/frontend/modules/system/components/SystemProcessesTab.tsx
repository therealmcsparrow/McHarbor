// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { DataGrid } from '@resources/components/DataGrid';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import { formatBytes, truncateId } from '@resources/utils/format';

export function SystemProcessesTab({
  processes,
  isLoading,
}: {
  processes: BulkContainerMetric[];
  isLoading: boolean;
}) {
  const { t } = useTranslation('system');
  const columns = useMemo<ColumnDef<BulkContainerMetric, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('processes.columns.name'),
        cell: ({ row }) => (
          <div>
            <p className="font-medium text-foreground">{row.original.name || '-'}</p>
            <p className="font-mono text-xs text-muted-foreground">
              {truncateId(row.original.id)}
            </p>
          </div>
        ),
      },
      {
        accessorKey: 'cpuPercent',
        header: t('processes.columns.cpu'),
        cell: ({ row }) => `${row.original.cpuPercent.toFixed(1)}%`,
      },
      {
        accessorKey: 'memUsage',
        header: t('processes.columns.memory'),
        cell: ({ row }) => (
          <span>
            {formatBytes(row.original.memUsage)}
            <span className="ml-1 text-muted-foreground">
              ({row.original.memPercent.toFixed(1)}%)
            </span>
          </span>
        ),
      },
      {
        accessorKey: 'pids',
        header: t('processes.columns.pids'),
      },
      {
        id: 'network',
        header: t('processes.columns.network'),
        cell: ({ row }) =>
          `${formatBytes(row.original.netRx)} / ${formatBytes(row.original.netTx)}`,
      },
      {
        id: 'block',
        header: t('processes.columns.block'),
        cell: ({ row }) =>
          `${formatBytes(row.original.blockRead)} / ${formatBytes(row.original.blockWrite)}`,
      },
    ],
    [t]
  );

  return (
    <DataGrid
      data={processes}
      columns={columns}
      searchPlaceholder={t('processes.searchPlaceholder')}
      emptyMessage={t('processes.empty')}
      loading={isLoading}
      pageSize={10}
      tableFixed
    />
  );
}
