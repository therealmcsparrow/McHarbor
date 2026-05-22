// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconPencil, IconTrash } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import type { BatchAction } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';

export function useNetworksPageConfig(onRemove: (networkId: string) => void) {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');
  const { t: tc } = useTranslation('common');

  const columns = useMemo<ColumnDef<NetworkInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => <span className="font-medium">{row.original.Name}</span>,
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
          <Badge variant="default" className="px-1.5 py-0 text-[10px]">{row.original.Driver}</Badge>
        ),
      },
      {
        accessorKey: 'Scope',
        header: t('columns.scope'),
        size: 80,
        cell: ({ row }) => (
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">{row.original.Scope}</Badge>
        ),
      },
      {
        accessorKey: 'Internal',
        header: t('columns.internal'),
        size: 80,
        cell: ({ row }) =>
          row.original.Internal ? (
            <Badge variant="warning" className="px-1.5 py-0 text-[10px]">{t('badges.yes')}</Badge>
          ) : (
            <span className="text-xs text-muted-foreground">{t('badges.no')}</span>
          ),
      },
      {
        id: 'containers',
        header: t('columns.containers'),
        size: 90,
        cell: ({ row }) => {
          const containers = row.original.Containers;
          const count = typeof containers === 'number' ? containers : containers ? Object.keys(containers).length : 0;
          return <span className="text-xs tabular-nums text-muted-foreground">{count}</span>;
        },
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        size: 100,
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('actions.edit')}
                  onClick={(event) => {
                    event.stopPropagation();
                    navigate(`/networks/${row.original.Id}`);
                  }}
                >
                  <IconPencil className="h-3.5 w-3.5 text-primary" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('actions.edit')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('actions.remove')}
                  onClick={(event) => {
                    event.stopPropagation();
                    onRemove(row.original.Id);
                  }}
                >
                  <IconTrash className="h-3.5 w-3.5 text-destructive" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('actions.remove')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [navigate, onRemove, t],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          for (const row of rows as NetworkInfo[]) {
            onRemove(row.Id);
          }
        },
      },
    ],
    [onRemove, tc],
  );

  return { columns, batchActions };
}
