// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

type StatCardProps = {
  title: string;
  value: string | number;
  unit?: string;
  description?: string;
  icon?: ReactNode;
  accentColor?: string;
  trend?: { value: number; label: string };
  chart?: ReactNode;
  className?: string;
};

export function StatCard({ title, value, unit, description, icon, accentColor, trend, chart, className }: StatCardProps) {
  return (
    <div className={cn('overflow-hidden rounded-lg border border-border bg-card', className)}>
      <div className={cn(chart ? 'px-6 pt-4 pb-0' : 'p-6')}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            {icon && <div style={accentColor ? { color: accentColor } : undefined} className={cn(!accentColor && 'text-muted-foreground')}>{icon}</div>}
            <p className="text-base font-semibold tracking-tight text-foreground">{title}</p>
          </div>
          <div className="flex items-baseline gap-1">
            <span className="text-3xl font-semibold text-foreground">{value}</span>
            {unit && <span className="text-sm font-light text-primary">{unit}</span>}
          </div>
        </div>
        {(description || trend) && (
          <div className="mt-1 text-right">
            {description && (
              <p className="text-xs text-muted-foreground">{description}</p>
            )}
            {trend && (
              <p
                className={cn(
                  'text-xs font-medium',
                  trend.value >= 0 ? 'text-green-500' : 'text-red-500'
                )}
              >
                {trend.value >= 0 ? '+' : ''}{trend.value}% {trend.label}
              </p>
            )}
          </div>
        )}
      </div>
      {chart && (
        <div className="mt-2 mb-1 h-16">{chart}</div>
      )}
    </div>
  );
}
