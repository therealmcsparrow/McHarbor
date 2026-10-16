// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { CardSkeleton } from '@resources/components/ui/Skeleton';

export function EnvironmentDetailSkeleton() {
  const { t } = useTranslation('environments');

  return (
    <div className="flex h-full flex-col gap-4" aria-label={t('common.loading', { ns: 'common' })}>
      <CardSkeleton lines={2} className="min-h-24" />
      <div className="grid gap-4 lg:grid-cols-4">
        <CardSkeleton lines={3} />
        <CardSkeleton lines={3} />
        <CardSkeleton lines={3} />
        <CardSkeleton lines={3} />
      </div>
      <CardSkeleton lines={6} className="min-h-48" />
    </div>
  );
}
