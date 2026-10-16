// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconDatabase, IconInfoCircle, IconPhoto, IconSettings } from '@tabler/icons-react';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useUpdateEnvironment } from '../hooks/useCompute';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { COMPUTE_PERM } from '../permissions';
import { formatBytes, usedPercent } from '../format';
import { hostStatus, worstPool } from '../overview-status';
import { PoolCapacityBar } from './PoolCapacityBar';
import type { OverviewEntry } from '../types';

interface UseComputeOverviewColumnsOptions {
  onNavigate: (to: string) => void;
  onSetup: (environmentId: string) => void;
}

/** The overview table's columns, mirroring the environments table's shape. */
export function useComputeOverviewColumns({ onNavigate, onSetup }: UseComputeOverviewColumnsOptions) {
  const { t } = useTranslation('compute');
  const updateEnvironment = useUpdateEnvironment();
  const { denied } = usePermissionGate(COMPUTE_PERM.MANAGE);

  return useMemo<ColumnDef<OverviewEntry, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('overview.columns.host'),
        cell: ({ row }) => (
          <Button
            variant="link"
            className="h-auto min-w-0 justify-start p-0 text-left"
            onClick={() => onNavigate(`/hosts/${row.original.environmentId}/overview`)}
          >
            <span className="min-w-0">
              <span className="block truncate font-medium text-foreground">{row.original.name}</span>
              <span className="block truncate font-mono text-xs text-muted-foreground">
                {row.original.environmentId}
              </span>
            </span>
          </Button>
        ),
      },
      {
        id: 'status',
        header: t('overview.columns.status'),
        meta: { sortValue: (e: OverviewEntry) => hostStatus(e).key },
        cell: ({ row }) => {
          const hostState = hostStatus(row.original);
          return <Badge variant={hostState.tone}>{t(`overview.status.${hostState.key}`)}</Badge>;
        },
      },
      {
        id: 'platform',
        header: t('overview.columns.platform'),
        meta: {
          sortValue: (e: OverviewEntry) => (e.capability ? `${e.capability.os} · ${e.capability.arch}` : null),
        },
        cell: ({ row }) => {
          const capability = row.original.capability;
          if (!capability) {
            return <span className="text-xs text-muted-foreground">{t('capability.neverProbed')}</span>;
          }
          return (
            <span className="font-mono text-xs text-muted-foreground">
              {capability.os} · {capability.arch}
            </span>
          );
        },
      },
      {
        id: 'pools',
        header: t('overview.columns.pools'),
        meta: {
          sortValue: (e: OverviewEntry) => {
            const fullest = worstPool(e);
            return fullest ? usedPercent(fullest.usedBytes, fullest.capacityBytes) : null;
          },
        },
        cell: ({ row }) => {
          const pools = row.original.pools ?? [];
          const fullest = worstPool(row.original);
          if (!fullest) {
            return <span className="text-sm text-warning">{t('overview.needsPool')}</span>;
          }
          const total = pools.reduce((sum, pool) => sum + pool.capacityBytes, 0);
          return (
            <div className="min-w-[10rem]">
              <PoolCapacityBar pool={fullest} compact />
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {pools.length === 1 ? fullest.name : formatBytes(total)}
              </p>
            </div>
          );
        },
      },
      {
        id: 'networks',
        header: t('overview.columns.networks'),
        meta: {
          sortValue: (e: OverviewEntry) => (e.networks ?? []).map((network) => network.name).join(', '),
        },
        cell: ({ row }) => {
          const networks = row.original.networks ?? [];
          if (networks.length === 0) {
            return <span className="text-sm text-warning">{t('overview.needsNetwork')}</span>;
          }
          return <span className="text-sm text-foreground">{networks.map((network) => network.name).join(', ')}</span>;
        },
      },
      {
        accessorKey: 'cachedImages',
        header: t('overview.columns.images'),
        cell: ({ row }) => <span className="text-sm text-foreground tabular-nums">{row.original.cachedImages}</span>,
      },
      {
        id: 'sharedLibrary',
        header: t('overview.sharedLibrary'),
        enableSorting: false,
        cell: ({ row }) => (
          <Switch
            aria-label={t('overview.sharedLibrary')}
            checked={Boolean(row.original.sharedLibrary)}
            disabled={denied || updateEnvironment.isPending}
            onCheckedChange={(checked) =>
              updateEnvironment.mutate({
                envId: row.original.environmentId,
                sharedLibrary: checked,
              })
            }
          />
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('hosts.detail.open')}
              title={t('hosts.detail.open')}
              onClick={() => onNavigate(`/hosts/${row.original.environmentId}/overview`)}
            >
              <IconInfoCircle className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('nav.pools')}
              title={t('nav.pools')}
              onClick={() => onNavigate('/pools')}
            >
              <IconDatabase className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('nav.images')}
              title={t('nav.images')}
              onClick={() => onNavigate('/images')}
            >
              <IconPhoto className="h-4 w-4" />
            </Button>
            <Button
              permission={COMPUTE_PERM.MANAGE}
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.runWizard')}
              title={t('actions.runWizard')}
              onClick={() => onSetup(row.original.environmentId)}
            >
              <IconSettings className="h-4 w-4" />
            </Button>
          </div>
        ),
      },
    ],
    [t, denied, updateEnvironment, onNavigate, onSetup],
  );
}
