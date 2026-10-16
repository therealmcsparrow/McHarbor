// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconCloudDownload, IconDownload, IconTransfer, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { StatusError } from '@resources/components/ui/StatusError';
import { COMPUTE_PERM } from '../permissions';
import { DistroLogo } from './DistroLogo';
import { ImageSize, ImageStateBadge, ImageTrustBadges, sourceHost } from './ImageBadges';
import type { ComputeImage } from '../types';

interface UseImageColumnsOptions {
  /** False when the selected host cannot take a download, so every row says so. */
  canDownload: boolean;
  downloading: boolean;
  onDownload: (image: ComputeImage) => void;
  onDownloadEverywhere: (image: ComputeImage) => void;
  onEvict: (image: ComputeImage) => void;
}

/** The image catalog's columns. */
export function useImageColumns({
  canDownload,
  downloading,
  onDownload,
  onDownloadEverywhere,
  onEvict,
}: UseImageColumnsOptions) {
  const { t } = useTranslation('compute');

  return useMemo<ColumnDef<ComputeImage, unknown>[]>(
    () => [
      {
        accessorKey: 'displayName',
        header: t('images.columns.name'),
        cell: ({ row }) => {
          const image = row.original;
          const originLabel = image.origin === 'upload' ? t('images.origin.library') : sourceHost(image.sourceUrl);
          return (
            <div className="flex min-w-0 items-center gap-2.5">
              <DistroLogo distro={image.distro} size="sm" />
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-medium text-foreground">{image.displayName}</span>
                  {image.isBuiltin ? (
                    <Badge variant="secondary">{t('images.builtin')}</Badge>
                  ) : image.origin === 'upload' ? (
                    <Badge variant="outline">{t('images.origin.upload')}</Badge>
                  ) : null}
                </div>
                <p className="truncate text-xs text-muted-foreground">
                  {[image.distro, image.release, image.variant].filter(Boolean).join(' ')}
                  {originLabel ? ` · ${originLabel}` : ''}
                </p>
              </div>
            </div>
          );
        },
      },
      {
        accessorKey: 'kind',
        header: t('images.columns.kind'),
        cell: ({ row }) => <span className="text-sm text-foreground">{t(`images.kind.${row.original.kind}`)}</span>,
      },
      {
        accessorKey: 'arch',
        header: t('images.columns.arch'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {row.original.arch}
            {row.original.initSystem ? ` · ${row.original.initSystem}` : ''}
          </span>
        ),
      },
      {
        id: 'size',
        header: t('images.columns.size'),
        meta: {
          sortValue: (image: ComputeImage) => image.cache?.sizeBytes || image.sizeBytes || null,
        },
        cell: ({ row }) => <ImageSize image={row.original} />,
      },
      {
        id: 'state',
        header: t('images.columns.state'),
        meta: { sortValue: (image: ComputeImage) => image.cache?.state ?? 'absent' },
        cell: ({ row }) => {
          const image = row.original;
          return (
            <div className="min-w-[10rem] space-y-1.5">
              <div className="flex flex-wrap items-center gap-1.5">
                <ImageStateBadge image={image} />
                <ImageTrustBadges image={image} />
                <StatusError error={image.cache?.lastError} />
              </div>
              {image.cache?.state === 'downloading' && (
                <div className="grid gap-1">
                  <ProgressBar size="sm" value={image.cache.progressPct ?? 0} />
                  {image.cache.phase ? (
                    <p className="text-[11px] text-muted-foreground">{t(`images.phase.${image.cache.phase}`)}</p>
                  ) : null}
                </div>
              )}
            </div>
          );
        },
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        enableSorting: false,
        cell: ({ row }) => {
          const image = row.original;
          if (image.cache?.state === 'ready') {
            return (
              <div className="flex justify-end">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('actions.evict')}
                  title={t('actions.evict')}
                  permission={COMPUTE_PERM.DELETE}
                  onClick={() => onEvict(image)}
                >
                  <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
                </Button>
              </div>
            );
          }
          return (
            <div className="flex justify-end gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={image.origin === 'upload' ? t('images.origin.send') : t('actions.download')}
                title={image.origin === 'upload' ? t('images.origin.send') : t('actions.download')}
                permission={COMPUTE_PERM.MANAGE}
                disabled={!canDownload || downloading || image.cache?.state === 'downloading'}
                onClick={() => onDownload(image)}
              >
                {image.origin === 'upload' ? (
                  <IconTransfer className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <IconDownload className="h-4 w-4" aria-hidden="true" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('images.downloadEverywhere')}
                title={t('images.downloadEverywhere')}
                permission={COMPUTE_PERM.MANAGE}
                disabled={downloading}
                onClick={() => onDownloadEverywhere(image)}
              >
                <IconCloudDownload className="h-4 w-4" aria-hidden="true" />
              </Button>
            </div>
          );
        },
      },
    ],
    [t, canDownload, downloading, onDownload, onDownloadEverywhere, onEvict],
  );
}
