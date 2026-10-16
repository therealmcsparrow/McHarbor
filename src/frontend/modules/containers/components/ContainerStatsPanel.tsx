// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconArrowBarDown, IconArrowBarUp, IconDeviceFloppy } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatBytes, splitBytes } from '@resources/utils/format';
import { useContainerStats, type UseContainerStatsResult } from '../hooks/useContainerStats';
import { CHART_COLORS } from '@resources/theme/chart-colors';
import { ShowHistoryButton } from './ShowHistoryButton';
import { buildStatsChartData } from './stats-chart-data';
import type { HistoryMetric } from '@resources/components/history-metrics';

type ContainerStatsPanelProps = {
  containerId: string;
  stats?: UseContainerStatsResult;
  /** Metric the history panel below is currently showing, if any. */
  activeMetric?: HistoryMetric;
  /** Omit to render the tiles without their history buttons. */
  onShowHistory?: (metric: HistoryMetric) => void;
};

export function ContainerStatsPanel({ containerId, stats, activeMetric, onShowHistory }: ContainerStatsPanelProps) {
  const { t } = useTranslation('containers');
  const ownStats = useContainerStats(containerId, !stats);
  const { current, history: historyData, connected } = stats ?? ownStats;

  if (!connected && !current) {
    return (
      <div className="mc-glass rounded-lg p-6 lg:col-span-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner size="sm" />
          {t('stats.connecting')}
        </div>
      </div>
    );
  }

  const { cpuData, memData, netData, blockData } = buildStatsChartData(historyData);

  const hasHistory = historyData.length > 1;
  const mem = splitBytes(current?.memUsage ?? 0);
  const netRx = splitBytes(current?.netRx ?? 0);
  const blockRead = splitBytes(current?.blockRead ?? 0);

  // undefined without onShowHistory, which keeps StatCard's footer strip out of
  // the DOM entirely rather than rendering an empty bordered band.
  const historyButton = (metric: HistoryMetric, metricName: string) =>
    onShowHistory ? (
      <ShowHistoryButton
        metric={metric}
        metricName={metricName}
        onSelect={onShowHistory}
        active={activeMetric === metric}
      />
    ) : undefined;

  return (
    <div className="space-y-4 lg:col-span-2">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          title={t('stats.cpu')}
          value={(current?.cpuPercent ?? 0).toFixed(1)}
          unit="%"
          icon={<IconCpu className="h-5 w-5" />}
          // CHART_COLORS.cpu, not var(--primary). The history panel below
          // draws CPU with CHART_COLORS.cpu, so a --primary tile made the
          // same metric two different colours on one screen — and the
          // tile-to-panel colour pairing is the whole point of clicking a
          // tile to expand it.
          accentColor={CHART_COLORS.cpu}
          chart={
            hasHistory ? (
              <StatsAreaChart
                data={cpuData}
                dataKey="value"
                xAxisKey="timestamp"
                color={CHART_COLORS.cpu}
                formatValue={(v) => `${v.toFixed(1)}%`}
                compact
              />
            ) : undefined
          }
          footer={historyButton('cpu', t('stats.cpu'))}
        />
        <StatCard
          title={t('stats.memory')}
          value={mem.value}
          unit={mem.unit}
          description={t('stats.memoryPercent', {
            percent: (current?.memPercent ?? 0).toFixed(1),
            limit: formatBytes(current?.memLimit ?? 0),
          })}
          icon={<IconCpu2 className="h-5 w-5" />}
          accentColor={CHART_COLORS.memory}
          chart={
            hasHistory ? (
              <StatsAreaChart
                data={memData}
                dataKey="value"
                xAxisKey="timestamp"
                color={CHART_COLORS.memory}
                formatValue={(v) => formatBytes(v)}
                compact
              />
            ) : undefined
          }
          footer={historyButton('memory', t('stats.memory'))}
        />
        <StatCard
          title={t('stats.networkIO')}
          value={netRx.value}
          unit={netRx.unit}
          description={t('stats.txDesc', {
            value: formatBytes(current?.netTx ?? 0),
          })}
          icon={<IconArrowBarDown className="h-5 w-5" />}
          // networkRx, not cpu. This tile drew RX in CPU's green, which
          // both mismatched the history panel (blue) and made network RX
          // indistinguishable from the CPU tile beside it. METRIC_STEP
          // assigns rx and tx opposite ends of the ramp precisely so the
          // pair reads apart; borrowing cpu's step defeated that.
          accentColor={CHART_COLORS.networkRx}
          chart={
            hasHistory ? (
              <StatsAreaChart
                data={netData}
                dataKey="rx"
                secondaryDataKey="tx"
                xAxisKey="timestamp"
                color={CHART_COLORS.networkRx}
                secondaryColor={CHART_COLORS.networkTx}
                formatValue={(v) => formatBytes(v)}
                label={t('stats.rx')}
                secondaryLabel={t('stats.tx')}
                compact
              />
            ) : undefined
          }
          footer={historyButton('network', t('stats.networkIO'))}
        />
        <StatCard
          title={t('stats.blockIO')}
          value={blockRead.value}
          unit={blockRead.unit}
          description={t('stats.writeDesc', {
            value: formatBytes(current?.blockWrite ?? 0),
          })}
          icon={<IconDeviceFloppy className="h-5 w-5" />}
          accentColor={CHART_COLORS.diskRead}
          chart={
            hasHistory ? (
              <StatsAreaChart
                data={blockData}
                dataKey="read"
                secondaryDataKey="write"
                xAxisKey="timestamp"
                color={CHART_COLORS.diskRead}
                secondaryColor={CHART_COLORS.diskWrite}
                formatValue={(v) => formatBytes(v)}
                label={t('stats.read')}
                secondaryLabel={t('stats.write')}
                compact
              />
            ) : undefined
          }
          footer={historyButton('disk', t('stats.blockIO'))}
        />
      </div>

      {!connected && current && (
        <div className="flex items-center gap-2 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
          <IconArrowBarUp className="h-3 w-3" />
          {t('stats.disconnected')}
        </div>
      )}
    </div>
  );
}
