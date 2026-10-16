// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconPackage } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent } from '@resources/components/ui/Card';
import { ENVIRONMENT_STATUS, StatusBadge } from '@resources/components/ui/StatusBadge';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatBytes } from '@resources/utils/format';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { deriveEnvironmentStatus } from '../hooks/useEnvironmentActions';
import { useEnvironmentMetrics, useEnvironmentUpdateSummary } from '../hooks/useEnvironments';
import { CHART_COLORS } from '@resources/theme/chart-colors';
import { AgentTlsShield } from './AgentTlsShield';
import { CountTile, MetricSparkline, lastValue } from './EnvironmentCardParts';
import { EnvironmentCardActions } from './EnvironmentCardActions';
import { EnvironmentEstateRow } from './EnvironmentEstateRow';

type EnvironmentCardProps = {
  environment: EnvironmentListItem;
  metricsEnabled: boolean;
  mcharborVersion?: string;
  onTest: (id: string) => void;
  onInstall: (id: string) => void;
  onRemove: (id: string) => void;
};

export function EnvironmentCard({
  environment,
  metricsEnabled,
  mcharborVersion,
  onTest,
  onInstall,
  onRemove,
}: EnvironmentCardProps) {
  const { t } = useTranslation('environments');
  const envStatus = deriveEnvironmentStatus(environment);
  const canLoadDockerStats = metricsEnabled && environment.isActive && envStatus === 'connected';

  const { data: stats, isLoading: metricsLoading } = useEnvironmentMetrics(environment.id, canLoadDockerStats);
  const {
    data: updateSummary,
    isLoading: updatesLoading,
    isError: updatesError,
  } = useEnvironmentUpdateSummary(environment.id, canLoadDockerStats);

  const cpuHistory = stats?.cpuHistory?.map((point) => point.value) ?? [];
  const memoryHistory = stats?.memoryHistory?.map((point) => point.value) ?? [];
  const cpuValue = lastValue(cpuHistory);
  const memoryValue = lastValue(memoryHistory);
  const containers = stats?.containers;
  const endpoint =
    environment.connectionType === 'agent'
      ? (environment.agentHostname ?? t('waitingForAgent'))
      : (environment.socketPath ?? (environment.host ? `${environment.host}:${environment.port ?? ''}` : '-'));
  const primaryVersion = environment.connectionType === 'agent' ? environment.agentVersion : environment.dockerVersion;
  const isLocalRuntime = environment.connectionType === 'socket';
  const controlPlaneVersion = isLocalRuntime ? mcharborVersion : environment.agentVersion;
  const runtimeVersion = environment.connectionType === 'agent' ? environment.dockerVersion : environment.dockerVersion;

  return (
    <Card className="min-h-[350px] transition-colors hover:border-primary/40">
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex min-w-0 items-center gap-2">
              <Link
                to={`/environments/${environment.id}`}
                className="truncate text-base font-semibold text-foreground hover:text-primary hover:underline"
              >
                {environment.name}
              </Link>
              {environment.isDefault && (
                <Badge variant="secondary" className="shrink-0 px-2 py-1 text-[10px]">
                  {t('badges.default')}
                </Badge>
              )}
            </div>
            <div className="flex min-w-0 items-center gap-1.5">
              <p className="truncate font-mono text-xs text-muted-foreground">{endpoint}</p>
              <AgentTlsShield mode={environment.agentTlsMode} />
            </div>
            {environment.agentHostIp ? (
              <p className="truncate font-mono text-xs text-muted-foreground">
                {t('detail.hostIp')}: {environment.agentHostIp}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 flex-col items-center gap-1">
            <StatusBadge status={envStatus} map={ENVIRONMENT_STATUS} className="px-2 py-1 text-[10px]" />
            {controlPlaneVersion && (
              <span className="font-mono text-xs text-muted-foreground">{controlPlaneVersion}</span>
            )}
          </div>
        </div>

        <div className="grid grid-cols-[auto_auto_1fr] items-center gap-3">
          <Badge variant="secondary">{t('platform.docker')}</Badge>
          <Badge variant="outline" className="uppercase">
            {environment.connectionType || '-'}
          </Badge>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {runtimeVersion ?? primaryVersion ?? t('card.versionUnknown')}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <MetricSparkline
            label={t('card.cpu')}
            value={cpuValue === null ? '-' : `${cpuValue.toFixed(0)}%`}
            data={cpuHistory}
            color={CHART_COLORS.cpu}
            icon={<IconCpu className="size-4" />}
          />
          <MetricSparkline
            label={t('card.ram')}
            value={memoryValue === null ? '-' : formatBytes(memoryValue)}
            data={memoryHistory}
            color={CHART_COLORS.memory}
            icon={<IconCpu2 className="size-4" />}
          />
        </div>

        <div className="flex items-center justify-between rounded-lg border border-border/70 bg-background/40 px-3 py-2">
          <span className="text-xs font-medium text-muted-foreground">{t('card.containers')}</span>
          <span className="text-sm font-semibold text-foreground tabular-nums">{containers?.total ?? '-'}</span>
        </div>

        <div className="grid grid-cols-4 gap-2">
          <CountTile label={t('card.running')} value={containers?.running ?? '-'} tone="success" />
          <CountTile label={t('card.stopped')} value={containers?.stopped ?? '-'} tone="muted" />
          <CountTile label={t('card.restarting')} value={containers?.restarting ?? '-'} tone="warning" />
          <CountTile label={t('card.killed')} value={containers?.killed ?? '-'} tone="danger" />
        </div>

        <EnvironmentEstateRow
          environmentId={environment.id}
          enabled={environment.isActive && envStatus === 'connected'}
        />

        <div className="mt-auto flex items-center justify-between rounded-lg border border-border/70 bg-background/40 px-3 py-2">
          <div className="flex min-w-0 items-center gap-2">
            <IconPackage className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate text-xs text-muted-foreground">{t('card.needsUpdates')}</span>
          </div>
          <div className="shrink-0 text-sm font-semibold text-foreground tabular-nums">
            {updatesLoading ? (
              <Spinner size="sm" />
            ) : updatesError ? (
              t('card.unavailable')
            ) : (
              (updateSummary?.available ?? (canLoadDockerStats ? 0 : '-'))
            )}
          </div>
        </div>

        {metricsLoading && canLoadDockerStats && (
          <p className="text-xs text-muted-foreground">{t('card.loadingMetrics')}</p>
        )}
      </CardContent>

      <EnvironmentCardActions
        environmentId={environment.id}
        isAgent={environment.connectionType === 'agent'}
        onTest={onTest}
        onInstall={onInstall}
        onRemove={onRemove}
      />
    </Card>
  );
}
