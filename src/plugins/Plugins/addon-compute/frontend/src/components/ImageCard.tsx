// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconTransfer, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { COMPUTE_PERM } from '../permissions';
import { DistroLogo } from './DistroLogo';
import { ImageSize, ImageStateBadge, ImageTrustBadges, sourceHost } from './ImageBadges';
import type { ComputeImage } from '../types';

interface ImageCardProps {
  image: ComputeImage;
  /** False when the selected host cannot take a download. */
  canDownload: boolean;
  downloading: boolean;
  onDownload: (image: ComputeImage) => void;
  onEvict: (image: ComputeImage) => void;
}

/** One catalog image, in the same shape as an environment card. */
export function ImageCard({ image, canDownload, downloading, onDownload, onEvict }: ImageCardProps) {
  const { t } = useTranslation('compute');

  const originLabel = image.origin === 'upload' ? t('images.origin.library') : sourceHost(image.sourceUrl);

  return (
    <Card className="min-h-[300px] transition-colors hover:border-primary/40">
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <DistroLogo distro={image.distro} />
          <div className="min-w-0 flex-1 space-y-1">
            <h3 className="truncate text-base font-semibold text-foreground">{image.displayName}</h3>
            <p className="truncate text-xs text-muted-foreground">
              {t(`images.kind.${image.kind}`)} · {image.arch}
              {image.initSystem ? ` · ${image.initSystem}` : ''}
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {[image.distro, image.release, image.variant].filter(Boolean).join(' ')}
              {originLabel ? ` · ${originLabel}` : ''}
            </p>
          </div>
          {image.isBuiltin ? (
            <Badge variant="secondary" className="shrink-0">
              {t('images.builtin')}
            </Badge>
          ) : image.origin === 'upload' ? (
            <Badge variant="outline" className="shrink-0">
              {t('images.origin.upload')}
            </Badge>
          ) : null}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <ImageStateBadge image={image} />
          <ImageTrustBadges image={image} />
          <ImageSize image={image} />
        </div>

        {image.cache?.state === 'downloading' && (
          <div className="grid gap-1">
            <ProgressBar size="sm" value={image.cache.progressPct ?? 0} />
            {image.cache.phase ? (
              <p className="text-[11px] text-muted-foreground">{t(`images.phase.${image.cache.phase}`)}</p>
            ) : null}
          </div>
        )}

        {image.cache?.lastError && <p className="text-xs break-words text-destructive">{image.cache.lastError}</p>}
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <div className="flex-1" />
        {image.cache?.state === 'ready' ? (
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
        ) : (
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
        )}
      </CardFooter>
    </Card>
  );
}
