// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTopologyStar3 } from '@tabler/icons-react';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { CardSkeleton, TableSkeleton } from '@resources/components/ui/Skeleton';
import { usePluginNavigate } from '../nav-context';
import type { KubernetesSummary } from '../types';

/**
 * No cluster answered. The reason code carries the fix, so this is an empty
 * state with a way forward rather than a red error — the cluster is not
 * broken, it is unconfigured.
 */
export function Disconnected({ summary }: { summary: KubernetesSummary }) {
  const { t } = useTranslation('kubernetes');
  const navigate = usePluginNavigate();
  const fallback = t('errors.verifyCredentials');
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: reason codes come from the backend and a new one must degrade to the generic hint
  const reasonText = summary.reason ? t(`errors.reason.${summary.reason}`, { defaultValue: fallback }) : fallback;

  return (
    <EmptyState
      variant="info"
      icon={<IconTopologyStar3 className="size-6" />}
      title={t('errors.unavailable')}
      description={reasonText}
      primaryAction={{
        label: t('actions.configure'),
        onClick: () => navigate('clusters'),
      }}
    />
  );
}

export function Loading() {
  return (
    <div className="space-y-4">
      <CardSkeleton lines={2} />
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <CardSkeleton lines={2} />
        <CardSkeleton lines={2} />
        <CardSkeleton lines={2} />
        <CardSkeleton lines={2} />
      </div>
      <TableSkeleton rows={6} columns={5} />
    </div>
  );
}
