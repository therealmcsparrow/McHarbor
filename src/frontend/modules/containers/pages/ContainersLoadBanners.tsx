// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { DockableBanner } from '@resources/layout/DockableBanner';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { DegradedStateBanner } from '@resources/components/DegradedStateBanner';

interface ContainersLoadBannersProps {
  error: Error | null;
  failureCount: number;
  updatedAt: number;
  onRetry: () => void;
}

/** Banners dock rather than disappear; the chip that replaces them lives in the filter band. */
export function ContainersLoadBanners({ error, failureCount, updatedAt, onRetry }: ContainersLoadBannersProps) {
  const { t } = useTranslation('containers');
  return (
    <>
      {error ? (
        <DockableBanner id="containers-load-error" label={t('common:degradedState.chipError')} tone="danger" bare>
          <ErrorState variant="banner" error={error} onRetry={onRetry} />
        </DockableBanner>
      ) : null}
      {(error || failureCount > 0) && (
        <DockableBanner id="containers-degraded" label={t('common:degradedState.chipDegraded')} tone="warning" bare>
          <DegradedStateBanner error={error} failureCount={failureCount} updatedAt={updatedAt} onRetry={onRetry} />
        </DockableBanner>
      )}
    </>
  );
}

export function ContainersLoadFailed({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const { t } = useTranslation('containers');
  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader title={t('title')} description={t('description', { count: 0 })} />
      <ErrorState variant="page" className="flex-1" error={error} onRetry={onRetry} />
    </div>
  );
}
