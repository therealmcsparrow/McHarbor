// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconArrowBarDown, IconArrowBarUp, IconDeviceFloppy } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatBytes, splitBytes } from '@resources/utils/format';
import { useContainerStats } from '../hooks/useContainerStats';

type ContainerStatsPanelProps = {
  containerId: string;
};

export function ContainerStatsPanel({ containerId }: ContainerStatsPanelProps) {
  const { t } = useTranslation('containers');
  const { current, history, connected } = useContainerStats(containerId);

  if (!connected && !current) {
    return (
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner size="sm" />
          {t('stats.connecting')}
        </div>
      </div>
    );
  }

  const cpuData = history.map((m, i) => ({ timestamp: String(i), value: m.cpuPercent }));
  const memData = history.map((m, i) => ({ timestamp: String(i), value: m.memUsage }));
  const netData = history.map((m, i) => ({ timestamp: String(i), rx: m.netRx, tx: m.netTx }));
  const blockData = history.map((m, i) => ({ timestamp: String(i), read: m.blockRead, write: m.blockWrite }));

  const hasHistory = history.length > 1;
  const mem = splitBytes(current?.memUsage ?? 0);
  const netRx = splitBytes(current?.netRx ?? 0);
  const blockRead = splitBytes(current?.blockRead ?? 0);

  return (
    <div className="space-y-4 lg:col-span-2">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          title={t('stats.cpu')}
          value={(current?.cpuPercent ?? 0).toFixed(1)}
          unit="%"
          icon={<IconCpu className="h-5 w-5" />}
          accentColor="var(--primary)"
          chart={hasHistory ? (
            <StatsAreaChart
              data={cpuData}
              dataKey="value"
              xAxisKey="timestamp"
              color="var(--primary)"
              formatValue={(v) => `${v.toFixed(1)}%`}
              compact
            />
          ) : undefined}
        />
        <StatCard
          title={t('stats.memory')}
          value={mem.value}
          unit={mem.unit}
          description={t('stats.memoryPercent', { percent: (current?.memPercent ?? 0).toFixed(1), limit: formatBytes(current?.memLimit ?? 0) })}
          icon={<IconCpu2 className="h-5 w-5" />}
          accentColor="hsl(280 70% 50%)"
          chart={hasHistory ? (
            <StatsAreaChart
              data={memData}
              dataKey="value"
              xAxisKey="timestamp"
              color="hsl(280 70% 50%)"
              formatValue={(v) => formatBytes(v)}
              compact
            />
          ) : undefined}
        />
        <StatCard
          title={t('stats.networkIO')}
          value={netRx.value}
          unit={netRx.unit}
          description={t('stats.txDesc', { value: formatBytes(current?.netTx ?? 0) })}
          icon={<IconArrowBarDown className="h-5 w-5" />}
          accentColor="hsl(142 70% 45%)"
          chart={hasHistory ? (
            <StatsAreaChart
              data={netData}
              dataKey="rx"
              secondaryDataKey="tx"
              xAxisKey="timestamp"
              color="hsl(142 70% 45%)"
              secondaryColor="hsl(30 80% 55%)"
              formatValue={(v) => formatBytes(v)}
              label={t('stats.rx')}
              secondaryLabel={t('stats.tx')}
              compact
            />
          ) : undefined}
        />
        <StatCard
          title={t('stats.blockIO')}
          value={blockRead.value}
          unit={blockRead.unit}
          description={t('stats.writeDesc', { value: formatBytes(current?.blockWrite ?? 0) })}
          icon={<IconDeviceFloppy className="h-5 w-5" />}
          accentColor="hsl(210 70% 50%)"
          chart={hasHistory ? (
            <StatsAreaChart
              data={blockData}
              dataKey="read"
              secondaryDataKey="write"
              xAxisKey="timestamp"
              color="hsl(210 70% 50%)"
              secondaryColor="hsl(0 70% 50%)"
              formatValue={(v) => formatBytes(v)}
              label={t('stats.read')}
              secondaryLabel={t('stats.write')}
              compact
            />
          ) : undefined}
        />
      </div>

      {!connected && current && (
        <div className="flex items-center gap-2 rounded-md border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-500">
          <IconArrowBarUp className="h-3 w-3" />
          {t('stats.disconnected')}
        </div>
      )}
    </div>
  );
}
