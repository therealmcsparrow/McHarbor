// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconAlertTriangle,
  IconCpu,
  IconDatabase,
  IconDeviceDesktop,
  IconDeviceSdCard,
  IconGauge,
  IconRefresh,
  IconTrash,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatCard } from '@resources/components/StatCard';
import type { HostMetrics, PruneType } from '@core/types/docker';
import { useHostPrune } from '@resources/hooks/useHostMetrics';
import { useEnvironmentHostMetrics, type EnvironmentInfo } from '../hooks/useEnvironments';
import { EnvironmentHostLogPreview } from './EnvironmentHostLogPreview';
import { formatBytes, formatUptime, splitBytes } from '@resources/utils/format';

const PRUNE_TYPES: PruneType[] = ['system', 'builder', 'volumes', 'images', 'containers', 'networks'];

type EnvironmentHostTabProps = {
  envId: string;
  env?: EnvironmentInfo;
};

export function EnvironmentHostTab({ envId, env }: EnvironmentHostTabProps) {
  const { t } = useTranslation('environments');
  const { data: metrics, isLoading, isError, refetch, isFetching, dataUpdatedAt } = useEnvironmentHostMetrics(envId);
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
          <CardDescription>{t('detail.host.loadError')}</CardDescription>
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
            <CardDescription>
              {t('detail.host.description', { hostname: metrics.host.hostname })}
            </CardDescription>
          </div>
          <RefreshIndicator updatedAt={dataUpdatedAt} isFetching={isFetching} t={t} />
        </CardHeader>
        <CardContent className="space-y-6">
          {metrics.agentLimit && (
            <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('detail.host.agentLimitedNotice')}</span>
            </div>
          )}

          <HostStatGrid metrics={metrics} t={t} />

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <DockerDiskPanel metrics={metrics} t={t} />
            <HostFSPanel metrics={metrics} t={t} />
          </div>

          <PrunePanel
            onPrune={setPending}
            busy={prune.isPending}
            agentLimited={metrics.agentLimit}
            t={t}
          />
        </CardContent>
      </Card>

      {env && <EnvironmentHostLogPreview env={env} />}

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => !open && setPending(null)}
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

function RefreshIndicator({
  updatedAt,
  isFetching,
  t,
}: {
  updatedAt: number;
  isFetching: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, []);

  const seconds = Math.max(0, Math.round((now - updatedAt) / 1000));
  return (
    <div className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
      <IconRefresh className={isFetching ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'} />
      <span>
        {t('detail.host.refreshed', {
          value: t('detail.host.secondsAgo', { count: seconds, defaultValue: '{{count}}s ago' }),
        })}
      </span>
    </div>
  );
}

function HostStatGrid({
  metrics,
  t,
}: {
  metrics: HostMetrics;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const { host } = metrics;
  const live = !metrics.agentLimit;

  const cpuDescription = useMemo(() => {
    const parts: string[] = [];
    if (live && host.load1 > 0) parts.push(t('detail.host.stats.load', { value: host.load1.toFixed(2) }));
    if (host.ncpu > 0) parts.push(t('detail.host.stats.cores', { count: host.ncpu }));
    return parts.length > 0 ? parts.join(' · ') : t('detail.host.stats.unavailable');
  }, [host.load1, host.ncpu, t, live]);

  const memSplit = useMemo(() => splitBytes(host.memUsed), [host.memUsed]);
  const memTotalSplit = useMemo(() => splitBytes(host.memTotal), [host.memTotal]);

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <StatCard
        title={t('detail.host.stats.cpu')}
        value={live ? host.cpuPercent.toFixed(1) : '—'}
        unit={live ? '%' : ''}
        description={cpuDescription}
        icon={<IconCpu className="h-5 w-5" />}
      />
      <StatCard
        title={t('detail.host.stats.memory')}
        value={live ? parseFloat(memSplit.value).toFixed(1) : '—'}
        unit={live ? memSplit.unit : ''}
        description={
          live
            ? `${parseFloat(memTotalSplit.value).toFixed(1)} ${memTotalSplit.unit} · ${host.memPercent.toFixed(0)}%`
            : t('detail.host.stats.unavailable')
        }
        icon={<IconDeviceSdCard className="h-5 w-5" />}
      />
      <StatCard
        title={t('detail.host.stats.uptime')}
        value={formatUptime(host.uptime)}
        description={`${host.os} · ${host.architecture}`}
        icon={<IconDeviceDesktop className="h-5 w-5" />}
      />
      <StatCard
        title={t('detail.host.stats.dockerVersion')}
        value={host.serverVersion}
        description={host.kernelVersion}
        icon={<IconGauge className="h-5 w-5" />}
      />
    </div>
  );
}

function DockerDiskPanel({
  metrics,
  t,
}: {
  metrics: HostMetrics;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const { disk } = metrics;
  const split = useMemo(() => splitBytes(disk.total), [disk.total]);

  const rows = useMemo(() => {
    const base = [
      { key: 'images', label: t('detail.host.disk.images'), value: disk.imagesSize },
      { key: 'volumes', label: t('detail.host.disk.volumes'), value: disk.volumesSize },
      { key: 'containers', label: t('detail.host.disk.containers'), value: disk.containersSize },
      { key: 'buildCache', label: t('detail.host.disk.buildCache'), value: disk.buildCacheSize },
    ];
    return base.sort((a, b) => b.value - a.value);
  }, [disk, t]);

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconDatabase className="h-4 w-4 text-muted-foreground" />
          {t('detail.host.disk.title')}
        </div>
        <div className="text-sm tabular-nums text-foreground">
          {parseFloat(split.value).toFixed(1)} <span className="text-xs text-muted-foreground">{split.unit}</span>
        </div>
      </div>
      <ul className="space-y-1.5 text-xs">
        {rows.map((r) => {
          const s = splitBytes(r.value);
          const pct = disk.total > 0 ? (r.value / disk.total) * 100 : 0;
          return (
            <li key={r.key} className="space-y-0.5">
              <div className="flex items-center justify-between text-muted-foreground">
                <span>{r.label}</span>
                <span className="tabular-nums text-foreground">
                  {parseFloat(s.value).toFixed(1)} {s.unit} · {pct.toFixed(1)}%
                </span>
              </div>
              <div className="h-1 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full bg-primary/60"
                  style={{ width: `${Math.min(100, pct)}%` }}
                />
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function HostFSPanel({
  metrics,
  t,
}: {
  metrics: HostMetrics;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const { hostFs } = metrics;
  const available = hostFs.total > 0;

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconDeviceDesktop className="h-4 w-4 text-muted-foreground" />
          {t('detail.host.hostFs.title')}
        </div>
        {available && (
          <div className="text-sm tabular-nums text-foreground">
            {formatBytes(hostFs.used)}{' '}
            <span className="text-xs text-muted-foreground">/ {formatBytes(hostFs.total)}</span>
          </div>
        )}
      </div>
      {available ? (
        <>
          <div className="mb-2 text-xs text-muted-foreground">{t('detail.host.hostFs.path', { path: hostFs.path })}</div>
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className={hostFs.percent > 90 ? 'h-full bg-red-500' : 'h-full bg-primary'}
              style={{ width: `${Math.min(100, hostFs.percent)}%` }}
            />
          </div>
          <div className="mt-1 text-right text-xs tabular-nums text-muted-foreground">
            {hostFs.percent.toFixed(1)}%
          </div>
        </>
      ) : (
        <div className="text-xs text-muted-foreground">{t('detail.host.hostFs.unavailable')}</div>
      )}
    </div>
  );
}

function PrunePanel({
  onPrune,
  busy,
  agentLimited,
  t,
}: {
  onPrune: (target: { type: PruneType; volumes: boolean }) => void;
  busy: boolean;
  agentLimited: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium text-foreground">
        <IconTrash className="h-4 w-4 text-muted-foreground" />
        {t('detail.host.prune.title')}
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        {agentLimited ? t('detail.host.prune.agentLimited') : t('detail.host.prune.description')}
      </p>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {PRUNE_TYPES.map((type) => {
          const destructive = type === 'volumes';
          return (
            <Button
              key={type}
              variant="outline"
              size="sm"
              disabled={busy || agentLimited}
              onClick={() => onPrune({ type, volumes: false })}
              className="justify-start"
            >
              <IconTrash className="mr-2 h-3.5 w-3.5 text-destructive" />
              {t(`detail.host.prune.${type}.label`)}
              {destructive && (
                <span className="ml-auto text-[10px] uppercase tracking-wide text-destructive">
                  {t('detail.host.prune.destructiveTag')}
                </span>
              )}
            </Button>
          );
        })}
        <Button
          variant="destructive"
          size="sm"
          disabled={busy || agentLimited}
          onClick={() => onPrune({ type: 'system', volumes: true })}
          className="justify-start"
        >
          <IconTrash className="mr-2 h-3.5 w-3.5" />
          {t('detail.host.prune.system.withVolumesLabel')}
          <span className="ml-auto text-[10px] uppercase tracking-wide opacity-80">
            {t('detail.host.prune.destructiveTag')}
          </span>
        </Button>
      </div>
    </div>
  );
}
