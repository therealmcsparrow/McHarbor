// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { EnvironmentCard } from './EnvironmentCard';

type EnvironmentCardGridProps = {
  environments: EnvironmentListItem[];
  hasEnvironments: boolean;
  isLoading: boolean;
  mcharborVersion?: string;
  onTest: (id: string) => void;
  onInstall: (id: string) => void;
  onRemove: (id: string) => void;
};

function EnvironmentCardSkeleton() {
  return (
    <div className="mc-glass min-h-[350px] animate-pulse rounded-xl p-4">
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
  hasEnvironments,
  isLoading,
  mcharborVersion,
  onTest,
  onInstall,
  onRemove,
}: EnvironmentCardGridProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="space-y-4">
      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 6 }, (_, index) => (
            <EnvironmentCardSkeleton key={`environment-card-skeleton-${index + 1}`} />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {environments.map((environment) => (
            <EnvironmentCard
              key={environment.id}
              environment={environment}
              metricsEnabled
              mcharborVersion={mcharborVersion}
              onTest={onTest}
              onInstall={onInstall}
              onRemove={onRemove}
            />
          ))}
        </div>
      )}

      {!isLoading && environments.length === 0 && (
        <p className="py-12 text-center text-sm text-muted-foreground">
          {hasEnvironments ? t('card.noMatches') : t('emptyMessage')}
        </p>
      )}
    </div>
  );
}
