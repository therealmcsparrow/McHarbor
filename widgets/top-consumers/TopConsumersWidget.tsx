// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BarChart, Bar, XAxis, YAxis, Tooltip } from 'recharts';
import { useContainersBulkStats } from '@modules/containers/hooks/useContainers';
import { MeasuredResponsiveContainer } from '@resources/components/MeasuredResponsiveContainer';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

export default function TopConsumersWidget({ typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: statsMap, isLoading } = useContainersBulkStats();
  const isCpu = typeId === 'top-cpu-consumers';

  const chartData = useMemo(() => {
    if (!statsMap) return [];
    const entries = Array.from(statsMap.values())
      .sort((a, b) => (isCpu ? b.cpuPercent - a.cpuPercent : b.memUsage - a.memUsage))
      .slice(0, 8)
      .map((m) => ({
        name: m.name.replace(/^\//, '').slice(0, 18),
        value: isCpu ? Math.round(m.cpuPercent * 100) / 100 : Math.round((m.memUsage / 1024 / 1024) * 10) / 10,
      }));
    return entries;
  }, [statsMap, isCpu]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (chartData.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('topConsumersWidget.noData')}
      </div>
    );
  }

  const label = isCpu
    ? t('topConsumersWidget.cpuTitle')
    : t('topConsumersWidget.memoryTitle');
  const unit = isCpu ? '%' : 'MB';

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-1 text-sm font-semibold text-foreground">{label}</h3>
      <div className="min-h-0 flex-1 px-2 pb-2">
        <MeasuredResponsiveContainer>
          <BarChart data={chartData} layout="vertical" margin={{ left: 4, right: 12, top: 4, bottom: 4 }}>
            <XAxis type="number" tick={{ fontSize: 10 }} tickFormatter={(v: number) => `${v}${unit}`} />
            <YAxis type="category" dataKey="name" tick={{ fontSize: 10 }} width={80} />
            <Tooltip
              formatter={(v) => [`${v ?? 0} ${unit}`, isCpu ? 'CPU' : 'Memory']}
              contentStyle={{ fontSize: 11, background: 'hsl(var(--popover))', border: '1px solid hsl(var(--border))' }}
            />
            <Bar dataKey="value" fill={isCpu ? 'hsl(210 70% 50%)' : 'hsl(280 70% 50%)'} radius={[0, 4, 4, 0]} />
          </BarChart>
        </MeasuredResponsiveContainer>
      </div>
    </div>
  );
}
