// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { CHART_COLORS } from '@resources/theme/chart-colors';

/**
 * Which of the four stat tiles the history panel is currently mirroring.
 *
 * Named after the tile rather than the underlying column because the tiles are
 * the primary way to change it: `ContainerStatsPanel` writes this,
 * `ContainerHistoryPanel` reads it, and `OverviewTab` owns the state between
 * them. It lives here rather than in either component so neither has to import
 * the other just for a type.
 */
export type HistoryMetric = 'cpu' | 'memory' | 'network' | 'disk' | 'gpu';

/** Tile order, which is also the switcher order. */
export const HISTORY_METRICS: HistoryMetric[] = ['cpu', 'memory', 'network', 'disk', 'gpu'];

/** One row of the chart, flattened from a `MetricPoint`. */
export type ChartRow = {
  label: string;
  cpu: number;
  memory: number;
  netRx: number;
  netTx: number;
  blockRead: number;
  blockWrite: number;
  // Undefined when the host reported no GPU telemetry, so the
  // series renders as absent rather than as a flat zero.
  gpu?: number;
};

export type MetricChart = {
  titleKey: string;
  dataKey: keyof ChartRow;
  secondaryDataKey?: keyof ChartRow;
  color: string;
  secondaryColor?: string;
  labelKey: string;
  secondaryLabelKey?: string;
  /** Format values as bytes rather than a percentage. */
  bytes: boolean;
};

/**
 * One entry per metric, so the panel's render body never branches on which is
 * active. Adding a fifth metric is a row here plus a tile, not another `if`.
 *
 * Colours come from CHART_COLORS so a metric keeps the same colour here as on
 * the live tile above it — the pairing is the whole point of the tile-to-panel
 * interaction.
 */
export const METRIC_CHARTS: Record<HistoryMetric, MetricChart> = {
  cpu: {
    titleKey: 'history.cpu',
    dataKey: 'cpu',
    color: CHART_COLORS.cpu,
    labelKey: 'history.cpu',
    bytes: false,
  },
  memory: {
    titleKey: 'history.memory',
    dataKey: 'memory',
    color: CHART_COLORS.memory,
    labelKey: 'history.memory',
    bytes: true,
  },
  network: {
    titleKey: 'history.network',
    dataKey: 'netRx',
    secondaryDataKey: 'netTx',
    color: CHART_COLORS.networkRx,
    secondaryColor: CHART_COLORS.networkTx,
    labelKey: 'history.networkRx',
    secondaryLabelKey: 'history.networkTx',
    bytes: true,
  },
  gpu: {
    titleKey: 'history.gpu',
    dataKey: 'gpu',
    color: CHART_COLORS.gpu,
    labelKey: 'history.gpu',
    bytes: false,
  },
  disk: {
    titleKey: 'history.disk',
    dataKey: 'blockRead',
    secondaryDataKey: 'blockWrite',
    color: CHART_COLORS.diskRead,
    secondaryColor: CHART_COLORS.diskWrite,
    labelKey: 'history.diskRead',
    secondaryLabelKey: 'history.diskWrite',
    bytes: true,
  },
};
