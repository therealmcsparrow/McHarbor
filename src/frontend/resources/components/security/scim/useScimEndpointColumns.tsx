// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudDownload, IconEdit, IconHistory, IconKey, IconRefresh, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatDate } from '@resources/utils/format';
import { describeCron } from '@resources/utils/schedule';
import { PERM } from '@core/auth/permissionKeys';
import type { Endpoint } from '@resources/hooks/useScimEndpoints';

export type EndpointRowActions = {
  onPreview: (endpoint: Endpoint) => void;
  onHistory: (endpoint: Endpoint) => void;
  onRotate: (endpoint: Endpoint) => void;
  onEdit: (endpoint: Endpoint) => void;
  onDelete: (endpoint: Endpoint) => void;
};

function LastSyncCell({ endpoint }: { endpoint: Endpoint }) {
  const { t } = useTranslation('scim');
  const statusValue = endpoint.lastSyncAt ? endpoint.lastSyncStatus : 'pending';
  const variant = statusValue === 'success' ? 'success' : statusValue === 'failed' ? 'destructive' : 'neutral';
  const label = statusValue === 'success' || statusValue === 'failed' ? statusValue : 'pending';
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1">
        <Badge variant={variant}>{t(`syncStatus.${label}`)}</Badge>
        <StatusError error={endpoint.lastSyncError} title={t('history.errorTitle')} />
      </div>
      {endpoint.lastSyncAt && <p className="text-xs text-muted-foreground">{formatDate(endpoint.lastSyncAt)}</p>}
    </div>
  );
}

function RowActions({ endpoint, actions }: { endpoint: Endpoint; actions: EndpointRowActions }) {
  const { t } = useTranslation('scim');
  return (
    <div className="flex justify-end gap-1">
      <Button
        permission={PERM.SCIM_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.preview')}
        onClick={() => actions.onPreview(endpoint)}
      >
        <IconRefresh className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.history')}
        onClick={() => actions.onHistory(endpoint)}
      >
        <IconHistory className="size-4" />
      </Button>
      {endpoint.authType === 'bearer' && (
        <Button
          permission={PERM.SCIM_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.rotate')}
          onClick={() => actions.onRotate(endpoint)}
        >
          <IconKey className="size-4" />
        </Button>
      )}
      <Button
        permission={PERM.SCIM_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.edit')}
        onClick={() => actions.onEdit(endpoint)}
      >
        <IconEdit className="size-4" />
      </Button>
      <Button
        permission={PERM.SCIM_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.delete')}
        onClick={() => actions.onDelete(endpoint)}
      >
        <IconTrash className="size-4" />
      </Button>
    </div>
  );
}

export function useScimEndpointColumns(actions: EndpointRowActions): ColumnDef<Endpoint, unknown>[] {
  const { t } = useTranslation('scim');
  const { t: tc } = useTranslation('common');
  return useMemo<ColumnDef<Endpoint, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (ep) => ep.name,
        header: t('table.name'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <IconCloudDownload className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate font-medium">{row.original.name}</span>
          </div>
        ),
      },
      {
        id: 'baseUrl',
        accessorFn: (ep) => ep.baseUrl,
        header: t('table.baseUrl'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="block max-w-[280px] truncate font-mono text-xs">{row.original.baseUrl}</span>
        ),
      },
      {
        id: 'schedule',
        accessorFn: (ep) => ep.pollSchedule,
        header: t('table.schedule'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="text-xs">
            {describeCron(row.original.pollSchedule, tc)}
            <span className="ml-2 font-mono opacity-70">{row.original.pollSchedule}</span>
          </span>
        ),
      },
      {
        id: 'lastSync',
        accessorFn: (ep) => ep.lastSyncAt ?? '',
        header: t('table.lastSync'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <LastSyncCell endpoint={row.original} />,
      },
      {
        id: 'counts',
        accessorFn: (ep) => ep.usersCount + ep.groupsCount,
        header: t('table.counts'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex gap-2 text-xs">
            <span>{t('counts.users', { count: row.original.usersCount })}</span>
            <span className="text-muted-foreground">·</span>
            <span>{t('counts.groups', { count: row.original.groupsCount })}</span>
          </div>
        ),
      },
      {
        id: 'status',
        accessorFn: (ep) => (ep.enabled ? t('status.enabled') : t('status.disabled')),
        header: t('table.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.enabled ? (
            <Badge variant="success">{t('status.enabled')}</Badge>
          ) : (
            <Badge variant="neutral">{t('status.disabled')}</Badge>
          ),
      },
      {
        id: 'actions',
        header: t('table.actions'),
        enableSorting: false,
        meta: { width: 'md', locked: true, noExport: true, label: t('table.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => <RowActions endpoint={row.original} actions={actions} />,
      },
    ],
    [actions, t, tc],
  );
}
