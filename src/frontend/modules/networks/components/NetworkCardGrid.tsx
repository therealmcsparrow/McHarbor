// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { Input } from '@resources/components/ui/Input';
import { NetworkCard } from './NetworkCard';

type NetworkCardGridProps = {
  networks: NetworkInfo[];
  isLoading: boolean;
  onClick: (net: NetworkInfo) => void;
  onRemove: (id: string) => void;
};

export function NetworkCardGrid({ networks, isLoading, onClick, onRemove }: NetworkCardGridProps) {
  const { t } = useTranslation('networks');
  const [search, setSearch] = useState('');

  const filtered = networks.filter((n) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return n.Name.toLowerCase().includes(q) || n.Driver.toLowerCase().includes(q);
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
        {filtered.map((n) => (
          <NetworkCard key={n.Id} network={n} onClick={onClick} onRemove={onRemove} />
        ))}
      </div>
      {!isLoading && networks.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
