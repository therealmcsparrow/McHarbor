// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { VolumeInfo } from '@core/types/docker';
import { VolumeCard } from './VolumeCard';

type VolumeCardGridProps = {
  volumes: VolumeInfo[];
  isLoading: boolean;
  onRemove: (name: string) => void;
  onOpen: (name: string) => void;
};

export function VolumeCardGrid({ volumes, isLoading, onRemove, onOpen }: VolumeCardGridProps) {
  const { t } = useTranslation('volumes');

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {volumes.map((v) => (
          <VolumeCard key={v.Name} volume={v} onRemove={onRemove} onOpen={onOpen} />
        ))}
      </div>
      {!isLoading && volumes.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
