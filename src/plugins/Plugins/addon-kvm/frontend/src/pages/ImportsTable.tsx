// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileImport, IconPlayerPlay, IconTrash, IconX } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { TablePage } from '@resources/layout/TablePage';
import { formatDate } from '@resources/utils/format';
import { formatBytes } from '../format';
import { VM_PERM } from '../permissions';
import { useCancelImport, useDeleteImport, useImports } from '../hooks/useImports';
import type { VMImport } from '../types';

type ImportsTableProps = {
  onConvert: (record: VMImport) => void;
};

function stateVariant(state: string): 'default' | 'secondary' | 'warning' | 'destructive' | 'success' {
  switch (state) {
    case 'failed':
      return 'destructive';
    case 'inspecting':
    case 'converting':
      return 'warning';
    case 'ready':
      return 'success';
    case 'inspected':
      return 'default';
    default:
      return 'secondary';
  }
}

function isRunning(state: string): boolean {
  return state === 'inspecting' || state === 'converting';
}

export function ImportsTable({ onConvert }: ImportsTableProps) {
  const { t } = useTranslation('vm');
  const { data, isPending, isFetching, refetch } = useImports();
  const remove = useDeleteImport();
  const cancel = useCancelImport();

  const rows = data?.imports ?? [];

  const columns = useMemo<ColumnDef<VMImport, unknown>[]>(
    () => [
      {
        id: 'fileName',
        header: t('import.columns.file'),
        accessorFn: (row) => row.fileName,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium text-foreground">{row.original.fileName}</p>
          </div>
        ),
      },
      {
        id: 'sourceKind',
        header: t('import.columns.format'),
        accessorFn: (row) => row.sourceKind,
        cell: ({ row }) => <span className="uppercase">{row.original.sourceKind || '—'}</span>,
      },
      {
        id: 'sizeBytes',
        header: t('import.columns.size'),
        accessorFn: (row) => row.sizeBytes,
        cell: ({ row }) => formatBytes(row.original.sizeBytes),
      },
      {
        id: 'state',
        header: t('import.columns.state'),
        accessorFn: (row) => row.state,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-2">
            {isRunning(row.original.state) ? (
              <span className="text-xs text-muted-foreground">{row.original.progress}%</span>
            ) : null}
            <Badge variant={stateVariant(row.original.state)}>{t(`import.state.${row.original.state}`)}</Badge>
            <StatusError error={row.original.lastError} />
          </div>
        ),
      },
      {
        id: 'createdAt',
        header: t('import.columns.uploaded'),
        accessorFn: (row) => row.createdAt,
        cell: ({ row }) => <div className="text-right">{formatDate(row.original.createdAt)}</div>,
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('import.columns.actions')}</span>,
        meta: { locked: true, noExport: true, label: t('import.columns.actions') },
        enableSorting: false,
        cell: ({ row }) => {
          const record = row.original;
          const running = isRunning(record.state);
          return (
            <div className="flex items-center justify-end gap-1">
              <Button
                size="sm"
                variant="outline"
                permission={VM_PERM.IMPORT}
                disabled={running || record.state === 'ready'}
                onClick={() => onConvert(record)}
              >
                <IconPlayerPlay className="mr-1 size-3.5" aria-hidden="true" />
                {t('import.actions.convert')}
              </Button>
              <Button
                size="sm"
                variant="outline"
                permission={VM_PERM.IMPORT}
                disabled={!running || cancel.isPending}
                onClick={() => cancel.mutate(record.id)}
              >
                <IconX className="mr-1 size-3.5" aria-hidden="true" />
                {t('import.actions.cancel')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                permission={VM_PERM.DELETE}
                aria-label={t('import.actions.delete')}
                disabled={running || remove.isPending}
                onClick={() => remove.mutate(record.id)}
              >
                <IconTrash className="size-3.5" aria-hidden="true" />
              </Button>
            </div>
          );
        },
      },
    ],
    [cancel, onConvert, remove, t],
  );

  return (
    <TablePage
      id="kvm-imports"
      title={t('import.tab')}
      description={t('import.tabDescription')}
      columns={columns}
      data={rows}
      loading={isPending}
      getRowId={(row) => row.id}
      empty={{
        icon: <IconFileImport className="h-5 w-5" />,
        title: t('import.emptyTitle'),
        description: t('import.emptyDescription'),
      }}
      toolbar={{
        columns: true,
        density: true,
        refresh: { onRefresh: () => void refetch(), isFetching },
        export: { filename: 'vm-imports' },
      }}
      pagination={{ mode: 'client' }}
    />
  );
}
