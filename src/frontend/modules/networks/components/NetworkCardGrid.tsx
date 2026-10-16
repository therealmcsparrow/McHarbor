// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { NetworkInfo } from '@core/types/docker';
import { NetworkCard } from './NetworkCard';

type NetworkCardGridProps = {
  networks: NetworkInfo[];
  isLoading: boolean;
  onClick: (net: NetworkInfo) => void;
  onRemove: (id: string) => void;
};

export function NetworkCardGrid({ networks, isLoading, onClick, onRemove }: NetworkCardGridProps) {
  const { t } = useTranslation('networks');

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {networks.map((n) => (
          <NetworkCard key={n.Id} network={n} onClick={onClick} onRemove={onRemove} />
        ))}
      </div>
      {!isLoading && networks.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
