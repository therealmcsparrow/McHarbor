// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconShieldCheck, IconShieldOff } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatBytes } from '../format';
import type { ComputeImage } from '../types';

/** Where the image stands on the selected host. */
export function ImageStateBadge({ image }: { image: ComputeImage }) {
  const { t } = useTranslation('compute');

  switch (image.cache?.state) {
    case 'ready':
      return <Badge variant="success">{t('images.state.ready')}</Badge>;
    case 'downloading':
      return (
        <Badge variant="default">
          {image.cache?.phase && image.cache.phase !== 'downloading'
            ? t(`images.phase.${image.cache.phase}`)
            : t('images.downloading', { percent: image.cache?.progressPct ?? 0 })}
        </Badge>
      );
    case 'pending':
      return <Badge variant="secondary">{t('images.state.pending')}</Badge>;
    case 'failed':
      return <Badge variant="destructive">{t('images.state.failed')}</Badge>;
    default:
      return <Badge variant="outline">{t('images.state.absent')}</Badge>;
  }
}

/**
 * What is known about the bytes: checksum, signature, or neither.
 *
 * An entry with no published checksum is marked before the operator clicks
 * anything — finding out at the confirmation step would be later than it needs
 * to be.
 */
export function ImageTrustBadges({ image }: { image: ComputeImage }) {
  const { t } = useTranslation('compute');

  return (
    <>
      {image.cache?.verified && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="success">
              <IconCheck className="h-3 w-3" aria-hidden="true" />
              {t('images.verified')}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{t('images.verifiedTooltip')}</TooltipContent>
        </Tooltip>
      )}

      {image.cache?.signed && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="success">
              <IconShieldCheck className="h-3 w-3" aria-hidden="true" />
              {t('images.signed')}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{t('images.signedTooltip')}</TooltipContent>
        </Tooltip>
      )}

      {image.origin !== 'upload' &&
        (image.checksum || image.checksumUrl) &&
        !image.signatureUrl &&
        !image.cache?.signed && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge variant="outline">
                <IconShieldOff className="h-3 w-3" aria-hidden="true" />
                {t('images.unsigned.title')}
              </Badge>
            </TooltipTrigger>
            <TooltipContent>{t('images.unsigned.body')}</TooltipContent>
          </Tooltip>
        )}

      {image.origin !== 'upload' && !image.checksum && !image.checksumUrl && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="warning">
              <IconAlertTriangle className="h-3 w-3" aria-hidden="true" />
              {t('images.unverified.title')}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{t('images.unverified.body')}</TooltipContent>
        </Tooltip>
      )}
    </>
  );
}

/** The size, marked approximate until the image is actually on a host. */
export function ImageSize({ image }: { image: ComputeImage }) {
  const { t } = useTranslation('compute');

  if (image.cache?.sizeBytes) {
    return <span className="text-xs text-muted-foreground">{formatBytes(image.cache.sizeBytes)}</span>;
  }
  if (image.sizeBytes) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="text-xs text-muted-foreground">~{formatBytes(image.sizeBytes)}</span>
        </TooltipTrigger>
        <TooltipContent>{t('images.sizeApproximate')}</TooltipContent>
      </Tooltip>
    );
  }
  return <span className="text-xs text-muted-foreground">—</span>;
}

/**
 * Where the bytes come from.
 *
 * The full URL is too long for a row, and the host is the part an operator
 * actually checks.
 */
export function sourceHost(url?: string): string {
  if (!url) return '';
  try {
    return new URL(url).host;
  } catch {
    return '';
  }
}
