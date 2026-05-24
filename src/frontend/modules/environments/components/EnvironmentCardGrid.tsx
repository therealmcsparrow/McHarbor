// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { EnvironmentCard } from './EnvironmentCard';

type EnvironmentCardGridProps = {
  environments: EnvironmentListItem[];
  isLoading: boolean;
  onTest: (id: string) => void;
  onRemove: (id: string) => void;
};

function EnvironmentCardSkeleton() {
  return (
    <div className="min-h-[350px] animate-pulse rounded-xl border border-border bg-card p-4">
      <div className="h-5 w-2/3 rounded bg-muted" />
      <div className="mt-2 h-4 w-1/2 rounded bg-muted" />
      <div className="mt-6 grid grid-cols-2 gap-3">
        <div className="h-24 rounded-lg bg-muted" />
        <div className="h-24 rounded-lg bg-muted" />
      </div>
      <div className="mt-4 grid grid-cols-4 gap-2">
        <div className="h-16 rounded-lg bg-muted" />
        <div className="h-16 rounded-lg bg-muted" />
        <div className="h-16 rounded-lg bg-muted" />
        <div className="h-16 rounded-lg bg-muted" />
      </div>
    </div>
  );
}

export function EnvironmentCardGrid({
  environments,
  isLoading,
  onTest,
  onRemove,
}: EnvironmentCardGridProps) {
  const { t } = useTranslation('environments');
  const [cardSearch, setCardSearch] = useState('');

  const filtered = environments.filter((environment) => {
    if (!cardSearch) return true;
    const query = cardSearch.toLowerCase();
    return [
      environment.name,
      environment.connectionType,
      environment.orchestratorType,
      environment.host,
      environment.socketPath,
      environment.agentHostname,
    ].some((value) => value?.toLowerCase().includes(query));
  });

  return (
    <div className="space-y-4">
      <div className="relative max-w-sm">
        <IconSearch className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t('searchPlaceholder')}
          value={cardSearch}
          onChange={(event) => setCardSearch(event.target.value)}
          className="pl-9"
        />
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 6 }, (_, index) => (
            <EnvironmentCardSkeleton key={`environment-card-skeleton-${index + 1}`} />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filtered.map((environment) => (
            <EnvironmentCard
              key={environment.id}
              environment={environment}
              metricsEnabled
              onTest={onTest}
              onRemove={onRemove}
            />
          ))}
        </div>
      )}

      {!isLoading && filtered.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">
          {environments.length === 0 ? t('emptyMessage') : t('card.noMatches')}
        </p>
      )}
    </div>
  );
}
