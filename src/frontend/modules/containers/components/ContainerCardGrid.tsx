// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import type { ContainerInfo } from '@core/types/docker';
import type { BulkContainerMetric } from '../hooks/useContainersBulkStats';
import { Input } from '@resources/components/ui/Input';
import { ContainerCard } from './ContainerCard';

type ContainerCardGridProps = {
  containers: ContainerInfo[];
  statsMap: Map<string, BulkContainerMetric> | undefined;
  highlightedIds?: Set<string>;
  isLoading: boolean;
  onAction: (id: string, action: string) => void;
  onTerminal: (c: ContainerInfo) => void;
  onLogs: (c: ContainerInfo) => void;
  onRemove: (c: ContainerInfo) => void;
  onClick: (c: ContainerInfo) => void;
};

export function ContainerCardGrid({
  containers,
  statsMap,
  highlightedIds,
  isLoading,
  onAction,
  onTerminal,
  onLogs,
  onRemove,
  onClick,
}: ContainerCardGridProps) {
  const { t } = useTranslation('containers');
  const [cardSearch, setCardSearch] = useState('');

  const filtered = containers.filter((c) => {
    if (!cardSearch) return true;
    const q = cardSearch.toLowerCase();
    const name = c.Names?.[0]?.toLowerCase() ?? '';
    const image = c.Image.toLowerCase();
    return name.includes(q) || image.includes(q);
  });

  return (
    <div className="space-y-4">
      <div className="relative">
        <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t('searchPlaceholder')}
          value={cardSearch}
          onChange={(e) => setCardSearch(e.target.value)}
          className="pl-9"
        />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {filtered.map((c) => (
          <ContainerCard
            key={c.Id}
            container={c}
            stats={statsMap?.get(c.Id)}
            highlighted={highlightedIds?.has(c.Id)}
            onAction={onAction}
            onTerminal={onTerminal}
            onLogs={onLogs}
            onRemove={onRemove}
            onClick={onClick}
          />
        ))}
      </div>
      {!isLoading && containers.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('noContainersFound')}</p>
      )}
    </div>
  );
}
