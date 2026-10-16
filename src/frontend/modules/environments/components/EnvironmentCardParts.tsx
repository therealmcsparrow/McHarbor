// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Sparkline } from '@resources/components/Sparkline';
import { cn } from '@resources/utils/cn';

export function lastValue(values: number[]): number | null {
  return values.length > 0 ? values[values.length - 1]! : null;
}

export function MetricSparkline({
  label,
  value,
  data,
  color,
  icon,
}: {
  label: string;
  value: string;
  data: number[];
  color: string;
  icon: ReactNode;
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-muted-foreground">{icon}</span>
          <span className="truncate text-xs font-medium text-muted-foreground">{label}</span>
        </div>
        <span className="shrink-0 text-sm font-semibold text-foreground tabular-nums">{value}</span>
      </div>
      <div className="mt-3 h-6">
        <Sparkline data={data.length >= 2 ? data : [0, 0]} width={160} height={24} color={color} className="w-full" />
      </div>
    </div>
  );
}

export function CountTile({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone: 'success' | 'muted' | 'warning' | 'danger';
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 px-3 py-2">
      <p
        className={cn(
          'text-lg font-semibold tabular-nums',
          tone === 'success' && 'text-chart-1',
          tone === 'muted' && 'text-muted-foreground',
          tone === 'warning' && 'text-warning',
          tone === 'danger' && 'text-danger',
        )}
      >
        {value}
      </p>
      <p className="truncate text-[11px] text-muted-foreground">{label}</p>
    </div>
  );
}
