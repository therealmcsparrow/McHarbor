// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconPencil, IconTrash } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import type { BatchAction } from '@resources/layout/table-page/types';
import { getNetworkContainerCount, isBuiltInNetwork } from '@core/utils/network';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';
import { textColumn, dateColumn, boolColumn, listColumn } from '@resources/layout/table-page/optional-columns';

export function useNetworksPageConfig(
  onRemove: (networkId: string) => void,
  onRemoveSelected?: (rows: NetworkInfo[]) => void,
) {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const { t: tc } = useTranslation('common');
  const group = t('columns.groupNetwork');

  const columns = useMemo<ColumnDef<NetworkInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => {
          const builtIn = isBuiltInNetwork(row.original);
          return (
            <div className="flex items-center gap-2">
              <span className="font-medium">{row.original.Name}</span>
              {builtIn && (
                <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                  {t('badges.builtIn')}
                </Badge>
              )}
            </div>
          );
        },
      },
      {
        accessorKey: 'Id',
        header: t('columns.id'),
        size: 110,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{truncateId(row.original.Id)}</span>
        ),
      },
      {
        accessorKey: 'Driver',
        header: t('columns.driver'),
        size: 90,
        cell: ({ row }) => (
          <Badge variant="default" className="px-1.5 py-0 text-[10px]">
            {row.original.Driver}
          </Badge>
        ),
      },
      {
        accessorKey: 'Scope',
        header: t('columns.scope'),
        size: 80,
        cell: ({ row }) => (
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
            {row.original.Scope}
          </Badge>
        ),
      },
      {
        accessorKey: 'Internal',
        header: t('columns.internal'),
        size: 80,
        cell: ({ row }) =>
          row.original.Internal ? (
            <Badge variant="warning" className="px-1.5 py-0 text-[10px]">
              {t('badges.yes')}
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">{t('badges.no')}</span>
          ),
      },
      {
        id: 'containers',
        header: t('columns.containers'),
        size: 90,
        meta: { sortValue: getNetworkContainerCount },
        cell: ({ row }) => {
          const count = getNetworkContainerCount(row.original);
          const inUse = count > 0;
          return (
            <div className="flex items-center gap-2">
              <span className="text-xs text-foreground tabular-nums">{count}</span>
              <Badge variant={inUse ? 'success' : 'secondary'} className="px-1.5 py-0 text-[10px]">
                {inUse ? t('badges.used') : t('badges.unused')}
              </Badge>
            </div>
          );
        },
      },
      dateColumn<NetworkInfo>('created', t('columns.created'), (r) => r.Created, { group }),
      boolColumn<NetworkInfo>('attachable', t('columns.attachable'), (r) => r.Attachable, { group }),
      listColumn<NetworkInfo>(
        'subnet',
        t('columns.subnet'),
        (r) => (r.IPAM?.Config ?? []).map((c) => c.Subnet).filter(Boolean) as string[],
        { group },
      ),
      listColumn<NetworkInfo>(
        'gateway',
        t('columns.gateway'),
        (r) => (r.IPAM?.Config ?? []).map((c) => c.Gateway).filter(Boolean) as string[],
        { group },
      ),
      listColumn<NetworkInfo>(
        'ipRange',
        t('columns.ipRange'),
        (r) => (r.IPAM?.Config ?? []).map((c) => c.IPRange).filter(Boolean) as string[],
        { group },
      ),
      textColumn<NetworkInfo>('ipamDriver', t('columns.ipamDriver'), (r) => r.IPAM?.Driver, { group, width: 'sm' }),
      listColumn<NetworkInfo>(
        'options',
        t('columns.options'),
        (r) => Object.entries(r.Options ?? {}).map(([k, v]) => `${k}=${v}`),
        { group },
      ),
      listColumn<NetworkInfo>(
        'labels',
        t('columns.labels'),
        (r) => Object.entries(r.Labels ?? {}).map(([k, v]) => `${k}=${v}`),
        { group, width: 'xl' },
      ),
      boolColumn<NetworkInfo>('builtIn', t('columns.builtIn'), (r) => r.BuiltIn, { group }),
      listColumn<NetworkInfo>('usedBy', t('columns.usedBy'), (r) => r.ContainerNames, {
        group: t('columns.groupRelations'),
      }),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 100,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => {
          const builtIn = isBuiltInNetwork(row.original);
          return (
            <div className="flex items-center justify-end">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('actions.edit')}
                    permission={PERM.NETWORKS_MANAGE}
                    disabled={builtIn}
                    onClick={(e) => {
                      e.stopPropagation();
                      void navigate(`/networks/${row.original.Id}?edit=1`);
                    }}
                  >
                    <IconPencil className="h-3.5 w-3.5 text-primary" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('actions.edit')}</TooltipContent>
              </Tooltip>
              {builtIn ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t('actions.remove')}
                      aria-disabled="true"
                      onClick={(e) => e.preventDefault()}
                      className="cursor-not-allowed opacity-40 hover:bg-transparent"
                    >
                      <IconTrash className="h-3.5 w-3.5 text-destructive" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('toast.builtIn')}</TooltipContent>
                </Tooltip>
              ) : (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t('actions.remove')}
                      onClick={(e) => {
                        e.stopPropagation();
                        onRemove(row.original.Id);
                      }}
                    >
                      <IconTrash className="h-3.5 w-3.5 text-destructive" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('actions.remove')}</TooltipContent>
                </Tooltip>
              )}
            </div>
          );
        },
      },
    ],
    [navigate, onRemove, t, group],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        onClick: (rows) => onRemoveSelected?.(rows as NetworkInfo[]),
      },
    ],
    [onRemoveSelected, tc],
  );

  return { columns, batchActions };
}
