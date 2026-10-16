// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconTrash, IconLock } from '@tabler/icons-react';
import type { VolumeInfo } from '@core/types/docker';
import { isProtectedVolume } from '@core/utils/protection';
import type { BatchAction } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';
import { textColumn, bytesColumn, boolColumn, listColumn } from '@resources/layout/table-page/optional-columns';

export function useVolumesPageConfig(
  onRequestRemove: (volumeName: string) => void,
  onBatchRemove: (volumeName: string) => void,
) {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const group = t('columns.groupVolume');
  const lockedLabel = tc('actions.locked');
  const removeLabel = tc('actions.remove');
  const inUseLabel = useCallback((count: number) => t('badges.inUseCount', { count }), [t]);

  const columns = useMemo<ColumnDef<VolumeInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Name',
        header: t('columns.name'),
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{row.original.Name}</span>
            {isProtectedVolume(row.original) && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconLock className="size-3.5 shrink-0 text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>{lockedLabel}</TooltipContent>
              </Tooltip>
            )}
          </div>
        ),
      },
      { accessorKey: 'Driver', header: t('columns.driver') },
      {
        id: 'usage',
        header: t('columns.usage'),
        meta: { sortValue: (v: VolumeInfo) => v.RefCount },
        cell: ({ row }) => {
          const count = row.original.RefCount;
          return count > 0 ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="success">{t('badges.inUse')}</Badge>
              </TooltipTrigger>
              <TooltipContent>{inUseLabel(count)}</TooltipContent>
            </Tooltip>
          ) : (
            <Badge variant="warning">{t('badges.unused')}</Badge>
          );
        },
      },
      {
        accessorKey: 'Mountpoint',
        header: t('columns.mountpoint'),
        cell: ({ row }) => (
          <span className="max-w-xs truncate font-mono text-xs text-muted-foreground">{row.original.Mountpoint}</span>
        ),
      },
      {
        accessorKey: 'CreatedAt',
        header: t('columns.created'),
        cell: ({ row }) => formatDate(row.original.CreatedAt),
      },
      textColumn<VolumeInfo>('scope', t('columns.scope'), (r) => r.Scope, { group, width: 'sm' }),
      listColumn<VolumeInfo>(
        'labels',
        t('columns.labels'),
        (r) => Object.entries(r.Labels ?? {}).map(([k, v]) => `${k}=${v}`),
        { group, width: 'xl' },
      ),
      listColumn<VolumeInfo>(
        'options',
        t('columns.options'),
        (r) => Object.entries(r.Options ?? {}).map(([k, v]) => `${k}=${v}`),
        { group },
      ),
      listColumn<VolumeInfo>(
        'status',
        t('columns.status'),
        (r) => Object.entries(r.Status ?? {}).map(([k, v]) => `${k}=${String(v)}`),
        { group },
      ),
      bytesColumn<VolumeInfo>('size', t('columns.size'), (r) => r.UsageData?.Size, { group }),
      textColumn<VolumeInfo>('refCount', t('columns.refCount'), (r) => r.RefCount ?? r.UsageData?.RefCount, {
        group,
        width: 'sm',
      }),
      textColumn<VolumeInfo>('stack', t('columns.stack'), (r) => r.Labels?.['com.docker.compose.project'], {
        group: t('columns.groupStack'),
      }),
      listColumn<VolumeInfo>('usedBy', t('columns.usedBy'), (r) => r.UsedBy, { group: t('columns.groupRelations') }),
      boolColumn<VolumeInfo>('protected', t('columns.protected'), (r) => r.Protected, { group }),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Button
              variant="ghost"
              size="icon"
              title={removeLabel}
              aria-label={removeLabel}
              disabled={isProtectedVolume(row.original)}
              onClick={(e) => {
                e.stopPropagation();
                if (isProtectedVolume(row.original)) return;
                onRequestRemove(row.original.Name);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [group, inUseLabel, lockedLabel, removeLabel, onRequestRemove, t],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          for (const row of rows as VolumeInfo[]) {
            if (isProtectedVolume(row)) continue;
            onBatchRemove(row.Name);
          }
        },
      },
    ],
    [tc, onBatchRemove],
  );

  return { columns, batchActions };
}
