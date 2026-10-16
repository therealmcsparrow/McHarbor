// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { UseContainerStatsResult } from '../hooks/useContainerStats';

type StatsHistory = UseContainerStatsResult['history'];

// Shapes the rolling stats history into the per-tile series the sparkline
// charts on ContainerStatsPanel read. Index-based timestamps: the compact
// charts have no axis, so the x value only needs to be monotonic.
export function buildStatsChartData(historyData: StatsHistory) {
  return {
    cpuData: historyData.map((m, i) => ({
      timestamp: String(i),
      value: m.cpuPercent,
    })),
    memData: historyData.map((m, i) => ({
      timestamp: String(i),
      value: m.memUsage,
    })),
    netData: historyData.map((m, i) => ({
      timestamp: String(i),
      rx: m.netRx,
      tx: m.netTx,
    })),
    blockData: historyData.map((m, i) => ({
      timestamp: String(i),
      read: m.blockRead,
      write: m.blockWrite,
    })),
  };
}
