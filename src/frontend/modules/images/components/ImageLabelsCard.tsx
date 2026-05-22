// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ImageInspect } from '@core/types/docker';

type ImageLabelsCardProps = {
  image: ImageInspect;
};

export function ImageLabelsCard({ image }: ImageLabelsCardProps) {
  const { t } = useTranslation('images');

  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t('detail.labels')}
      </h3>
      <div className="max-h-48 overflow-y-auto">
        {image.Config?.Labels && Object.keys(image.Config.Labels).length > 0 ? (
          Object.entries(image.Config.Labels).map(([key, value]) => (
            <div key={key} className="flex gap-2 border-b border-border py-1.5 last:border-0">
              <span className="shrink-0 font-mono text-xs font-medium text-foreground">{key}</span>
              <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
            </div>
          ))
        ) : (
          <p className="text-sm text-muted-foreground">{t('detail.noLabels')}</p>
        )}
      </div>
    </div>
  );
}
