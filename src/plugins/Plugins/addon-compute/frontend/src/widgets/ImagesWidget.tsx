// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPhoto } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useImages, useOverview } from '../hooks/useCompute';
import { formatBytes } from '../format';
import { useWidgetContext } from './mount';

/**
 * What the image library is costing, and where it is cached.
 *
 * Images are the one compute resource that grows without anybody deciding to
 * grow it: every guest created from a new release pulls another few gigabytes
 * that nothing ever reclaims. The tile reads as a running total for that.
 */
export function ImagesWidget() {
  const { t } = useTranslation('compute');
  const { navigate } = useWidgetContext();
  const overview = useOverview();
  const images = useImages();

  if (overview.isLoading || images.isLoading) {
    return <Skeleton className="h-full w-full" />;
  }

  const cached = (images.data ?? []).filter((image) => image.cache?.state === 'ready');
  const bytes = cached.reduce((total, image) => total + (image.sizeBytes ?? 0), 0);

  const perHost = (overview.data?.environments ?? [])
    .filter((entry) => entry.computeEnabled && entry.available)
    .map((entry) => ({
      host: entry.name || entry.environmentId,
      count: entry.cachedImages,
    }))
    .sort((a, b) => b.count - a.count);

  if (cached.length === 0 && perHost.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconPhoto className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('images.empty.title')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-compute/images')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-2 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{formatBytes(bytes)}</span>
        <span className="text-sm text-muted-foreground">
          {t('widgets.computeImages.cached', { count: cached.length })}
        </span>
      </div>

      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {perHost.slice(0, 6).map((entry) => (
          <div key={entry.host} className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-sm text-foreground">{entry.host}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t('widgets.computeImages.onHost', { count: entry.count })}
            </span>
          </div>
        ))}
      </div>
    </Button>
  );
}
