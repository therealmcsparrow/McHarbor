// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconCrown } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { StatusBadge } from '../components/StatusBadge';
import { useSetNodeAvailability, useSetNodeRole } from '../hooks/useSwarmNodes';
import { formatBytes, formatCpus, nodeAvailabilityTone, nodeStateTone } from '../format';
import type { SwarmNode } from '../types';
import { NodeRowActions } from '../components/NodeRowActions';

type Handlers = {
  setAvailability: ReturnType<typeof useSetNodeAvailability>;
  setRole: ReturnType<typeof useSetNodeRole>;
  onRemove: (node: SwarmNode) => void;
};

export function useNodeColumns({ setAvailability, setRole, onRemove }: Handlers) {
  const { t } = useTranslation('swarm');

  const columns = useMemo<ColumnDef<SwarmNode, unknown>[]>(
    () => [
      {
        accessorKey: 'hostname',
        header: t('columns.hostname'),
        meta: { flex: true },
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            {row.original.isLeader && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="shrink-0 text-warning">
                    <IconCrown className="size-4" aria-hidden />
                  </span>
                </TooltipTrigger>
                <TooltipContent>{t('nodes.leader')}</TooltipContent>
              </Tooltip>
            )}
            <span className="truncate font-medium">{row.original.hostname || row.original.id}</span>
            {row.original.self && (
              <Badge variant="outline" className="shrink-0 px-1.5 py-0 text-[10px]">
                {t('nodes.thisHost')}
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'role',
        header: t('columns.role'),
        size: 110,
        cell: ({ row }) => (
          <Badge variant={row.original.role === 'manager' ? 'default' : 'secondary'}>
            {t(`nodeRole.${row.original.role}`)}
          </Badge>
        ),
      },
      {
        accessorKey: 'state',
        header: t('columns.state'),
        size: 110,
        cell: ({ row }) => (
          <StatusBadge tone={nodeStateTone(row.original.state)}>{t(`nodeState.${row.original.state}`)}</StatusBadge>
        ),
      },
      {
        accessorKey: 'availability',
        header: t('columns.availability'),
        size: 120,
        cell: ({ row }) => (
          <StatusBadge tone={nodeAvailabilityTone(row.original.availability)}>
            {t(`nodeAvailability.${row.original.availability}`)}
          </StatusBadge>
        ),
      },
      {
        accessorKey: 'taskCount',
        header: t('columns.tasks'),
        size: 80,
        cell: ({ row }) => <span className="text-sm">{row.original.taskCount}</span>,
      },
      {
        id: 'resources',
        header: t('columns.resources'),
        size: 140,
        meta: { sortValue: (node: SwarmNode) => node.nanoCpus },
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {formatCpus(row.original.nanoCpus)} · {formatBytes(row.original.memoryBytes)}
          </span>
        ),
      },
      {
        accessorKey: 'engineVersion',
        header: t('columns.engine'),
        size: 100,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.engineVersion || '—'}</span>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        size: 160,
        cell: ({ row }) => (
          <NodeRowActions node={row.original} setAvailability={setAvailability} setRole={setRole} onRemove={onRemove} />
        ),
      },
    ],
    [t, setAvailability, setRole, onRemove],
  );
  return columns;
}
