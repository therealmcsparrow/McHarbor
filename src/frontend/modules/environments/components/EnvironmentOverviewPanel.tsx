// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconActivity, IconCpu, IconCpu2, IconDatabase, IconNetwork } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { formatBytes, splitBytes } from '@resources/utils/format';
import type { HostMetrics } from '@core/types/docker';
import type { DashboardStats, EnvironmentInfo } from '../hooks/useEnvironments';
import { AgentInfoCard, ConnectionDetailsCard } from './ConnectionDetailsCard';
import { DuplicateEnvironmentNotice } from './DuplicateEnvironmentNotice';
import { AgentLogCard } from './AgentLogCard';
import { AgentHealthCard } from './AgentHealthCard';
import { AgentUptimeCard } from './AgentUptimeCard';
import { StatsPathNotice } from './StatsPathNotice';
import { useAgentHealthHistory } from '../hooks/useAgentHealthHistory';
import { CHART_COLORS } from '@resources/theme/chart-colors';

type EnvironmentOverviewPanelProps = {
  env: EnvironmentInfo;
  stats?: DashboardStats;
  hostMetrics?: HostMetrics;
  onRegenerateToken: () => void;
  isRegenerating: boolean;
  onMigrate?: () => void;
  onCancelMigration?: () => void;
  cancelMigrationPending?: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
};

export function EnvironmentOverviewPanel({
  env,
  stats,
  hostMetrics,
  onRegenerateToken,
  isRegenerating,
  onMigrate,
  onCancelMigration,
  cancelMigrationPending,
  t,
}: EnvironmentOverviewPanelProps) {
  // Only agent environments have a keepalive to sample, so the query stays
  // disabled for a local or TCP environment rather than 404ing on every poll.
  const isAgent = env.connectionType === 'agent';
  const { data: agentHealth, isLoading: agentHealthLoading } = useAgentHealthHistory(env.id, isAgent);

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

  // The server sends bytes per second over the host's physical NICs and disks.
  const perSecond = (value: number) => `${formatBytes(value)}/s`;
  const latestRx = networkData.at(-1)?.rx ?? 0;
  const latestTx = networkData.at(-1)?.tx ?? 0;
  const networkNow = splitBytes(latestRx + latestTx);
  const networkSplit = `${perSecond(latestRx)} ↓ · ${perSecond(latestTx)} ↑`;
  const latestRead = diskData.at(-1)?.read ?? 0;
  const latestWrite = diskData.at(-1)?.write ?? 0;
  const diskNow = splitBytes(latestRead + latestWrite);

  return (
    <div className="space-y-6">
      <DuplicateEnvironmentNotice env={env} />

      <ConnectionDetailsCard
        env={env}
        onMigrate={onMigrate}
        onCancelMigration={onCancelMigration}
        cancelPending={cancelMigrationPending}
      />

      {isAgent && <AgentInfoCard env={env} onRegenerateToken={onRegenerateToken} isRegenerating={isRegenerating} />}

      <StatsPathNotice envId={env.id} />

      {hostMetrics &&
        (() => {
          const totalMem = splitBytes(hostMetrics.host.memTotal);

          return (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <StatCard
                title={t('detail.cpuCores')}
                value={hostMetrics.host.ncpu}
                unit={t('detail.cores')}
                description={hostMetrics.host.architecture}
                icon={<IconCpu className="h-5 w-5" />}
                // CHART_COLORS.cpu, not var(--primary) — the memory and
                // network tiles beside it already use their metric colour,
                // so CPU was the only one whose colour changed depending on
                // which surface you were looking at.
                accentColor={CHART_COLORS.cpu}
                chart={
                  stats?.cpuHistory?.length ? (
                    <StatsAreaChart
                      data={stats.cpuHistory}
                      dataKey="value"
                      xAxisKey="timestamp"
                      color={CHART_COLORS.cpu}
                      formatValue={(value) => `${value.toFixed(1)}%`}
                      compact
                    />
                  ) : undefined
                }
              />
              <StatCard
                title={t('detail.totalMemory')}
                value={totalMem.value}
                unit={totalMem.unit}
                description={hostMetrics.host.os}
                icon={<IconCpu2 className="h-5 w-5" />}
                accentColor={CHART_COLORS.memory}
                chart={
                  stats?.memoryHistory?.length ? (
                    <StatsAreaChart
                      data={stats.memoryHistory}
                      dataKey="value"
                      xAxisKey="timestamp"
                      color={CHART_COLORS.memory}
                      formatValue={(value) => formatBytes(value)}
                      compact
                    />
                  ) : undefined
                }
              />
              <StatCard
                title={t('detail.network')}
                value={networkData.length > 0 ? networkNow.value : '—'}
                unit={networkData.length > 0 ? `${networkNow.unit}/s` : undefined}
                description={networkData.length > 0 ? networkSplit : t('detail.networkUnavailable')}
                icon={<IconNetwork className="h-5 w-5" />}
                accentColor={CHART_COLORS.networkRx}
                chart={
                  networkData.length > 0 ? (
                    <StatsAreaChart
                      data={networkData}
                      dataKey="rx"
                      secondaryDataKey="tx"
                      xAxisKey="timestamp"
                      color={CHART_COLORS.networkRx}
                      secondaryColor={CHART_COLORS.networkTx}
                      formatValue={perSecond}
                      label={t('detail.chartRx')}
                      secondaryLabel={t('detail.chartTx')}
                      compact
                    />
                  ) : undefined
                }
              />
              <StatCard
                title={t('detail.diskIo')}
                value={diskData.length > 0 ? diskNow.value : '—'}
                unit={diskData.length > 0 ? `${diskNow.unit}/s` : undefined}
                description={
                  diskData.length > 0
                    ? t('detail.diskIoSplit', { read: perSecond(latestRead), write: perSecond(latestWrite) })
                    : t('detail.diskIoUnavailable')
                }
                icon={<IconDatabase className="h-5 w-5" />}
                accentColor={CHART_COLORS.diskRead}
                chart={
                  diskData.length > 0 ? (
                    <StatsAreaChart
                      data={diskData}
                      dataKey="read"
                      secondaryDataKey="write"
                      xAxisKey="timestamp"
                      color={CHART_COLORS.diskRead}
                      secondaryColor={CHART_COLORS.diskWrite}
                      formatValue={perSecond}
                      label={t('detail.chartRead')}
                      secondaryLabel={t('detail.chartWrite')}
                      compact
                    />
                  ) : undefined
                }
              />
            </div>
          );
        })()}

      {isAgent && (
        <>
          <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
            <AgentHealthCard history={agentHealth} loading={agentHealthLoading} />
            <AgentUptimeCard history={agentHealth} loading={agentHealthLoading} />
          </div>
          <AgentLogCard envId={env.id} />
        </>
      )}

      {hostMetrics && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-border bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
          <IconActivity className="size-4" />
          <span>
            {t('detail.engineFooter', {
              version: hostMetrics.host.serverVersion,
              hostname: hostMetrics.host.hostname,
            })}
          </span>
        </div>
      )}
    </div>
  );
}
