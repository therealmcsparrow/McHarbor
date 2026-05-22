// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import type { StackInfo } from '../hooks/useStacks';
import { StackCard } from './StackCard';

type StackCardViewProps = {
  stacks: StackInfo[];
  isLoading: boolean;
  statsMap?: Map<string, BulkContainerMetric>;
  onAction: (name: string, action: string) => void;
  onEdit: (s: StackInfo) => void;
  onLogs: (s: StackInfo) => void;
  onRemove: (s: StackInfo) => void;
  onTakeOver: (s: StackInfo) => void;
};

export function StackCardView({
  stacks,
  isLoading,
  statsMap,
  onAction,
  onEdit,
  onLogs,
  onRemove,
  onTakeOver,
}: StackCardViewProps) {
  const { t } = useTranslation('stacks');
  const [cardSearch, setCardSearch] = useState('');

  const filteredStacks = stacks.filter((s) => {
    if (!cardSearch) return true;
    const q = cardSearch.toLowerCase();
    return (
      s.name.toLowerCase().includes(q) ||
      s.services.some((svc) => svc.name.toLowerCase().includes(q))
    );
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
        {filteredStacks.map((s) => (
          <StackCard
            key={s.id}
            stack={s}
            statsMap={statsMap}
            onAction={onAction}
            onEdit={onEdit}
            onLogs={onLogs}
            onRemove={onRemove}
            onTakeOver={onTakeOver}
          />
        ))}
      </div>
      {!isLoading && stacks.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">{t('emptyMessage')}</p>
      )}
    </div>
  );
}
