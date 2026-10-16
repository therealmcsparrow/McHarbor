// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Per-widget loading shapes.
//
// The grid used to fall back to a bare grey block while a widget's lazy
// chunk loaded. A grey block is honest about "something is coming" and
// silent about *what*, so the layout appeared to reflow when the real
// content arrived at a different shape.
//
// These match the shape the widget will actually take, which is the whole
// point of a skeleton: the page should not move when the data lands.

import { cssVars } from '@resources/utils/cssVars';
import type { CSSProperties } from 'react';
import { cn } from '@resources/utils/cn';

function Bar({ className, style }: { className?: string; style?: CSSProperties }) {
  return <div className={cn('animate-pulse rounded-md bg-muted-foreground/15', className)} style={style} />;
}

/** Shapes, keyed by what the widget looks like rather than by its id. */
export type WidgetSkeletonShape = 'stat' | 'chart' | 'list' | 'verdict';

/** Map a widget type to the shape it renders. Unknown types get a list. */
export function skeletonShapeFor(typeId: string): WidgetSkeletonShape {
  if (
    typeId === 'containers' ||
    typeId === 'images' ||
    typeId === 'volumes' ||
    typeId === 'networks' ||
    typeId === 'cpu-cores' ||
    typeId === 'total-memory' ||
    typeId === 'docker-version' ||
    typeId === 'disk-usage' ||
    typeId === 'uptime'
  ) {
    return 'stat';
  }
  if (typeId.endsWith('-chart') || typeId === 'resource-donut') return 'chart';
  if (typeId === 'fleet-health') return 'verdict';
  return 'list';
}

export function WidgetSkeleton({ shape }: { shape: WidgetSkeletonShape }) {
  if (shape === 'stat') {
    return (
      <div className="flex h-full flex-col justify-center gap-2 px-4">
        <Bar className="h-3 w-20" />
        <Bar className="h-7 w-16" />
      </div>
    );
  }

  if (shape === 'chart') {
    return (
      <div className="flex h-full flex-col gap-3 px-4 pt-3 pb-3">
        <Bar className="h-3 w-24" />
        <div className="flex min-h-0 flex-1 items-end gap-1.5">
          {[40, 65, 50, 80, 60, 90, 45, 70, 55, 85].map((height) => (
            <Bar
              key={height}
              className="h-(--h) w-full"
              // The bar heights are the data being faked; they travel as a
              // CSS variable the class reads.
              style={cssVars({ h: `${height}%` })}
            />
          ))}
        </div>
      </div>
    );
  }

  if (shape === 'verdict') {
    return (
      <div className="flex h-full flex-col justify-center gap-3 px-4">
        <div className="flex items-center gap-2.5">
          <Bar className="size-5 shrink-0 rounded-full" />
          <Bar className="h-5 w-56" />
        </div>
        <Bar className="h-2 w-full rounded-full" />
        <div className="flex gap-4">
          <Bar className="h-3 w-16" />
          <Bar className="h-3 w-16" />
          <Bar className="h-3 w-16" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-2 px-4 pt-3 pb-3">
      <Bar className="h-3 w-28" />
      {[0, 1, 2, 3].map((i) => (
        <div key={`row-${i}`} className="flex items-center gap-2.5">
          <Bar className="size-4 shrink-0 rounded-full" />
          <Bar className="h-3 flex-1" />
          <Bar className="h-3 w-10 shrink-0" />
        </div>
      ))}
    </div>
  );
}
