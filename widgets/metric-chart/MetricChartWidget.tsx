// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconActivity, IconCpu2, IconArrowBarDown, IconDeviceFloppy } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { formatBytes, splitBytes } from '@resources/utils/format';
import { useDashboardStats, type DashboardStats } from '@modules/dashboard/hooks/useDashboardStats';
import { useHostMetrics } from '@modules/dashboard/hooks/useHostMetrics';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type ChartConfig = {
  titleKey: string;
  icon: typeof IconActivity;
  accentColor: string;
  getValue: (s: DashboardStats) => string;
  getUnit: (s: DashboardStats) => string;
  getDescription?: (s: DashboardStats, host: ReturnType<typeof useHostMetrics>['data'], t: (key: string, params?: Record<string, string>) => string) => string | undefined;
  getChart: (s: DashboardStats, t: (key: string) => string) => React.ReactNode | null;
  hasData: (s: DashboardStats) => boolean;
};

const CONFIG: Record<string, ChartConfig> = {
  'cpu-chart': {
    titleKey: 'metricChartWidget.cpuUsage',
    icon: IconActivity,
    accentColor: 'var(--primary)',
    getValue: (s) => {
      const last = s.cpuHistory?.[s.cpuHistory.length - 1];
      return (last?.value ?? 0).toFixed(1);
    },
    getUnit: () => '%',
    hasData: (s) => (s.cpuHistory?.length ?? 0) > 0,
    getChart: (s) =>
      s.cpuHistory ? (
        <StatsAreaChart
          data={s.cpuHistory}
          dataKey="value"
          xAxisKey="timestamp"
          color="var(--primary)"
          formatValue={(v) => `${v.toFixed(1)}%`}
          compact
        />
      ) : null,
  },
  'memory-chart': {
    titleKey: 'metricChartWidget.memoryUsage',
    icon: IconCpu2,
    accentColor: 'hsl(280 70% 50%)',
    getValue: (s) => {
      const last = s.memoryHistory?.[s.memoryHistory.length - 1];
      return last ? splitBytes(last.value).value : '0';
    },
    getUnit: (s) => {
      const last = s.memoryHistory?.[s.memoryHistory.length - 1];
      return last ? splitBytes(last.value).unit : 'B';
    },
    getDescription: (_s, host, t) => host ? t('metricChartWidget.ofTotal', { total: formatBytes(host.host.memTotal) }) : undefined,
    hasData: (s) => (s.memoryHistory?.length ?? 0) > 0,
    getChart: (s) =>
      s.memoryHistory ? (
        <StatsAreaChart
          data={s.memoryHistory}
          dataKey="value"
          xAxisKey="timestamp"
          color="hsl(280 70% 50%)"
          formatValue={(v) => formatBytes(v)}
          compact
        />
      ) : null,
  },
  'network-io-chart': {
    titleKey: 'metricChartWidget.networkIO',
    icon: IconArrowBarDown,
    accentColor: 'hsl(142 70% 45%)',
    getValue: (s) => {
      const last = s.networkRxHistory?.[s.networkRxHistory.length - 1];
      return last ? splitBytes(last.value).value : '0';
    },
    getUnit: (s) => {
      const last = s.networkRxHistory?.[s.networkRxHistory.length - 1];
      return last ? splitBytes(last.value).unit : 'B';
    },
    getDescription: (s, _host, t) => {
      const last = s.networkTxHistory?.[s.networkTxHistory.length - 1];
      return t('metricChartWidget.tx', { value: last ? formatBytes(last.value) : '0 B' });
    },
    hasData: (s) => (s.networkRxHistory?.length ?? 0) > 0,
    getChart: (s, t) =>
      s.networkRxHistory ? (
        <StatsAreaChart
          data={s.networkRxHistory.map((p, i) => ({
            timestamp: p.timestamp,
            rx: p.value,
            tx: s.networkTxHistory?.[i]?.value ?? 0,
          }))}
          dataKey="rx"
          secondaryDataKey="tx"
          xAxisKey="timestamp"
          color="hsl(142 70% 45%)"
          secondaryColor="hsl(30 80% 55%)"
          formatValue={(v) => formatBytes(v)}
          label={t('metricChartWidget.rxLabel')}
          secondaryLabel={t('metricChartWidget.txLabel')}
          compact
        />
      ) : null,
  },
  'disk-io-chart': {
    titleKey: 'metricChartWidget.diskIO',
    icon: IconDeviceFloppy,
    accentColor: 'hsl(210 70% 50%)',
    getValue: (s) => {
      const last = s.blockReadHistory?.[s.blockReadHistory.length - 1];
      return last ? splitBytes(last.value).value : '0';
    },
    getUnit: (s) => {
      const last = s.blockReadHistory?.[s.blockReadHistory.length - 1];
      return last ? splitBytes(last.value).unit : 'B';
    },
    getDescription: (s, _host, t) => {
      const last = s.blockWriteHistory?.[s.blockWriteHistory.length - 1];
      return t('metricChartWidget.write', { value: last ? formatBytes(last.value) : '0 B' });
    },
    hasData: (s) => (s.blockReadHistory?.length ?? 0) > 0,
    getChart: (s, t) =>
      s.blockReadHistory ? (
        <StatsAreaChart
          data={s.blockReadHistory.map((p, i) => ({
            timestamp: p.timestamp,
            read: p.value,
            write: s.blockWriteHistory?.[i]?.value ?? 0,
          }))}
          dataKey="read"
          secondaryDataKey="write"
          xAxisKey="timestamp"
          color="hsl(210 70% 50%)"
          secondaryColor="hsl(0 70% 50%)"
          formatValue={(v) => formatBytes(v)}
          label={t('metricChartWidget.readLabel')}
          secondaryLabel={t('metricChartWidget.writeLabel')}
          compact
        />
      ) : null,
  },
};

export default function MetricChartWidget({ typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: stats } = useDashboardStats();
  const { data: hostMetrics } = useHostMetrics();
  const cfg = CONFIG[typeId];
  if (!cfg || !stats || !cfg.hasData(stats)) return null;
  const Icon = cfg.icon;

  return (
    <StatCard
      title={t(cfg.titleKey)}
      value={cfg.getValue(stats)}
      unit={cfg.getUnit(stats)}
      description={cfg.getDescription?.(stats, hostMetrics, t)}
      icon={<Icon className="h-5 w-5" />}
      accentColor={cfg.accentColor}
      chart={cfg.getChart(stats, t)}
      className="h-full border-0"
    />
  );
}
