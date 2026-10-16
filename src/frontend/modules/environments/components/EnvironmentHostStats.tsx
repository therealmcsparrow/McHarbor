// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { IconCpu, IconDeviceDesktop, IconDeviceSdCard, IconGauge, IconRefresh } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import type { HostMetrics } from '@core/types/docker';
import { formatUptime, splitBytes } from '@resources/utils/format';

export function RefreshIndicator({
  updatedAt,
  isFetching,
  t,
}: {
  updatedAt: number;
  isFetching: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const [now, setNow] = useState(() => Date.now());

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
          value: t('detail.host.secondsAgo', { count: seconds }),
        })}
      </span>
    </div>
  );
}

export function HostStatGrid({
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
