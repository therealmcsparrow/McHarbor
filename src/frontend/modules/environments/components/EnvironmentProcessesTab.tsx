// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { api } from '@core/api/client';
import type { ContainerMetric } from '@core/types/docker';
import { formatBytes, truncateId } from '@resources/utils/format';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import { bytesColumn, textColumn } from '@resources/layout/table-page/optional-columns';

type EnvironmentProcessesTabProps = {
  envId: string;
};

function useEnvironmentContainerProcesses(envId: string) {
  const uploadActive = useEnvironmentUploadActive(envId);
  const { collectContainerMetricsEnabled } = useCurrentEnvironmentActivitySettings();
  return useQuery({
    queryKey: ['containers-bulk-stats', envId],
    queryFn: () =>
      api
        .get<ContainerMetric[]>('/containers/stats/summary', envId ? { env: envId } : {})
        .then(assertSuccess)
        .then((metrics) => metrics ?? []),
    refetchInterval: collectContainerMetricsEnabled && !uploadActive ? 5_000 : false,
    enabled: !!envId && collectContainerMetricsEnabled && !uploadActive,
  });
}

export function EnvironmentProcessesTab({ envId }: EnvironmentProcessesTabProps) {
  const { t } = useTranslation('environments');
  const { data: processes, isLoading, isError, error, refetch } = useEnvironmentContainerProcesses(envId);
  const group = t('detail.processes.columns.groupContainer');

  const columns = useMemo<ColumnDef<ContainerMetric, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('detail.processes.columns.name'),
        cell: ({ row }) => (
          <div>
            <p className="font-medium text-foreground">{row.original.name || '-'}</p>
            <p className="font-mono text-xs text-muted-foreground">{truncateId(row.original.id)}</p>
          </div>
        ),
      },
      {
        accessorKey: 'cpuPercent',
        header: t('detail.processes.columns.cpu'),
        cell: ({ row }) => `${row.original.cpuPercent.toFixed(1)}%`,
      },
      {
        accessorKey: 'memUsage',
        header: t('detail.processes.columns.memory'),
        cell: ({ row }) => (
          <span>
            {formatBytes(row.original.memUsage)}
            <span className="ml-1 text-muted-foreground">({row.original.memPercent.toFixed(1)}%)</span>
          </span>
        ),
      },
      {
        accessorKey: 'pids',
        header: t('detail.processes.columns.pids'),
      },
      {
        id: 'network',
        header: t('detail.processes.columns.network'),
        meta: { sortValue: (r: ContainerMetric) => r.netRx + r.netTx },
        cell: ({ row }) => `${formatBytes(row.original.netRx)} / ${formatBytes(row.original.netTx)}`,
      },
      {
        id: 'block',
        header: t('detail.processes.columns.block'),
        meta: { sortValue: (r: ContainerMetric) => r.blockRead + r.blockWrite },
        cell: ({ row }) => `${formatBytes(row.original.blockRead)} / ${formatBytes(row.original.blockWrite)}`,
      },
      bytesColumn<ContainerMetric>('memLimit', t('detail.processes.columns.memLimit'), (r) => r.memLimit, { group }),
      textColumn<ContainerMetric>('containerId', t('detail.processes.columns.containerId'), (r) => r.id.slice(0, 12), {
        group,
      }),
    ],
    [t, group],
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.processes.title')}</CardTitle>
        <CardDescription>{t('detail.processes.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="flex h-64 items-center justify-center">
            <Spinner />
          </div>
        ) : isError ? (
          <ErrorState variant="card" error={error} onRetry={() => void refetch()} />
        ) : (
          <TablePage
            id="environment-processes"
            // Sits inside a Card that already carries the heading.
            header={false}
            title={t('detail.processes.title')}
            columns={columns}
            data={processes ?? []}
            empty={{ title: t('detail.processes.empty') }}
            // The query behind this tab polls every 5s on its own; a refresh
            // control here would be a second, competing mechanism.
            toolbar={{ columns: true, density: true }}
            pagination={{ mode: 'client' }}
          />
        )}
      </CardContent>
    </Card>
  );
}
