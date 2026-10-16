// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconDownload, IconTrash, IconRefresh } from '@tabler/icons-react';
import { StatusBadge, ENVIRONMENT_STATUS } from '@resources/components/ui/StatusBadge';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { deriveEnvironmentStatus } from '../hooks/useEnvironmentActions';
import { ipv4SortKey } from '@resources/utils/ip-sort';
import { PERM } from '@core/auth/permissionKeys';
import { AgentTlsShield } from './AgentTlsShield';
import { useEnvironmentOptionalColumns } from './useEnvironmentOptionalColumns';

function environmentEndpoint(e: EnvironmentListItem): string {
  if (e.connectionType === 'agent') return e.agentHostname ?? '';
  return e.socketPath ?? (e.host ? `${e.host}:${e.port ?? ''}` : '');
}

function environmentVersion(e: EnvironmentListItem): string {
  return (e.connectionType === 'agent' ? e.agentVersion : e.dockerVersion) ?? '';
}

interface UseEnvironmentColumnsOptions {
  onTest: (id: string) => void;
  onInstall: (id: string) => void;
  onRemove: (id: string) => void;
}

export function useEnvironmentColumns({ onTest, onInstall, onRemove }: UseEnvironmentColumnsOptions) {
  const { t } = useTranslation('environments');
  const optionalColumns = useEnvironmentOptionalColumns();

  const columns = useMemo<ColumnDef<EnvironmentListItem, unknown>[]>(
    () => [
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
        accessorKey: 'connectionType',
        header: t('columns.connection'),
        cell: ({ row }) => (
          <div className="flex items-center gap-1.5">
            <span className="text-sm uppercase">{row.original.connectionType || '-'}</span>
            <AgentTlsShield mode={row.original.agentTlsMode} />
          </div>
        ),
      },
      {
        id: 'endpoint',
        header: t('columns.endpoint'),
        meta: { sortValue: environmentEndpoint },
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
        id: 'hostIp',
        header: t('columns.hostIp'),
        meta: { sortValue: (e: EnvironmentListItem) => ipv4SortKey(e.agentHostIp) },
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.agentHostIp ?? '-'}</span>
        ),
      },
      {
        id: 'version',
        header: t('columns.version'),
        meta: { wrap: true, sortValue: environmentVersion },
        cell: ({ row }) => {
          if (row.original.connectionType === 'agent') {
            return (
              <div className="min-w-0">
                <p className="font-mono text-xs font-medium text-foreground">{row.original.agentVersion ?? '-'}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {row.original.dockerVersion
                    ? `${t('versions.docker')} ${row.original.dockerVersion}`
                    : t('card.versionUnknown')}
                </p>
              </div>
            );
          }
          return row.original.dockerVersion ?? '-';
        },
      },
      {
        id: 'status',
        header: t('columns.status'),
        meta: { sortValue: deriveEnvironmentStatus },
        cell: ({ row }) => {
          const envStatus = deriveEnvironmentStatus(row.original);
          return <StatusBadge status={envStatus} map={ENVIRONMENT_STATUS} />;
        },
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              permission={PERM.ENVIRONMENTS_MANAGE}
              variant="ghost"
              size="icon"
              aria-label={t('testConnection')}
              title={t('testConnection')}
              onClick={(e) => {
                e.stopPropagation();
                onTest(row.original.id);
              }}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
            {row.original.connectionType === 'agent' && (
              <Button
                permission={PERM.ENVIRONMENTS_MANAGE}
                variant="ghost"
                size="icon"
                aria-label={t('installAgent')}
                title={t('installAgent')}
                onClick={(e) => {
                  e.stopPropagation();
                  onInstall(row.original.id);
                }}
              >
                <IconDownload className="h-4 w-4" />
              </Button>
            )}
            <Button
              permission={PERM.ENVIRONMENTS_MANAGE}
              variant="ghost"
              size="icon"
              aria-label={t('actions.remove', { ns: 'common' })}
              title={t('actions.remove', { ns: 'common' })}
              onClick={(e) => {
                e.stopPropagation();
                onRemove(row.original.id);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [t, optionalColumns, onTest, onInstall, onRemove],
  );

  return columns;
}
