// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PieChart, Pie, Cell, ResponsiveContainer } from 'recharts';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { cn } from '@resources/utils/cn';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

const STATE_COLORS: Record<string, string> = {
  running: 'hsl(142 70% 45%)',
  exited: 'hsl(0 60% 50%)',
  stopped: 'hsl(0 0% 50%)',
  paused: 'hsl(45 80% 50%)',
  restarting: 'hsl(30 80% 55%)',
  created: 'hsl(210 50% 60%)',
  dead: 'hsl(0 40% 40%)',
};

const STATE_SWATCH_CLASSES: Record<string, string> = {
  running: 'bg-[hsl(142_70%_45%)]',
  exited: 'bg-[hsl(0_60%_50%)]',
  stopped: 'bg-[hsl(0_0%_50%)]',
  paused: 'bg-[hsl(45_80%_50%)]',
  restarting: 'bg-[hsl(30_80%_55%)]',
  created: 'bg-[hsl(210_50%_60%)]',
  dead: 'bg-[hsl(0_40%_40%)]',
  unknown: 'bg-[hsl(0_0%_50%)]',
};

export default function ContainerStatesWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: containers, isLoading } = useContainers(true);

  const segments = useMemo(() => {
    if (!containers) return [];
    const counts: Record<string, number> = {};
    for (const c of containers) {
      const state = c.State ?? 'unknown';
      counts[state] = (counts[state] ?? 0) + 1;
    }
    return Object.entries(counts)
      .map(([name, value]) => ({ name, value }))
      .filter((s) => s.value > 0)
      .sort((a, b) => b.value - a.value);
  }, [containers]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }

  const total = segments.reduce((sum, s) => sum + s.value, 0);

  if (total === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('containerStatesWidget.noContainers')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-1 text-sm font-semibold text-foreground">
        {t('containerStatesWidget.title')}
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
                {segments.map((s) => (
                  <Cell key={s.name} fill={STATE_COLORS[s.name] ?? 'hsl(0 0% 50%)'} />
                ))}
              </Pie>
            </PieChart>
          </ResponsiveContainer>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-2xl font-semibold text-foreground">{total}</span>
            <span className="text-[10px] text-muted-foreground">{t('containerStatesWidget.total')}</span>
          </div>
        </div>
        <div className="flex flex-col gap-1.5">
          {segments.map((s) => (
            <div key={s.name} className="flex items-center gap-2 text-xs">
              <div
                className={cn('h-2.5 w-2.5 rounded-full', STATE_SWATCH_CLASSES[s.name] ?? STATE_SWATCH_CLASSES.unknown)}
              />
              <span className="text-muted-foreground capitalize">{s.name}</span>
              <span className="font-medium text-foreground">{s.value}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
