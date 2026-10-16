// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// CHART_COLORS — one colour per metric, everywhere.
//
// Chart colour used to be a `hsl(142 71% 45%)` literal repeated at each
// call site, which meant two things: no theme could restyle a chart, and
// the same metric drew in different colours depending on which component
// happened to render it. CPU was green on a container card and green in
// the stats panel, but memory was blue on the card and violet in the
// panel — so the colour told you nothing you could carry between views.
//
// Both problems are fixed by naming the metric rather than the colour.
// The values resolve through `--chart-1…8`, so a theme restyles every
// chart at once, and a metric keeps its colour across every surface.

import { chartVar } from './status-palette';

export type MetricKey =
  | 'cpu'
  | 'memory'
  | 'networkRx'
  | 'networkTx'
  | 'diskRead'
  | 'diskWrite'
  | 'gpu'
  | 'containers'
  | 'images'
  | 'volumes'
  | 'networks';

/**
 * Fixed metric → ramp step assignment. Chosen so that the pairs users
 * read together stay far apart on the ramp: rx/tx and read/write are
 * each other's contrast, and network never collides with disk.
 */
export const METRIC_STEP: Record<MetricKey, 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8> = {
  cpu: 4, // green
  memory: 3, // violet
  networkRx: 5, // blue
  networkTx: 2, // amber
  diskRead: 1, // teal
  diskWrite: 6, // red
  gpu: 7, // magenta — unused elsewhere, so GPU never collides with CPU or memory
  containers: 1,
  images: 3,
  volumes: 2,
  networks: 5,
};

/** `var(--chart-n)` for a named metric. */
export function metricColor(metric: MetricKey): string {
  return chartVar(METRIC_STEP[metric]);
}

export const CHART_COLORS: Record<MetricKey, string> = Object.fromEntries(
  (Object.keys(METRIC_STEP) as MetricKey[]).map((k) => [k, metricColor(k)]),
) as Record<MetricKey, string>;
