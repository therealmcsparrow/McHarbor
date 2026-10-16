// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import type { PruneType } from '@core/types/docker';
import { useHostPrune } from '@resources/hooks/useHostMetrics';
import { useEnvironmentHostMetrics, type EnvironmentInfo } from '../hooks/useEnvironments';
import { EnvironmentHostLogPreview } from './EnvironmentHostLogPreview';
import { CapacityOverview } from '@resources/components/capacity/CapacityOverview';
import { IdleResources } from '@resources/components/capacity/IdleResources';
import { RefreshIndicator, HostStatGrid } from './EnvironmentHostStats';
import { DockerDiskPanel, HostFSPanel, PrunePanel } from './EnvironmentHostPanels';

type EnvironmentHostTabProps = {
  envId: string;
  env?: EnvironmentInfo;
};

export function EnvironmentHostTab({ envId, env }: EnvironmentHostTabProps) {
  const { t } = useTranslation('environments');
  const {
    data: metrics,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
    dataUpdatedAt,
  } = useEnvironmentHostMetrics(envId);
  const prune = useHostPrune();
  const [pending, setPending] = useState<{ type: PruneType; volumes: boolean } | null>(null);

  useEffect(() => {
    if (!envId) {
      return;
    }
    void refetch();
  }, [envId, refetch]);

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (isError || !metrics) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('detail.host.title')}</CardTitle>
          <CardDescription>{[t('detail.host.loadError'), error?.message].filter(Boolean).join(' — ')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            <IconRefresh className="mr-1.5 h-3.5 w-3.5" />
            {t('detail.host.retry')}
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle>{t('detail.host.title')}</CardTitle>
            <CardDescription>{t('detail.host.description', { hostname: metrics.host.hostname })}</CardDescription>
          </div>
          <RefreshIndicator updatedAt={dataUpdatedAt} isFetching={isFetching} t={t} />
        </CardHeader>
        <CardContent className="space-y-6">
          {metrics.agentLimit && (
            <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('detail.host.agentLimitedNotice')}</span>
            </div>
          )}

          <HostStatGrid metrics={metrics} t={t} />

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <DockerDiskPanel metrics={metrics} t={t} />
            <HostFSPanel metrics={metrics} t={t} />
          </div>

          <PrunePanel onPrune={setPending} busy={prune.isPending} agentLimited={metrics.agentLimit} t={t} />
        </CardContent>
      </Card>

      <CapacityOverview envId={envId} />
      <IdleResources envId={envId} />

      {env && <EnvironmentHostLogPreview env={env} />}

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(next) => !next && setPending(null)}
        title={pending ? t(`detail.host.prune.${pending.type}.title`) : ''}
        description={
          pending && pending.volumes
            ? t(`detail.host.prune.${pending.type}.withVolumesDescription`)
            : pending
              ? t(`detail.host.prune.${pending.type}.description`)
              : ''
        }
        confirmLabel={t('detail.host.prune.confirm')}
        onConfirm={() => {
          if (pending) {
            prune.mutate(
              { type: pending.type, volumes: pending.volumes, confirm: true },
              { onSuccess: () => setPending(null) },
            );
          }
        }}
        loading={prune.isPending}
        variant="destructive"
      />
    </div>
  );
}
