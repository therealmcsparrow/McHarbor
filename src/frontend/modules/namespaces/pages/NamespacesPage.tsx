// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import type { NamespaceSummary } from '@core/types/kubernetes';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { useNamespaces } from '../hooks/useNamespaces';

export default function NamespacesPage() {
  const { t } = useTranslation('kubernetes');
  const { data: namespaces = [], isLoading } = useNamespaces();

  const columns = useMemo<ColumnDef<NamespaceSummary, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('namespaces.columns.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      {
        accessorKey: 'status',
        header: t('namespaces.columns.status'),
        cell: ({ row }) => (
          <Badge variant={row.original.status === 'Active' ? 'success' : 'secondary'}>
            {row.original.status}
          </Badge>
        ),
      },
      { accessorKey: 'age', header: t('namespaces.columns.age') },
    ],
    [t]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('namespaces.title')}
        description={t('namespaces.description', { count: namespaces.length })}
      />

      <DataGrid
        data={namespaces}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('namespaces.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('namespaces.emptyMessage')}
      />
    </div>
  );
}
