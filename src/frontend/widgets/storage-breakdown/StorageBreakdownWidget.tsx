// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PieChart, Pie, Cell, ResponsiveContainer } from 'recharts';
import { useHostMetrics } from '@modules/dashboard/hooks/useHostMetrics';
import { formatBytes } from '@resources/utils/format';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

const COLORS = [
  'hsl(210 70% 50%)',  // images - blue
  'hsl(280 70% 50%)',  // volumes - purple
  'hsl(142 70% 45%)',  // containers - green
  'hsl(30 80% 55%)',   // build cache - orange
];

export default function StorageBreakdownWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: metrics } = useHostMetrics();

  const segments = useMemo(() => {
    if (!metrics) return [];
    const items = [
      { name: t('storageBreakdownWidget.images'), value: metrics.disk.imagesSize },
      { name: t('storageBreakdownWidget.volumes'), value: metrics.disk.volumesSize },
      { name: t('storageBreakdownWidget.containers'), value: metrics.disk.containersSize },
      { name: t('storageBreakdownWidget.buildCache'), value: metrics.disk.buildCacheSize },
    ].filter((s) => s.value > 0);
    return items;
  }, [metrics, t]);

  if (!metrics) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  if (segments.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('storageBreakdownWidget.noData')}
      </div>
    );
  }

  const total = segments.reduce((sum, s) => sum + s.value, 0);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-1 text-sm font-semibold text-foreground">
        {t('storageBreakdownWidget.title')}
      </h3>
      <div className="flex flex-1 items-center gap-4 px-4 pb-3">
        <div className="relative h-[140px] w-[140px] shrink-0">
          <ResponsiveContainer width="100%" height="100%" minWidth={0} minHeight={0}>
            <PieChart>
              <Pie
                data={segments}
                cx="50%"
                cy="50%"
                innerRadius={40}
                outerRadius={65}
                paddingAngle={2}
                dataKey="value"
                stroke="none"
              >
                {segments.map((segment, i) => (
                  <Cell key={segment.name} fill={COLORS[i % COLORS.length]} />
                ))}
              </Pie>
            </PieChart>
          </ResponsiveContainer>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-lg font-semibold text-foreground">{formatBytes(total)}</span>
            <span className="text-[10px] text-muted-foreground">{t('storageBreakdownWidget.total')}</span>
          </div>
        </div>
        <div className="flex flex-col gap-1.5">
          {segments.map((s, i) => (
            <div key={s.name} className="flex items-center gap-2 text-xs">
              <div className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: COLORS[i % COLORS.length] }} />
              <span className="text-muted-foreground">{s.name}</span>
              <span className="font-medium text-foreground">{formatBytes(s.value)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
