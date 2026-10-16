// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ImageInfo } from '@core/types/docker';
import { ImageCard } from './ImageCard';

type ImageCardGridProps = {
  images: ImageInfo[];
  isLoading: boolean;
  onClick: (img: ImageInfo) => void;
  onRemove: (id: string) => void;
};

export function ImageCardGrid({ images, isLoading, onClick, onRemove }: ImageCardGridProps) {
  const { t } = useTranslation('images');

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {images.map((img) => (
          <ImageCard key={img.Id} image={img} onClick={onClick} onRemove={onRemove} />
        ))}
      </div>
      {!isLoading && images.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
