// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDatabase, IconFolder, IconHome, IconInfoCircle } from '@tabler/icons-react';
import { Card } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Tabs, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { formatBytes } from '../format';
import { usePermission } from '@core/auth/usePermission';
import { COMPUTE_PERM } from '../permissions';
import { usePools, useRefreshPool } from '../hooks/useCompute';
import { PoolCapacityBar } from '../components/PoolCapacityBar';
import { PoolFileBrowser } from '../components/PoolFileBrowser';
import { PoolDetailHeader } from './PoolDetailHeader';

type PoolDetailPageProps = {
  poolId: string;
  environments: { id: string; name: string }[];
  onBack: () => void;
};

/**
 * One pool: what it is, and what is on it.
 *
 * The listing is reused rather than refetched per id — the pools query is
 * already live behind the list this page was opened from, and a pool row is
 * small enough that a dedicated read would only add a spinner.
 */
export function PoolDetailPage({ poolId, environments, onBack }: PoolDetailPageProps) {
  const { t } = useTranslation('compute');
  const [tab, setTab] = useState<'overview' | 'files'>('overview');

  const { data, isLoading, isError, refetch } = usePools();
  const refresh = useRefreshPool();
  const canBrowse = usePermission(COMPUTE_PERM.POOLS_BROWSE);

  const pool = (data ?? []).find((candidate) => candidate.id === poolId);

  // Claiming the header slot puts the breadcrumb in the topbar and hands the
  // shell's padding back to this page, matching every core detail page.
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  if (!pool) {
    return (
      <EmptyState
        icon={<IconDatabase className="size-6" />}
        title={t('pools.detail.missing.title')}
        description={t('pools.detail.missing.description')}
        primaryAction={{ label: t('pools.detail.back'), onClick: onBack }}
      />
    );
  }

  const hostName = environments.find((env) => env.id === pool.environmentId)?.name ?? pool.environmentId;
  // lvm keeps its volumes as block devices, so there is no tree to walk. The
  // tab says that rather than showing an empty listing that reads as 'empty'.
  const browsable = pool.backend !== 'lvm';

  return (
    <div className="flex h-full min-h-0 flex-col">
      <HeaderBreadcrumb
        crumbs={[
          { to: '/dashboard', label: <IconHome className="size-3" /> },
          { label: t('pools.title'), onClick: onBack },
          { label: pool.name },
        ]}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-visible">
        <Card className="flex-none overflow-visible">
          <PoolDetailHeader
            pool={pool}
            hostName={hostName}
            onRefresh={() => refresh.mutate(pool.id)}
            refreshing={refresh.isPending}
          />

          <Tabs className="px-5 py-3" value={tab} onValueChange={(next) => setTab(next as 'overview' | 'files')}>
            <TabsList>
              <TabsTrigger value="overview">
                <IconInfoCircle className="size-4" />
                {t('pools.detail.tabs.overview')}
              </TabsTrigger>
              <TabsTrigger value="files">
                <IconFolder className="size-4" />
                {t('pools.detail.tabs.files')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </Card>

        <div
          id="pool-detail-panel"
          role="tabpanel"
          aria-labelledby={`pool-tab-${tab}`}
          tabIndex={0}
          className="flex min-h-0 flex-col"
        >
          {tab === 'overview' && (
            <div className="mc-glass rounded-lg p-6">
              <PoolCapacityBar pool={pool} />
              <dl className="mt-5 grid gap-x-8 gap-y-3 sm:grid-cols-2">
                <InfoRow label={t('pools.columns.host')}>{hostName}</InfoRow>
                <InfoRow label={t('pools.columns.backend')}>{t(`pools.backend.${pool.backend}`)}</InfoRow>
                <InfoRow label={t('pools.columns.target')}>{pool.target}</InfoRow>
                <InfoRow label={t('pools.columns.capacity')}>{formatBytes(pool.capacityBytes)}</InfoRow>
                <InfoRow label={t('pools.columns.used')}>{formatBytes(pool.usedBytes)}</InfoRow>
                <InfoRow label={t('pools.columns.allocated')}>{formatBytes(pool.allocatedBytes)}</InfoRow>
                <InfoRow label={t('pools.columns.volumes')}>{String(pool.volumeCount)}</InfoRow>
                <InfoRow label={t('pools.detail.instant')}>
                  {pool.instant ? t('pools.detail.instantYes') : t('pools.detail.instantNo')}
                </InfoRow>
              </dl>
            </div>
          )}

          {tab === 'files' && !browsable && (
            <EmptyState
              icon={<IconFolder className="size-6" />}
              title={t('pools.files.notBrowsable.title')}
              description={t('pools.files.notBrowsable.description')}
            />
          )}

          {tab === 'files' && browsable && !canBrowse && (
            <EmptyState
              icon={<IconFolder className="size-6" />}
              title={t('pools.files.denied.title')}
              description={t('pools.files.denied.description')}
            />
          )}

          {tab === 'files' && browsable && canBrowse && <PoolFileBrowser pool={pool} />}
        </div>
      </div>
    </div>
  );
}
