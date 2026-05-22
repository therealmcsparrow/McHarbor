// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import type { PodSummary } from '@core/types/kubernetes';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { usePods } from '../hooks/usePods';

function statusVariant(status: string) {
  switch (status) {
    case 'Running':
      return 'success' as const;
    case 'Pending':
      return 'warning' as const;
    case 'Failed':
      return 'destructive' as const;
    case 'Succeeded':
      return 'secondary' as const;
    default:
      return 'outline' as const;
  }
}

export default function PodsPage() {
  const { t } = useTranslation('kubernetes');
  const { data: pods = [], isLoading } = usePods();
  const navigate = useNavigate();

  const columns = useMemo<ColumnDef<PodSummary, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('pods.columns.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      { accessorKey: 'namespace', header: t('pods.columns.namespace') },
      {
        accessorKey: 'status',
        header: t('pods.columns.status'),
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>{row.original.status}</Badge>
        ),
      },
      { accessorKey: 'ready', header: t('pods.columns.ready') },
      { accessorKey: 'restarts', header: t('pods.columns.restarts') },
      { accessorKey: 'node', header: t('pods.columns.node') },
      { accessorKey: 'age', header: t('pods.columns.age') },
    ],
    [t]
  );

  return (
    <div className="space-y-6">
      <PageHeader title={t('pods.title')} description={t('pods.description', { count: pods.length })} />

      <DataGrid
        data={pods}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('pods.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('pods.emptyMessage')}
        onRowClick={(row) => navigate(`/pods/${row.namespace}/${row.name}`)}
      />
    </div>
  );
}
