// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import type { MetricsPoint, Stats } from '../types';

/** One hour at the server's 30s cadence, plus room for the live points after it. */
const MAX_POINTS = 150;

export type StatsPoint = {
  at: number;
  cpuPercent: number;
  memoryPercent: number;
  diskReadRate: number;
  diskWriteRate: number;
  netRxRate: number;
  netTxRate: number;
};

function fromStored(point: MetricsPoint): StatsPoint {
  return {
    at: new Date(point.sampledAt).getTime(),
    cpuPercent: point.cpuPercent,
    memoryPercent: point.memoryPercent,
    diskReadRate: point.diskReadRate,
    diskWriteRate: point.diskWriteRate,
    netRxRate: point.netRxRate,
    netTxRate: point.netTxRate,
  };
}

/**
 * The detail charts' series: the stored history the server sampled every 30s,
 * then the live 5s polls taken since its newest point.
 *
 * Disk and network arrive live as cumulative counters, which plot as a line that
 * only ever rises. They are differenced against the previous poll so the chart
 * shows throughput, the same unit the stored history already uses. A counter
 * that goes backwards means the guest restarted, so that step reads 0 rather
 * than a negative spike.
 */
export function useStatsHistory(
  stats: Stats | undefined,
  memoryMaxBytes: number,
  stored: MetricsPoint[] | undefined,
): StatsPoint[] {
  const [sampleHistory, setHistory] = useState<{ before: Stats | null; sample: string; live: StatsPoint[] }>({
    before: null,
    sample: '',
    live: [],
  });
  const { live } = sampleHistory;

  if (!stats) {
    if (sampleHistory.before || sampleHistory.sample || sampleHistory.live.length > 0) {
      setHistory({ before: null, sample: '', live: [] });
    }
  } else if (stats.sampledAt !== sampleHistory.sample) {
    const { before } = sampleHistory;
    let nextLive = sampleHistory.live;
    if (before) {
      const at = new Date(stats.sampledAt).getTime();
      const elapsed = (at - new Date(before.sampledAt).getTime()) / 1000;
      if (Number.isFinite(elapsed) && elapsed > 0) {
        const rate = (now: number, then: number) => (now < then ? 0 : (now - then) / elapsed);
        const max = memoryMaxBytes || stats.memoryMaxBytes || 0;
        nextLive = [
          ...nextLive,
          {
            at,
            cpuPercent: stats.cpuPercent,
            memoryPercent: max > 0 ? (stats.memoryBytes / max) * 100 : 0,
            diskReadRate: rate(stats.diskReadBytes, before.diskReadBytes),
            diskWriteRate: rate(stats.diskWriteBytes, before.diskWriteBytes),
            netRxRate: rate(stats.netRxBytes, before.netRxBytes),
            netTxRate: rate(stats.netTxBytes, before.netTxBytes),
          },
        ].slice(-MAX_POINTS);
      }
    }
    setHistory({ before: stats, sample: stats.sampledAt, live: nextLive });
  }

  return useMemo(() => {
    const storedPoints = (stored ?? []).map(fromStored);
    const newestStored = storedPoints.at(-1)?.at ?? 0;
    return [...storedPoints, ...live.filter((point) => point.at > newestStored)].slice(-MAX_POINTS);
  }, [stored, live]);
}
