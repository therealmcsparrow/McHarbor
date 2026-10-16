// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconFileExport, IconLock, IconTrash } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import type { ImageInfo } from '@core/types/docker';
import { isProtectedImage } from '@core/utils/protection';
import type { BatchAction } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatBytes, timeAgo, truncateId } from '@resources/utils/format';
import { textColumn, bytesColumn, boolColumn, listColumn } from '@resources/layout/table-page/optional-columns';

const SHA_PREFIX = 'sha256:';
const NONE_TAG = '<none>';

type UseImagesPageConfigOptions = {
  onExport: (image: ImageInfo) => void;
  onRemove: (imageId: string) => void;
  onBatchRemove: (images: ImageInfo[]) => void;
};

export function useImagesPageConfig({ onExport, onRemove, onBatchRemove }: UseImagesPageConfigOptions) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');
  const group = t('columns.groupImage');
  const lockedLabel = tc('actions.locked');
  const removeLabel = tc('actions.remove');

  const columns = useMemo<ColumnDef<ImageInfo, unknown>[]>(
    () => [
      {
        id: 'repoTag',
        header: t('columns.repoTag'),
        meta: { sortValue: (i: ImageInfo) => i.RepoTags?.[0] ?? NONE_TAG },
        cell: ({ row }) => {
          const tags = row.original.RepoTags;
          return (
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium">{tags && tags.length > 0 ? tags[0] : NONE_TAG}</span>
              {isProtectedImage(row.original) && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <IconLock className="size-3.5 shrink-0 text-muted-foreground" />
                  </TooltipTrigger>
                  <TooltipContent>{lockedLabel}</TooltipContent>
                </Tooltip>
              )}
            </div>
          );
        },
      },
      {
        id: 'status',
        header: t('columns.status'),
        meta: { sortValue: (i: ImageInfo) => i.Containers > 0 },
        cell: ({ row }) =>
          row.original.Containers > 0 ? (
            <Badge variant="success">{t('badges.inUse')}</Badge>
          ) : (
            <Badge variant="warning">{t('badges.unused')}</Badge>
          ),
      },
      {
        accessorKey: 'Id',
        header: t('columns.id'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {truncateId((row.original.Id ?? '').replace(SHA_PREFIX, ''))}
          </span>
        ),
      },
      {
        accessorKey: 'Size',
        header: t('columns.size'),
        cell: ({ row }) => formatBytes(row.original.Size),
      },
      {
        accessorKey: 'Created',
        header: t('columns.created'),
        cell: ({ row }) => timeAgo(new Date(row.original.Created * 1000).toISOString()),
      },
      listColumn<ImageInfo>('allTags', t('columns.allTags'), (r) => r.RepoTags ?? [], { group }),
      listColumn<ImageInfo>('digests', t('columns.digests'), (r) => r.RepoDigests ?? [], { group, width: 'xl' }),
      textColumn<ImageInfo>(
        'parentId',
        t('columns.parentId'),
        (r) => r.ParentId?.replace(SHA_PREFIX, '').slice(0, 12),
        { group },
      ),
      bytesColumn<ImageInfo>('sharedSize', t('columns.sharedSize'), (r) => r.SharedSize, { group }),
      listColumn<ImageInfo>(
        'labels',
        t('columns.labels'),
        (r) => Object.entries(r.Labels ?? {}).map(([k, v]) => `${k}=${v}`),
        { group, width: 'xl' },
      ),
      textColumn<ImageInfo>('containerCount', t('columns.containerCount'), (r) => r.Containers, { group, width: 'sm' }),
      boolColumn<ImageInfo>('protected', t('columns.protected'), (r) => r.Protected, { group }),
      listColumn<ImageInfo>('usedBy', t('columns.usedBy'), (r) => r.UsedBy, { group: t('columns.groupRelations') }),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('export.title')}
              permission={PERM.IMAGES_MANAGE}
              onClick={(e) => {
                e.stopPropagation();
                onExport(row.original);
              }}
            >
              <IconFileExport className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={removeLabel}
              disabled={isProtectedImage(row.original)}
              onClick={(e) => {
                e.stopPropagation();
                if (isProtectedImage(row.original)) return;
                onRemove(row.original.Id);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [group, lockedLabel, removeLabel, onExport, onRemove, t],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          onBatchRemove((rows as ImageInfo[]).filter((row) => !isProtectedImage(row)));
        },
      },
    ],
    [onBatchRemove, tc],
  );

  return { columns, batchActions };
}
