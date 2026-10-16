// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Windows the history endpoints accept as a `range` shorthand. */
export type HistoryRange = '1h' | '6h' | '12h' | '24h' | '7d' | '30d' | '90d';

export const HISTORY_RANGES: HistoryRange[] = ['1h', '6h', '24h', '7d', '30d'];

/**
 * One point of a stored series.
 *
 * Raw and rollup points carry identical fields, so a chart never branches on
 * resolution: for a raw point the `*Max` fields equal their average
 * counterpart and `samples` is 1.
 *
 * The counter fields come in pairs on purpose. `netRx` and friends are
 * Docker's cumulative totals since container start; the `*Delta` fields are
 * the movement across this point's interval. Throughput charts want the
 * delta — plotting the counter itself draws a permanently rising line that
 * says nothing about current activity.
 */
export type MetricPoint = {
  timestamp: string;
  resolution: 'raw' | 'rollup';
  samples: number;
  cpuPercent: number;
  cpuMax: number;
  memUsed: number;
  memUsedMax: number;
  memLimit: number;
  memPercent: number;
  netRx: number;
  netTx: number;
  blockRead: number;
  blockWrite: number;
  netRxDelta: number;
  netTxDelta: number;
  blockReadDelta: number;
  blockWriteDelta: number;
  /**
   * Mean GPU utilisation across the host's GPUs, absent when the host
   * reported no GPU telemetry. Omitted rather than zero so the chart
   * can leave the series out instead of drawing a flat zero line.
   */
  gpuPercent?: number;
  sizeRw?: number;
  sizeRootFs?: number;
};

/**
 * A stored history response.
 *
 * `coveredFrom`/`coveredTo` describe what the series actually contains, which
 * is not always what was asked for — history only begins when the collector
 * first ran. Callers that label a period should compare these against
 * `from`/`to` rather than assume the request was fully satisfied.
 */
export type MetricSeries = {
  environmentId: string;
  containerId?: string;
  name?: string;
  from: string;
  to: string;
  coveredFrom?: string;
  coveredTo?: string;
  bucketSeconds: number;
  points: MetricPoint[];
};

/**
 * Stored metric history for one container.
 *
 * Deliberately separate from useContainerStats: that hook streams live values
 * and holds them in memory only, so it resets on reload and cannot look back
 * further than the page has been open.
 */
export function useContainerMetricsHistory(containerId: string, range: HistoryRange, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: ['container-metrics-history', envId, containerId, range],
    queryFn: () =>
      api
        .get<MetricSeries>(`/metrics/containers/${containerId}/history`, envId ? { env: envId, range } : { range })
        .then(assertSuccess),
    enabled: enabled && !!containerId,
    // The series only changes when the collector ticks, so polling faster than
    // its 30s cadence would just re-fetch identical rows.
    refetchInterval: 30_000,
  });
}

/** Stored metric history for the environment aggregate. */
export function useHostMetricsHistory(range: HistoryRange, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: ['host-metrics-history', envId, range],
    queryFn: () =>
      api.get<MetricSeries>('/metrics/host/history', envId ? { env: envId, range } : { range }).then(assertSuccess),
    enabled,
    refetchInterval: 30_000,
  });
}
