// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconTrash, IconRefresh } from '@tabler/icons-react';
import { StatusBadge, ENVIRONMENT_STATUS } from '@resources/components/ui/StatusBadge';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { deriveEnvironmentStatus } from '../hooks/useEnvironmentActions';

interface UseEnvironmentColumnsOptions {
  onTest: (id: string) => void;
  onRemove: (id: string) => void;
}

export function useEnvironmentColumns({ onTest, onRemove }: UseEnvironmentColumnsOptions) {
  const { t } = useTranslation('environments');
  const { t: tc } = useTranslation('common');

  const columns: ColumnDef<EnvironmentListItem, unknown>[] = [
    {
      accessorKey: 'name',
      header: t('columns.name'),
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <Link
            to={`/environments/${row.original.id}`}
            className="font-medium text-foreground hover:text-primary hover:underline"
          >
            {row.original.name}
          </Link>
          {row.original.isDefault && <Badge variant="secondary">{t('badges.default')}</Badge>}
        </div>
      ),
    },
    {
      accessorKey: 'orchestratorType',
      header: t('columns.platform'),
      cell: ({ row }) => (
        <Badge variant={row.original.orchestratorType === 'kubernetes' ? 'default' : 'secondary'}>
          {row.original.orchestratorType === 'kubernetes' ? t('platform.kubernetes') : t('platform.docker')}
        </Badge>
      ),
    },
    {
      accessorKey: 'connectionType',
      header: t('columns.connection'),
      cell: ({ row }) => (
        <span className="text-sm uppercase">{row.original.connectionType || '-'}</span>
      ),
    },
    {
      id: 'endpoint',
      header: t('columns.endpoint'),
      cell: ({ row }) => {
        if (row.original.connectionType === 'agent') {
          return (
            <span className="font-mono text-xs text-muted-foreground">
              {row.original.agentHostname ?? t('waitingForAgent')}
            </span>
          );
        }
        return (
          <span className="font-mono text-xs text-muted-foreground">
            {row.original.socketPath ?? (row.original.host ? `${row.original.host}:${row.original.port ?? ''}` : '-')}
          </span>
        );
      },
    },
    {
      id: 'version',
      header: t('columns.version'),
      cell: ({ row }) => row.original.dockerVersion ?? row.original.k8sVersion ?? '-',
    },
    {
      id: 'status',
      header: t('columns.status'),
      cell: ({ row }) => {
        const status = deriveEnvironmentStatus(row.original);
        return <StatusBadge status={status} map={ENVIRONMENT_STATUS} />;
      },
    },
    {
      id: 'actions',
      header: () => <span className="ml-auto">{t('columns.actions')}</span>,
      cell: ({ row }) => (
        <div className="flex items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('testConnection')}
            title={t('testConnection')}
            onClick={(event) => {
              event.stopPropagation();
              onTest(row.original.id);
            }}
          >
            <IconRefresh className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={tc('actions.remove')}
            title={tc('actions.remove')}
            onClick={(event) => {
              event.stopPropagation();
              onRemove(row.original.id);
            }}
          >
            <IconTrash className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      ),
    },
  ];

  return columns;
}
