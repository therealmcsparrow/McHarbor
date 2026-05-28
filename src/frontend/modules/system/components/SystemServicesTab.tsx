// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { Badge } from '@resources/components/ui/Badge';
import { DataGrid } from '@resources/components/DataGrid';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import type { HostMetrics } from '@core/types/docker';
import packageJson from '../../../package.json';
import type { SystemInfo } from '../types';

type ServiceRow = {
  name: string;
  status: 'available' | 'degraded' | 'unavailable';
  detail: string;
  source: string;
};

function statusVariant(status: ServiceRow['status']) {
  if (status === 'available') return 'success';
  if (status === 'degraded') return 'warning';
  return 'destructive';
}

export function SystemServicesTab({
  info,
  hostMetrics,
  containerMetrics,
}: {
  info: SystemInfo;
  hostMetrics?: HostMetrics;
  containerMetrics: BulkContainerMetric[];
}) {
  const { t } = useTranslation('system');
  const services = useMemo<ServiceRow[]>(
    () => [
      {
        name: t('services.api'),
        status: 'available',
        detail: `v${info.version}`,
        source: t('services.sources.backend'),
      },
      {
        name: t('services.webUi'),
        status: 'available',
        detail: `v${packageJson.version}`,
        source: t('services.sources.frontend'),
      },
      {
        name: t('services.dockerHost'),
        status: hostMetrics ? 'available' : 'unavailable',
        detail: hostMetrics?.host.serverVersion ?? t('services.noData'),
        source: hostMetrics?.host.hostname ?? t('services.sources.environment'),
      },
      {
        name: t('services.metrics'),
        status: containerMetrics.length > 0 ? 'available' : 'degraded',
        detail: t('services.containerSamples', { count: containerMetrics.length }),
        source: t('services.sources.metrics'),
      },
      {
        name: t('services.database'),
        status: 'available',
        detail: 'SQLite',
        source: t('services.sources.backend'),
      },
    ],
    [containerMetrics.length, hostMetrics, info.version, t]
  );

  const columns = useMemo<ColumnDef<ServiceRow, unknown>[]>(
    () => [
      { accessorKey: 'name', header: t('services.columns.name') },
      {
        accessorKey: 'status',
        header: t('services.columns.status'),
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>
            {t(`services.status.${row.original.status}`)}
          </Badge>
        ),
      },
      { accessorKey: 'detail', header: t('services.columns.detail') },
      { accessorKey: 'source', header: t('services.columns.source') },
    ],
    [t]
  );

  return (
    <DataGrid
      data={services}
      columns={columns}
      searchPlaceholder={t('services.searchPlaceholder')}
      emptyMessage={t('services.empty')}
      pageSize={10}
    />
  );
}
