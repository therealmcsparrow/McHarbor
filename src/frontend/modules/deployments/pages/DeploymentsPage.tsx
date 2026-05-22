// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import type { DeploymentSummary } from '@core/types/kubernetes';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { useDeployments } from '../hooks/useDeployments';

export default function DeploymentsPage() {
  const { t } = useTranslation('kubernetes');
  const { data: deployments = [], isLoading } = useDeployments();
  const navigate = useNavigate();

  const columns = useMemo<ColumnDef<DeploymentSummary, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('deployments.columns.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      { accessorKey: 'namespace', header: t('deployments.columns.namespace') },
      { accessorKey: 'ready', header: t('deployments.columns.ready') },
      { accessorKey: 'upToDate', header: t('deployments.columns.upToDate') },
      { accessorKey: 'available', header: t('deployments.columns.available') },
      {
        accessorKey: 'images',
        header: t('deployments.columns.images'),
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.images?.join(', ') || '-'}
          </span>
        ),
      },
      { accessorKey: 'age', header: t('deployments.columns.age') },
    ],
    [t]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('deployments.title')}
        description={t('deployments.description', { count: deployments.length })}
      />

      <DataGrid
        data={deployments}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('deployments.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('deployments.emptyMessage')}
        onRowClick={(row) => navigate(`/deployments/${row.namespace}/${row.name}`)}
      />
    </div>
  );
}
