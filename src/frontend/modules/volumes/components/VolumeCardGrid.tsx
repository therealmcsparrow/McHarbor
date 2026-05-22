// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import type { VolumeInfo } from '@core/types/docker';
import { Input } from '@resources/components/ui/Input';
import { VolumeCard } from './VolumeCard';

type VolumeCardGridProps = {
  volumes: VolumeInfo[];
  isLoading: boolean;
  onRemove: (name: string) => void;
};

export function VolumeCardGrid({ volumes, isLoading, onRemove }: VolumeCardGridProps) {
  const { t } = useTranslation('volumes');
  const [search, setSearch] = useState('');

  const filtered = volumes.filter((v) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return v.Name.toLowerCase().includes(q) || v.Driver.toLowerCase().includes(q);
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
        {filtered.map((v) => (
          <VolumeCard key={v.Name} volume={v} onRemove={onRemove} />
        ))}
      </div>
      {!isLoading && volumes.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
