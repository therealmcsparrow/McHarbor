// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconNetwork, IconDatabase } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { CHART_COLORS } from '@resources/theme/chart-colors';
import { formatDateCustom } from '@resources/utils/format';
import { formatBytes, splitBytes, usedPercent } from '../format';
import { useStatsHistory, type StatsPoint } from '../hooks/useStatsHistory';
import { useVMStatsHistory } from '../hooks/useVMs';
import { Section } from './DetailSection';
import type { Stats, VM } from '../types';

type MetricsProps = {
  vm: VM;
  stats?: Stats;
  unavailable?: boolean;
};

function chartData(points: StatsPoint[], pick: (point: StatsPoint) => number) {
  return points.map((point) => ({
    timestamp: formatDateCustom(point.at, { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    value: pick(point),
  }));
}

export function VMMetricsSection({ vm, stats, unavailable }: MetricsProps) {
  const { t } = useTranslation('vm');
  const running = vm.state === 'running';
  const memMax = stats?.memoryMaxBytes || vm.memoryMb * 1024 * 1024;
  const { data: storedHistory } = useVMStatsHistory(vm.id, running);
  const points = useStatsHistory(running ? stats : undefined, memMax, running ? storedHistory : undefined);

  const series = useMemo(
    () => ({
      cpu: chartData(points, (point) => point.cpuPercent),
      memory: chartData(points, (point) => point.memoryPercent),
      disk: chartData(points, (point) => point.diskReadRate + point.diskWriteRate),
      net: chartData(points, (point) => point.netRxRate + point.netTxRate),
    }),
    [points],
  );

  if (!running || unavailable || !stats) {
    return (
      <Section title={t('detail.overview.metrics')}>
        <p className="text-sm text-muted-foreground">
          {unavailable && running ? t('detail.overview.metricsUnavailable') : t('detail.overview.metricsIdle')}
        </p>
      </Section>
    );
  }

  const hasHistory = points.length > 1;
  const memPercent = usedPercent(stats.memoryBytes, memMax);
  const memory = splitBytes(stats.memoryBytes);
  const diskTotal = splitBytes(stats.diskReadBytes + stats.diskWriteBytes);
  const netTotal = splitBytes(stats.netRxBytes + stats.netTxBytes);

  const chart = (data: Array<Record<string, unknown>>, color: string, format: (value: number) => string) =>
    hasHistory ? (
      <StatsAreaChart data={data} dataKey="value" xAxisKey="timestamp" color={color} formatValue={format} compact />
    ) : undefined;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          title={t('detail.overview.cpuUsage')}
          value={stats.cpuPercent.toFixed(1)}
          unit="%"
          icon={<IconCpu className="h-5 w-5" />}
          accentColor={CHART_COLORS.cpu}
          chart={chart(series.cpu, CHART_COLORS.cpu, (value) => `${value.toFixed(1)}%`)}
        />
        <StatCard
          title={t('detail.overview.memoryUsage')}
          value={memory.value}
          unit={memory.unit}
          description={`${memPercent}% / ${formatBytes(memMax)}`}
          icon={<IconCpu2 className="h-5 w-5" />}
          accentColor={CHART_COLORS.memory}
          chart={chart(series.memory, CHART_COLORS.memory, (value) => `${value.toFixed(1)}%`)}
        />
        <StatCard
          title={t('detail.overview.diskIo')}
          value={diskTotal.value}
          unit={diskTotal.unit}
          description={`${formatBytes(stats.diskReadBytes)} / ${formatBytes(stats.diskWriteBytes)}`}
          icon={<IconDatabase className="h-5 w-5" />}
          accentColor={CHART_COLORS.diskRead}
          chart={chart(series.disk, CHART_COLORS.diskRead, (value) => `${formatBytes(value)}/s`)}
        />
        <StatCard
          title={t('detail.overview.networkIo')}
          value={netTotal.value}
          unit={netTotal.unit}
          description={`${formatBytes(stats.netRxBytes)} / ${formatBytes(stats.netTxBytes)}`}
          icon={<IconNetwork className="h-5 w-5" />}
          accentColor={CHART_COLORS.networkRx}
          chart={chart(series.net, CHART_COLORS.networkRx, (value) => `${formatBytes(value)}/s`)}
        />
      </div>
    </div>
  );
}
