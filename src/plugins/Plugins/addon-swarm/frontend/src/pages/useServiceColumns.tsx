// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconArrowBackUp, IconRefresh, IconResize, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusBadge } from '../components/StatusBadge';
import { portsText, replicaText, serviceTone, updateStateTone } from '../format';
import type { SwarmService } from '../types';

type Handlers = {
  restart: { mutate: (id: string) => void };
  rollback: { mutate: (id: string) => void };
  onScale: (service: SwarmService) => void;
  onRemove: (service: SwarmService) => void;
};

export function useServiceColumns({ restart, rollback, onScale, onRemove }: Handlers) {
  const { t } = useTranslation('swarm');

  const columns = useMemo<ColumnDef<SwarmService, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{row.original.name}</span>
            {row.original.stackName && (
              <Badge variant="outline" className="shrink-0 px-1.5 py-0 text-[10px]">
                {row.original.stackName}
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'mode',
        header: t('columns.mode'),
        size: 110,
        cell: ({ row }) => (
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
            {t(`serviceMode.${row.original.mode}`)}
          </Badge>
        ),
      },
      {
        id: 'replicas',
        header: t('columns.replicas'),
        size: 100,
        meta: { sortValue: (service: SwarmService) => service.runningTasks },
        cell: ({ row }) => <StatusBadge tone={serviceTone(row.original)}>{replicaText(row.original)}</StatusBadge>,
      },
      {
        accessorKey: 'image',
        header: t('columns.image'),
        meta: { flex: true },
        cell: ({ row }) => (
          <span className="truncate font-mono text-xs text-muted-foreground">{row.original.image.split('@')[0]}</span>
        ),
      },
      {
        id: 'ports',
        header: t('columns.ports'),
        size: 150,
        meta: {
          sortValue: (service: SwarmService) => {
            const published = (service.ports ?? []).map((p) => p.publishedPort).filter((n) => n > 0);
            return published.length ? Math.min(...published) : null;
          },
        },
        cell: ({ row }) => {
          const text = portsText(row.original);
          return text ? (
            <span className="font-mono text-xs text-muted-foreground">{text}</span>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          );
        },
      },
      {
        id: 'update',
        header: t('columns.rollout'),
        size: 120,
        meta: { sortValue: (service: SwarmService) => service.updateState || null },
        cell: ({ row }) =>
          row.original.updateState ? (
            <StatusBadge tone={updateStateTone(row.original.updateState)}>
              {t(`updateState.${row.original.updateState}`)}
            </StatusBadge>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        size: 160,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.scale')}
              permission={PERM.SWARM_SERVICES_SCALE}
              disabled={!row.original.mode.startsWith('replicated')}
              onClick={(e) => {
                e.stopPropagation();
                onScale(row.original);
              }}
            >
              <IconResize className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.restart')}
              permission={PERM.SWARM_SERVICES_MANAGE}
              onClick={(e) => {
                e.stopPropagation();
                restart.mutate(row.original.id);
              }}
            >
              <IconRefresh className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.rollback')}
              permission={PERM.SWARM_SERVICES_MANAGE}
              onClick={(e) => {
                e.stopPropagation();
                rollback.mutate(row.original.id);
              }}
            >
              <IconArrowBackUp className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.remove')}
              permission={PERM.SWARM_SERVICES_DELETE}
              onClick={(e) => {
                e.stopPropagation();
                onRemove(row.original);
              }}
            >
              <IconTrash className="size-4" />
            </Button>
          </div>
        ),
      },
    ],
    [t, restart, rollback, onScale, onRemove],
  );
  return columns;
}
