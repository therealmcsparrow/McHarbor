// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import type { K8sServiceSummary } from '@core/types/kubernetes';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { useK8sServices } from '../hooks/useK8sServices';

export default function K8sServicesPage() {
  const { t } = useTranslation('kubernetes');
  const { data: services = [], isLoading } = useK8sServices();

  const columns = useMemo<ColumnDef<K8sServiceSummary, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('services.columns.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      { accessorKey: 'namespace', header: t('services.columns.namespace') },
      {
        accessorKey: 'type',
        header: t('services.columns.type'),
        cell: ({ row }) => <Badge variant="secondary">{row.original.type}</Badge>,
      },
      {
        accessorKey: 'clusterIP',
        header: t('services.columns.clusterIP'),
        cell: ({ row }) => (
          <span className="font-mono text-xs">{row.original.clusterIP}</span>
        ),
      },
      {
        accessorKey: 'ports',
        header: t('services.columns.ports'),
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.ports
              ?.map((p) => `${p.port}${p.nodePort ? `:${p.nodePort}` : ''}/${p.protocol}`)
              .join(', ') || '-'}
          </span>
        ),
      },
      { accessorKey: 'age', header: t('services.columns.age') },
    ],
    [t]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('services.title')}
        description={t('services.description', { count: services.length })}
      />

      <DataGrid
        data={services}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('services.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('services.emptyMessage')}
      />
    </div>
  );
}
