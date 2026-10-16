// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ImageInspect } from '@core/types/docker';
import { ImageKeyValueRow } from './ImageKeyValueRow';

type ImageLabelsCardProps = {
  image: ImageInspect;
};

export function ImageLabelsCard({ image }: ImageLabelsCardProps) {
  const { t } = useTranslation('images');

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('detail.labels')}
      </h3>
      <div className="max-h-48 overflow-x-hidden overflow-y-auto">
        {image.Config?.Labels && Object.keys(image.Config.Labels).length > 0 ? (
          Object.entries(image.Config.Labels).map(([key, value]) => (
            <ImageKeyValueRow key={key} itemKey={key} value={value} />
          ))
        ) : (
          <p className="text-sm text-muted-foreground">{t('detail.noLabels')}</p>
        )}
      </div>
    </div>
  );
}
