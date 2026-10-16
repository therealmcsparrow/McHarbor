// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { MetricHistoryPanel } from '@resources/components/MetricHistoryPanel';
import { useContainerMetricsHistory, type HistoryRange } from '@resources/hooks/useMetricsHistory';
import type { HistoryMetric } from '@resources/components/history-metrics';

type ContainerHistoryPanelProps = {
  containerId: string;
  metric: HistoryMetric;
  onMetricChange?: (metric: HistoryMetric) => void;
  /**
   * Render the in-panel metric switcher. Only wanted when the four stat tiles
   * are absent — a stopped container has no live tiles to click, and its past
   * is exactly what this panel exists to show.
   */
  showMetricSwitcher?: boolean;
};

/**
 * Stored resource history for one container, as opposed to the live tiles
 * above it.
 *
 * ContainerStatsPanel streams current values and keeps them in memory, so it
 * resets on reload and can never look back further than the page has been
 * open. This reads the persisted tiers instead: 30-second samples for the last
 * day, 10-minute averages before that. One metric at a time, chosen by the
 * tile the user clicked.
 *
 * The rendering lives in `MetricHistoryPanel`, which the host panel shares.
 * This file is now only "which series, and at what range".
 */
export function ContainerHistoryPanel({
  containerId,
  metric,
  onMetricChange,
  showMetricSwitcher = false,
}: ContainerHistoryPanelProps) {
  const [range, setRange] = useState<HistoryRange>('24h');
  const { data: series, isLoading } = useContainerMetricsHistory(containerId, range);

  return (
    <MetricHistoryPanel
      series={series}
      isLoading={isLoading}
      range={range}
      onRangeChange={setRange}
      metric={metric}
      onMetricChange={onMetricChange}
      showMetricSwitcher={showMetricSwitcher}
      className="lg:col-span-2"
    />
  );
}
