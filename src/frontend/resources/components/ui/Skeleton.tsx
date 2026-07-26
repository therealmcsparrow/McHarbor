// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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
      className={cn(
        'animate-pulse bg-muted',
        roundedClasses[rounded],
        className,
      )}
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
    <div
      className={cn(
        'rounded-lg border border-border bg-card p-5 space-y-3',
        className,
      )}
    >
      <Skeleton className="h-5 w-2/3" />
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton
          key={i}
          className={cn('h-3', i === lines - 1 ? 'w-2/3' : 'w-full')}
        />
      ))}
    </div>
  );
}

type TableSkeletonProps = {
  className?: string;
  rows?: number;
  columns?: number;
};

export function TableSkeleton({
  className,
  rows = 5,
  columns = 4,
}: TableSkeletonProps) {
  return (
    <div className={cn('rounded-lg border border-border bg-card', className)}>
      <div className="border-b border-border p-3">
        <Skeleton className="h-4 w-1/4" />
      </div>
      <div className="divide-y divide-border">
        {Array.from({ length: rows }).map((_, r) => (
          <div key={r} className="flex items-center gap-4 p-3">
            {Array.from({ length: columns }).map((_, c) => (
              <Skeleton
                key={c}
                className={cn(
                  'h-3 flex-1',
                  c === 0 ? 'max-w-[40%]' : '',
                )}
              />
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
      className={cn(
        'rounded-lg border border-border bg-card p-5 space-y-3',
        className,
      )}
      style={{ minHeight: height }}
    >
      <Skeleton className="h-4 w-1/3" />
      <div className="flex items-end gap-2" style={{ height: height - 60 }}>
        {barHeights.map((h, i) => (
          <Skeleton
            key={i}
            className="flex-1"
            style={{ height: `${h}%` }}
          />
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
    <div
      className={cn(
        'rounded-lg border border-border bg-card p-4 space-y-2',
        className,
      )}
    >
      <Skeleton className="h-3 w-1/2" />
      <Skeleton className="h-8 w-3/4" />
      <Skeleton className="h-3 w-1/3" />
    </div>
  );
}
