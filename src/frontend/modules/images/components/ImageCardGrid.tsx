// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import type { ImageInfo } from '@core/types/docker';
import { Input } from '@resources/components/ui/Input';
import { ImageCard } from './ImageCard';

type ImageCardGridProps = {
  images: ImageInfo[];
  isLoading: boolean;
  onClick: (img: ImageInfo) => void;
  onRemove: (id: string) => void;
};

export function ImageCardGrid({ images, isLoading, onClick, onRemove }: ImageCardGridProps) {
  const { t } = useTranslation('images');
  const [search, setSearch] = useState('');

  const filtered = images.filter((img) => {
    if (!search) return true;
    const q = search.toLowerCase();
    const tag = img.RepoTags?.[0]?.toLowerCase() ?? '';
    const id = img.Id.toLowerCase();
    return tag.includes(q) || id.includes(q);
  });

  return (
    <div className="space-y-4">
      <div className="relative">
        <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t('searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {filtered.map((img) => (
          <ImageCard key={img.Id} image={img} onClick={onClick} onRemove={onRemove} />
        ))}
      </div>
      {!isLoading && images.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
