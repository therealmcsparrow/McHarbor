// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PieChart, Pie, Cell, ResponsiveContainer } from 'recharts';
import { useDashboardStats } from '@modules/dashboard/hooks/useDashboardStats';
import { cn } from '@resources/utils/cn';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

const COLORS = [
  'hsl(142 70% 45%)',  // running - green
  'hsl(0 70% 55%)',    // stopped - red
  'hsl(210 70% 50%)',  // images - blue
  'hsl(280 70% 50%)',  // volumes - purple
  'hsl(30 80% 55%)',   // networks - orange
];

const SWATCH_CLASSES = [
  'bg-[hsl(142_70%_45%)]',
  'bg-[hsl(0_70%_55%)]',
  'bg-[hsl(210_70%_50%)]',
  'bg-[hsl(280_70%_50%)]',
  'bg-[hsl(30_80%_55%)]',
];

const SEGMENT_KEYS = ['running', 'stopped', 'images', 'volumes', 'networks'] as const;

export default function ResourceDonutWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: stats } = useDashboardStats();

  if (!stats) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  const segmentValues = [
    stats.containers?.running ?? 0,
    stats.containers?.stopped ?? 0,
    stats.images ?? 0,
    stats.volumes ?? 0,
    stats.networks ?? 0,
  ];

  const segments = SEGMENT_KEYS
    .map((key, i) => ({
      name: t(`resourceDonutWidget.${key}`),
      value: segmentValues[i],
    }))
    .filter((s): s is { name: string; value: number } => (s.value ?? 0) > 0);

  const total = segments.reduce((sum, s) => sum + s.value, 0);

  if (total === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('resourceDonutWidget.noResources')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-1 text-sm font-semibold text-foreground">{t('resourceDonutWidget.title')}</h3>
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
            <span className="text-2xl font-semibold text-foreground">{total}</span>
            <span className="text-[10px] text-muted-foreground">{t('resourceDonutWidget.total')}</span>
          </div>
        </div>
        <div className="flex flex-col gap-1.5">
          {segments.map((s, i) => (
            <div key={s.name} className="flex items-center gap-2 text-xs">
              <div className={cn('h-2.5 w-2.5 rounded-full', SWATCH_CLASSES[i % SWATCH_CLASSES.length])} />
              <span className="text-muted-foreground">{s.name}</span>
              <span className="font-medium text-foreground">{s.value}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
