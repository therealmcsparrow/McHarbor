// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { StatusError } from '@resources/components/ui/StatusError';
import type { LifecycleAction } from '../hooks/useContainers';
import { IsolationBadge, StateBadge } from './StateBadge';
import { formatBytes, formatRelative } from '../format';
import { ObservedAddress, RowActions } from './ContainerRowActions';
import { isLive, type Container } from '../types';

interface UseContainerColumnsOptions {
  hostName: (environmentId: string) => string;
  busy: boolean;
  onOpen: (id: string) => void;
  onAction: (id: string, action: LifecycleAction) => void;
}

export function useContainerColumns({ hostName, busy, onOpen, onAction }: UseContainerColumnsOptions) {
  const { t } = useTranslation('ct');

  return useMemo<ColumnDef<Container, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('list.columns.name'),
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <PlainButton
              type="button"
              onClick={() => onOpen(row.original.id)}
              className="truncate text-left font-medium text-foreground hover:underline"
            >
              {row.original.name}
            </PlainButton>
            {row.original.isTemplate && <Badge variant="secondary">{t('nav.templates')}</Badge>}
          </div>
        ),
      },
      {
        accessorKey: 'state',
        header: t('list.columns.state'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <StateBadge container={row.original} />
            <StatusError error={row.original.lastError} />
          </div>
        ),
      },
      {
        id: 'host',
        header: t('list.columns.host'),
        accessorFn: (container) => hostName(container.environmentId),
        cell: ({ row }) => <span className="text-muted-foreground">{hostName(row.original.environmentId)}</span>,
      },
      {
        id: 'isolation',
        header: t('list.columns.isolation'),
        meta: { sortValue: (container: Container) => Boolean(container.privileged) },
        cell: ({ row }) => <IsolationBadge container={row.original} />,
      },
      {
        id: 'memory',
        header: t('list.columns.memory'),
        accessorFn: (container) => container.limits.memoryMaxBytes ?? 0,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {row.original.limits.memoryMaxBytes
              ? formatBytes(row.original.limits.memoryMaxBytes)
              : t('resources.usage.noLimit')}
          </span>
        ),
      },
      {
        id: 'address',
        header: t('list.columns.address'),
        accessorFn: (container) => container.observedIps?.[0] ?? '',
        cell: ({ row }) => <ObservedAddress container={row.original} />,
      },
      {
        id: 'uptime',
        header: t('list.columns.uptime'),
        accessorFn: (container) => container.lastStartedAt ?? '',
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {isLive(row.original) && row.original.lastStartedAt ? formatRelative(row.original.lastStartedAt) : '—'}
          </span>
        ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <RowActions
            container={row.original}
            busy={busy}
            onOpen={onOpen}
            onAction={(action) => onAction(row.original.id, action)}
          />
        ),
      },
    ],
    [t, hostName, busy, onOpen, onAction],
  );
}
