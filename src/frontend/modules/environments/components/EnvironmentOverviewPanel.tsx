// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCpu, IconCpu2, IconDatabase, IconServer } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { formatBytes, splitBytes } from '@resources/utils/format';
import type { HostMetrics } from '@core/types/docker';
import type { DashboardStats, EnvironmentInfo } from '../hooks/useEnvironments';
import { AgentInfoCard, ConnectionDetailsCard } from './ConnectionDetailsCard';

type EnvironmentOverviewPanelProps = {
  env: EnvironmentInfo;
  stats?: DashboardStats;
  hostMetrics?: HostMetrics;
  onRegenerateToken: () => void;
  isRegenerating: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function EnvironmentOverviewPanel({
  env,
  stats,
  hostMetrics,
  onRegenerateToken,
  isRegenerating,
  t,
}: EnvironmentOverviewPanelProps) {
  const networkData = (stats?.networkRxHistory ?? []).map((point, index) => ({
    timestamp: point.timestamp,
    rx: point.value,
    tx: stats?.networkTxHistory?.[index]?.value ?? 0,
  }));

  const diskData = (stats?.blockReadHistory ?? []).map((point, index) => ({
    timestamp: point.timestamp,
    read: point.value,
    write: stats?.blockWriteHistory?.[index]?.value ?? 0,
  }));

  return (
    <div className="space-y-6">
      <ConnectionDetailsCard env={env} />

      {env.connectionType === 'agent' && (
        <AgentInfoCard
          env={env}
          onRegenerateToken={onRegenerateToken}
          isRegenerating={isRegenerating}
        />
      )}

      {hostMetrics && (() => {
        const totalMem = splitBytes(hostMetrics.host.memTotal);
        const diskTotal = splitBytes(hostMetrics.disk.total);

        return (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              title={t('detail.cpuCores')}
              value={hostMetrics.host.ncpu}
              unit={t('detail.cores')}
              description={hostMetrics.host.architecture}
              icon={<IconCpu className="h-5 w-5" />}
              accentColor="var(--primary)"
              chart={stats?.cpuHistory?.length ? (
                <StatsAreaChart data={stats.cpuHistory} dataKey="value" xAxisKey="timestamp" color="var(--primary)" formatValue={(value) => `${value.toFixed(1)}%`} compact />
              ) : undefined}
            />
            <StatCard
              title={t('detail.totalMemory')}
              value={totalMem.value}
              unit={totalMem.unit}
              description={hostMetrics.host.os}
              icon={<IconCpu2 className="h-5 w-5" />}
              accentColor="hsl(280 70% 50%)"
              chart={stats?.memoryHistory?.length ? (
                <StatsAreaChart data={stats.memoryHistory} dataKey="value" xAxisKey="timestamp" color="hsl(280 70% 50%)" formatValue={(value) => formatBytes(value)} compact />
              ) : undefined}
            />
            <StatCard
              title={t('detail.dockerVersionStat')}
              value={hostMetrics.host.serverVersion}
              description={hostMetrics.host.hostname}
              icon={<IconServer className="h-5 w-5" />}
              accentColor="hsl(142 70% 45%)"
              chart={networkData.length > 0 ? (
                <StatsAreaChart data={networkData} dataKey="rx" secondaryDataKey="tx" xAxisKey="timestamp" color="hsl(142 70% 45%)" secondaryColor="hsl(30 80% 55%)" formatValue={(value) => formatBytes(value)} label={t('detail.chartRx')} secondaryLabel={t('detail.chartTx')} compact />
              ) : undefined}
            />
            <StatCard
              title={t('detail.diskUsage')}
              value={diskTotal.value}
              unit={diskTotal.unit}
              description={t('detail.imagesVolumes', {
                images: formatBytes(hostMetrics.disk.imagesSize),
                volumes: formatBytes(hostMetrics.disk.volumesSize),
              })}
              icon={<IconDatabase className="h-5 w-5" />}
              accentColor="hsl(210 70% 50%)"
              chart={diskData.length > 0 ? (
                <StatsAreaChart data={diskData} dataKey="read" secondaryDataKey="write" xAxisKey="timestamp" color="hsl(210 70% 50%)" secondaryColor="hsl(0 70% 50%)" formatValue={(value) => formatBytes(value)} label={t('detail.chartRead')} secondaryLabel={t('detail.chartWrite')} compact />
              ) : undefined}
            />
          </div>
        );
      })()}

      {stats?.containers && (
        <div className="grid grid-cols-3 gap-4">
          <div className="rounded-lg border border-border bg-card p-4 text-center">
            <p className="text-2xl font-bold text-green-500">{stats.containers.running}</p>
            <p className="text-xs text-muted-foreground">{t('detail.running')}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4 text-center">
            <p className="text-2xl font-bold text-muted-foreground">{stats.containers.stopped}</p>
            <p className="text-xs text-muted-foreground">{t('detail.stopped')}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4 text-center">
            <p className="text-2xl font-bold text-yellow-500">{stats.containers.paused}</p>
            <p className="text-xs text-muted-foreground">{t('detail.paused')}</p>
          </div>
        </div>
      )}
    </div>
  );
}
