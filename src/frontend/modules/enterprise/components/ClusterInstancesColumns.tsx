// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowsExchange, IconPlayerPause, IconPlayerPlay } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { formatDate, timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { ClusterInstance } from '../hooks/useCluster';
import { nodeHealth, releaseOf } from './useClusterInstanceFilters';

export type NodeAction = 'failover' | 'drain' | 'resume';

const DASH = '—';
const ROLE_VARIANT = { active: 'success', standby: 'secondary', peer: 'info' } as const;
const STATE_VARIANT = { running: 'success', draining: 'warning', stopped: 'secondary' } as const;
const HEALTH_VARIANT = { up: 'success', degraded: 'warning', down: 'destructive' } as const;

type ClusterColumnsArgs = {
  conflict: boolean;
  activePassive: boolean;
  drifted: (node: ClusterInstance) => boolean;
  onAction: (kind: NodeAction, node: ClusterInstance) => void;
};

export function useClusterInstanceColumns({
  conflict,
  activePassive,
  drifted,
  onAction,
}: ClusterColumnsArgs): ColumnDef<ClusterInstance, unknown>[] {
  const { t } = useTranslation('enterprise');

  return useMemo<ColumnDef<ClusterInstance, unknown>[]>(
    () => [
      {
        id: 'node',
        accessorFn: (node) => node.id,
        header: t('ha.columns.node'),
        meta: { locked: true, label: t('ha.columns.node') } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div>
            <span className="font-medium text-foreground">{row.original.id}</span>
            {row.original.self && (
              <Badge variant="outline" className="ml-2">
                {t('ha.thisNode')}
              </Badge>
            )}
            <div className="text-xs text-muted-foreground">{row.original.hostname}</div>
          </div>
        ),
      },
      {
        id: 'role',
        accessorFn: (node) => t(`ha.role.${node.haRole}`),
        header: t('ha.columns.role'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={ROLE_VARIANT[row.original.haRole] ?? 'secondary'}>
            {t(`ha.role.${row.original.haRole}`)}
          </Badge>
        ),
      },
      {
        id: 'state',
        accessorFn: (node) => t(`ha.state.${node.state}`),
        header: t('ha.columns.state'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={STATE_VARIANT[row.original.state] ?? 'secondary'}>
            {t(`ha.state.${row.original.state}`)}
          </Badge>
        ),
      },
      {
        id: 'health',
        accessorFn: (node) => t(`ha.health.${nodeHealth(node)}`),
        header: t('ha.columns.health'),
        meta: { width: 'sm', defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={HEALTH_VARIANT[nodeHealth(row.original)]}>{t(`ha.health.${nodeHealth(row.original)}`)}</Badge>
        ),
      },
      {
        id: 'heartbeat',
        accessorFn: (node) => node.heartbeatAt,
        header: t('ha.columns.heartbeat'),
        cell: ({ row }) => (
          <span className={row.original.alive ? 'text-foreground' : 'text-destructive'}>
            {row.original.alive ? timeAgo(row.original.heartbeatAt) : t('ha.notResponding')}
          </span>
        ),
      },
      {
        id: 'version',
        accessorFn: releaseOf,
        header: t('ha.columns.version'),
        cell: ({ row }) => (
          <span className={drifted(row.original) ? 'font-mono text-xs text-warning' : 'font-mono text-xs'}>
            {releaseOf(row.original)}
          </span>
        ),
      },
      {
        id: 'agents',
        accessorFn: (node) => node.agents,
        header: t('ha.columns.agents'),
        meta: { width: 'xs' } satisfies TableColumnMeta,
      },
      {
        id: 'advertiseUrl',
        accessorFn: (node) => node.advertiseUrl || DASH,
        header: t('ha.columns.advertiseUrl'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.advertiseUrl || DASH}</span>
        ),
      },
      {
        id: 'started',
        accessorFn: (node) => node.startedAt,
        header: t('ha.columns.started'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="text-muted-foreground">{formatDate(row.original.startedAt)}</span>,
      },
      {
        id: 'actions',
        header: t('ha.columns.actions'),
        enableSorting: false,
        meta: { locked: true, noExport: true, label: t('ha.columns.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <NodeActions node={row.original} conflict={conflict} activePassive={activePassive} onAction={onAction} />
        ),
      },
    ],
    [t, conflict, activePassive, drifted, onAction],
  );
}

type NodeActionsProps = {
  node: ClusterInstance;
  conflict: boolean;
  activePassive: boolean;
  onAction: (kind: NodeAction, node: ClusterInstance) => void;
};

function NodeActions({ node, conflict, activePassive, onAction }: NodeActionsProps) {
  const { t } = useTranslation('enterprise');
  const draining = node.state === 'draining';
  return (
    <div className="flex justify-end">
      {activePassive && node.haRole === 'standby' && (
        <Button
          variant="outline"
          size="sm"
          permission={PERM.CLUSTER_MANAGE}
          disabled={conflict || !node.alive || node.state !== 'running'}
          onClick={() => onAction('failover', node)}
        >
          <IconArrowsExchange className="size-4" aria-hidden="true" />
          {t('ha.actions.makeActive')}
        </Button>
      )}
      <Button
        variant="ghost"
        size="sm"
        className="ml-1"
        permission={PERM.CLUSTER_MANAGE}
        disabled={conflict || node.state === 'stopped'}
        onClick={() => onAction(draining ? 'resume' : 'drain', node)}
      >
        {draining ? (
          <IconPlayerPlay className="size-4" aria-hidden="true" />
        ) : (
          <IconPlayerPause className="size-4" aria-hidden="true" />
        )}
        {t(draining ? 'ha.actions.resume' : 'ha.actions.drain')}
      </Button>
    </div>
  );
}
