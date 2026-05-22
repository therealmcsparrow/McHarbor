// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useEffect } from 'react';
import type { BulkContainerMetric } from './useContainersBulkStats';

const MAX_HISTORY = 30;

export type StatsHistory = {
  cpu: number[];
  mem: number[];
  netRx: number[];
  netTx: number[];
};

export function useStatsHistory(stats: BulkContainerMetric | undefined): StatsHistory {
  const historyRef = useRef<StatsHistory>({ cpu: [], mem: [], netRx: [], netTx: [] });

  useEffect(() => {
    if (!stats) return;
    const h = historyRef.current;
    h.cpu = [...h.cpu, stats.cpuPercent].slice(-MAX_HISTORY);
    h.mem = [...h.mem, stats.memUsage].slice(-MAX_HISTORY);
    h.netRx = [...h.netRx, stats.netRx].slice(-MAX_HISTORY);
    h.netTx = [...h.netTx, stats.netTx].slice(-MAX_HISTORY);
  }, [stats]);

  return historyRef.current;
}
