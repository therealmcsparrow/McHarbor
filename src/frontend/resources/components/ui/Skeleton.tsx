// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import type { CSSProperties } from 'react';
import { cn } from '@resources/utils/cn';

type SkeletonProps = {
  className?: string;
  rounded?: 'sm' | 'md' | 'lg' | 'full';
  style?: CSSProperties;
};

const roundedClasses = {
  sm: 'rounded-sm',
  md: 'rounded-md',
  lg: 'rounded-lg',
  full: 'rounded-full',
};

export function Skeleton({ className, rounded = 'md', style }: SkeletonProps) {
  return (
    <div
      className={cn('animate-pulse bg-muted', roundedClasses[rounded], className)}
      style={style}
      aria-busy="true"
      aria-live="polite"
    />
  );
}

type CardSkeletonProps = {
  className?: string;
  lines?: number;
};

export function CardSkeleton({ className, lines = 3 }: CardSkeletonProps) {
  return (
    <div className={cn('mc-glass space-y-3 rounded-lg p-5', className)}>
      <Skeleton className="h-5 w-2/3" />
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={`line-${i}`} className={cn('h-3', i === lines - 1 ? 'w-2/3' : 'w-full')} />
      ))}
    </div>
  );
}

type DataGridDensity = 'compact' | 'default' | 'comfortable';

type TableSkeletonProps = {
  className?: string;
  rows?: number;
  columns?: number;
  density?: DataGridDensity;
};

const densityPadding: Record<DataGridDensity, string> = {
  compact: 'p-2',
  default: 'p-3',
  comfortable: 'p-4',
};

export function TableSkeleton({ className, rows = 5, columns = 4, density = 'default' }: TableSkeletonProps) {
  return (
    <div className={cn('mc-glass rounded-lg', className)}>
      <div className={cn('border-b border-border', densityPadding[density])}>
        <Skeleton className="h-4 w-1/4" />
      </div>
      <div className="divide-y divide-border">
        {Array.from({ length: rows }).map((_, r) => (
          <div key={r} className={cn('flex items-center gap-4', densityPadding[density])}>
            {Array.from({ length: columns }).map((_cell, c) => (
              <Skeleton key={c} className={cn('h-3 flex-1', c === 0 ? 'max-w-[40%]' : '')} />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

type ChartSkeletonProps = {
  className?: string;
  height?: number;
};

export function ChartSkeleton({ className, height = 200 }: ChartSkeletonProps) {
  const barHeights = [40, 65, 50, 80, 70, 55, 90, 60];
  return (
    <div
      className={cn('mc-glass min-h-(--h) space-y-3 rounded-lg p-5', className)}
      style={cssVars({ h: `${height}px` })}
    >
      <Skeleton className="h-4 w-1/3" />
      <div className="flex h-(--h) items-end gap-2" style={cssVars({ h: `${height - 60}px` })}>
        {barHeights.map((h, i) => (
          <Skeleton key={`bar-${i}`} className="h-(--h) flex-1" style={cssVars({ h: `${h}%` })} />
        ))}
      </div>
    </div>
  );
}

type WidgetSkeletonProps = {
  className?: string;
};

export function WidgetSkeleton({ className }: WidgetSkeletonProps) {
  return (
    <div className={cn('mc-glass space-y-2 rounded-lg p-4', className)}>
      <Skeleton className="h-3 w-1/2" />
      <Skeleton className="h-8 w-3/4" />
      <Skeleton className="h-3 w-1/3" />
    </div>
  );
}
