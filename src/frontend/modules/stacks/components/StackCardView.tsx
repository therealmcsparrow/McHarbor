// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import type { StackInfo } from '@resources/hooks/useStacks';
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

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {stacks.map((s) => (
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
