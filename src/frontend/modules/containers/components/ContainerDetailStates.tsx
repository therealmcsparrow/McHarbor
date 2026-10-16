// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useTranslation } from 'react-i18next';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';

export function ContainerDetailLoading() {
  const { t } = useTranslation('containers');
  return (
    <div className="flex h-full flex-col gap-4" aria-label={t('detail.loading')}>
      <CardSkeleton lines={2} className="min-h-24" />
      <div className="grid gap-4 lg:grid-cols-3">
        <CardSkeleton lines={4} />
        <CardSkeleton lines={4} />
        <CardSkeleton lines={4} />
      </div>
      <CardSkeleton lines={8} className="min-h-64" />
    </div>
  );
}

export function ContainerDetailNotFound({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation('containers');
  return (
    <div>
      <ErrorState
        variant="page"
        title={t('detail.notFoundTitle')}
        description={t('detail.notFoundDescription')}
        primaryAction={{
          label: t('actions.backToContainers'),
          onClick: onBack,
        }}
      />
    </div>
  );
}
