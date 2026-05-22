// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconFileExport, IconFilterOff, IconTrash } from '@tabler/icons-react';
import type { ImageInfo } from '@core/types/docker';
import type { BatchAction } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { formatBytes, timeAgo, truncateId } from '@resources/utils/format';

type UseImagesPageConfigOptions = {
  onExport: (image: ImageInfo) => void;
  onRemove: (imageId: string) => void;
  onBatchRemove: (images: ImageInfo[]) => void;
  onPrune: () => void;
};

export function useImagesPageConfig({
  onExport,
  onRemove,
  onBatchRemove,
  onPrune,
}: UseImagesPageConfigOptions) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');

  const columns = useMemo<ColumnDef<ImageInfo, unknown>[]>(
    () => [
      {
        id: 'repoTag',
        header: t('columns.repoTag'),
        cell: ({ row }) => {
          const tags = row.original.RepoTags;
          return <span className="font-medium">{tags && tags.length > 0 ? tags[0] : '<none>'}</span>;
        },
      },
      {
        id: 'status',
        header: t('columns.status'),
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
            {truncateId((row.original.Id ?? '').replace('sha256:', ''))}
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
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('export.title')}
              onClick={(event) => {
                event.stopPropagation();
                onExport(row.original);
              }}
            >
              <IconFileExport className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={tc('actions.remove')}
              onClick={(event) => {
                event.stopPropagation();
                onRemove(row.original.Id);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [onExport, onRemove, t, tc],
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          onBatchRemove(rows as ImageInfo[]);
        },
      },
      {
        label: tc('batch.prune'),
        icon: IconFilterOff,
        variant: 'default',
        confirm: true,
        onClick: () => {
          onPrune();
        },
      },
    ],
    [onBatchRemove, onPrune, tc],
  );

  return { columns, batchActions };
}
