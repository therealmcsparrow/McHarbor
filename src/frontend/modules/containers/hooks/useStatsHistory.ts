// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { BulkContainerMetric } from './useContainersBulkStats';

const MAX_HISTORY = 30;

export type StatsHistory = {
  cpu: number[];
  mem: number[];
  netRx: number[];
  netTx: number[];
};

export function useStatsHistory(stats: BulkContainerMetric | undefined): StatsHistory {
  const [state, setState] = useState<{ source: BulkContainerMetric | undefined; history: StatsHistory }>({
    source: undefined,
    history: { cpu: [], mem: [], netRx: [], netTx: [] },
  });

  if (stats && state.source !== stats) {
    const h = state.history;
    setState({
      source: stats,
      history: {
        cpu: [...h.cpu, stats.cpuPercent].slice(-MAX_HISTORY),
        mem: [...h.mem, stats.memUsage].slice(-MAX_HISTORY),
        netRx: [...h.netRx, stats.netRx].slice(-MAX_HISTORY),
        netTx: [...h.netTx, stats.netTx].slice(-MAX_HISTORY),
      },
    });
  }

  return state.history;
}
