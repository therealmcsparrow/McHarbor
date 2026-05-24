// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconCpu,
  IconCpu2,
  IconExternalLink,
  IconPackage,
  IconRefresh,
  IconTrash,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { ENVIRONMENT_STATUS, StatusBadge } from '@resources/components/ui/StatusBadge';
import { Sparkline } from '@resources/components/Sparkline';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { formatBytes } from '@resources/utils/format';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';
import { deriveEnvironmentStatus } from '../hooks/useEnvironmentActions';
import { useEnvironmentMetrics, useEnvironmentUpdateSummary } from '../hooks/useEnvironments';

type EnvironmentCardProps = {
  environment: EnvironmentListItem;
  metricsEnabled: boolean;
  onTest: (id: string) => void;
  onRemove: (id: string) => void;
};

function lastValue(values: number[]): number | null {
  return values.length > 0 ? values[values.length - 1]! : null;
}

function MetricSparkline({
  label,
  value,
  data,
  color,
  icon,
}: {
  label: string;
  value: string;
  data: number[];
  color: string;
  icon: ReactNode;
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-muted-foreground">{icon}</span>
          <span className="truncate text-xs font-medium text-muted-foreground">{label}</span>
        </div>
        <span className="shrink-0 text-sm font-semibold tabular-nums text-foreground">{value}</span>
      </div>
      <div className="mt-3 h-6">
        <Sparkline
          data={data.length >= 2 ? data : [0, 0]}
          width={160}
          height={24}
          color={color}
          className="w-full"
        />
      </div>
    </div>
  );
}

function CountTile({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone: 'success' | 'muted' | 'warning' | 'danger';
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 px-3 py-2">
      <p
        className={cn(
          'text-lg font-semibold tabular-nums',
          tone === 'success' && 'text-teal-400',
          tone === 'muted' && 'text-muted-foreground',
          tone === 'warning' && 'text-yellow-400',
          tone === 'danger' && 'text-red-400',
        )}
      >
        {value}
      </p>
      <p className="truncate text-[11px] text-muted-foreground">{label}</p>
    </div>
  );
}

export function EnvironmentCard({
  environment,
  metricsEnabled,
  onTest,
  onRemove,
}: EnvironmentCardProps) {
  const { t } = useTranslation('environments');
  const { t: tc } = useTranslation('common');
  const status = deriveEnvironmentStatus(environment);
  const canLoadDockerStats =
    metricsEnabled
    && environment.orchestratorType === 'docker'
    && environment.isActive
    && status === 'connected';

  const { data: stats, isLoading: metricsLoading } = useEnvironmentMetrics(
    environment.id,
    canLoadDockerStats
  );
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
  const endpoint = environment.connectionType === 'agent'
    ? environment.agentHostname ?? t('waitingForAgent')
    : environment.socketPath
      ?? (environment.host ? `${environment.host}:${environment.port ?? ''}` : '-');

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
            <p className="truncate font-mono text-xs text-muted-foreground">{endpoint}</p>
          </div>
          <StatusBadge status={status} map={ENVIRONMENT_STATUS} className="shrink-0 px-2 py-1 text-[10px]" />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={environment.orchestratorType === 'kubernetes' ? 'default' : 'secondary'}>
            {environment.orchestratorType === 'kubernetes' ? t('platform.kubernetes') : t('platform.docker')}
          </Badge>
          <Badge variant="outline" className="uppercase">
            {environment.connectionType || '-'}
          </Badge>
          <span className="truncate text-xs text-muted-foreground">
            {environment.dockerVersion ?? environment.k8sVersion ?? t('card.versionUnknown')}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <MetricSparkline
            label={t('card.cpu')}
            value={cpuValue === null ? '-' : `${cpuValue.toFixed(0)}%`}
            data={cpuHistory}
            color="hsl(142 71% 45%)"
            icon={<IconCpu className="size-4" />}
          />
          <MetricSparkline
            label={t('card.ram')}
            value={memoryValue === null ? '-' : formatBytes(memoryValue)}
            data={memoryHistory}
            color="hsl(217 91% 60%)"
            icon={<IconCpu2 className="size-4" />}
          />
        </div>

        <div className="flex items-center justify-between rounded-lg border border-border/70 bg-background/40 px-3 py-2">
          <span className="text-xs font-medium text-muted-foreground">{t('card.containers')}</span>
          <span className="text-sm font-semibold tabular-nums text-foreground">{containers?.total ?? '-'}</span>
        </div>

        <div className="grid grid-cols-4 gap-2">
          <CountTile label={t('card.running')} value={containers?.running ?? '-'} tone="success" />
          <CountTile label={t('card.stopped')} value={containers?.stopped ?? '-'} tone="muted" />
          <CountTile label={t('card.restarting')} value={containers?.restarting ?? '-'} tone="warning" />
          <CountTile label={t('card.killed')} value={containers?.killed ?? '-'} tone="danger" />
        </div>

        <div className="mt-auto flex items-center justify-between rounded-lg border border-border/70 bg-background/40 px-3 py-2">
          <div className="flex min-w-0 items-center gap-2">
            <IconPackage className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate text-xs text-muted-foreground">{t('card.needsUpdates')}</span>
          </div>
          <div className="shrink-0 text-sm font-semibold tabular-nums text-foreground">
            {updatesLoading ? (
              <Spinner size="sm" />
            ) : updatesError ? (
              t('card.unavailable')
            ) : (
              updateSummary?.available ?? (canLoadDockerStats ? 0 : '-')
            )}
          </div>
        </div>

        {metricsLoading && canLoadDockerStats && (
          <p className="text-xs text-muted-foreground">{t('card.loadingMetrics')}</p>
        )}
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <Button asChild variant="ghost" size="icon-sm" aria-label={t('card.open')}>
          <Link to={`/environments/${environment.id}`}>
            <IconExternalLink className="h-4 w-4" />
          </Link>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('testConnection')}
          title={t('testConnection')}
          onClick={() => onTest(environment.id)}
        >
          <IconRefresh className="h-4 w-4" />
        </Button>
        <div className="flex-1" />
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={tc('actions.remove')}
          title={tc('actions.remove')}
          onClick={() => onRemove(environment.id)}
        >
          <IconTrash className="h-4 w-4 text-destructive" />
        </Button>
      </CardFooter>
    </Card>
  );
}
