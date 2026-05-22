// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense, lazy } from 'react';

export type StatsAreaChartProps = {
  data: Array<Record<string, unknown>>;
  dataKey: string;
  secondaryDataKey?: string;
  xAxisKey?: string;
  color?: string;
  secondaryColor?: string;
  formatValue?: (value: number) => string;
  label?: string;
  secondaryLabel?: string;
  compact?: boolean;
};

const StatsAreaChartImpl = lazy(() =>
  import('./StatsAreaChartImpl').then((module) => ({ default: module.StatsAreaChartImpl }))
);

export function StatsAreaChart(props: StatsAreaChartProps) {
  return (
    <Suspense
      fallback={
        <div className={props.compact ? 'h-full' : 'my-2 flex-1'}>
          <div className="h-full min-h-[4rem] animate-pulse rounded bg-muted/40" />
        </div>
      }
    >
      <StatsAreaChartImpl {...props} />
    </Suspense>
  );
}
