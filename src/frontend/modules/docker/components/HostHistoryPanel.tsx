// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { MetricHistoryPanel } from '@resources/components/MetricHistoryPanel';
import { useHostMetricsHistory, type HistoryRange } from '@resources/hooks/useMetricsHistory';
import type { HistoryMetric } from '@resources/components/history-metrics';

/**
 * Stored resource history for the environment as a whole.
 *
 * The Resources tab above this shows the host's *current* CPU, memory and disk;
 * this is where those numbers have been. The backend has persisted the series
 * all along (`GET /metrics/host/history`, the same tiered store the per-container
 * chart reads) and `useHostMetricsHistory` was written to fetch it — there was
 * simply no panel drawing it, which is why the hook read as orphaned.
 *
 * The metric switcher is always shown here: unlike the container view there are
 * no live tiles to click, so without it the panel could only ever draw CPU.
 */
export function HostHistoryPanel() {
  const [range, setRange] = useState<HistoryRange>('24h');
  const [metric, setMetric] = useState<HistoryMetric>('cpu');
  const { data: series, isLoading } = useHostMetricsHistory(range);

  return (
    <MetricHistoryPanel
      series={series}
      isLoading={isLoading}
      range={range}
      onRangeChange={setRange}
      metric={metric}
      onMetricChange={setMetric}
      showMetricSwitcher
    />
  );
}
