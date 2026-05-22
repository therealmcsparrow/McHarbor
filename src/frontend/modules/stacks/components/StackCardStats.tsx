// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useEffect } from 'react';
import type { StackInfo } from '../hooks/useStacks';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import { Sparkline } from '@resources/components/Sparkline';

const MAX_HISTORY = 30;

export type AggregatedStats = {
  cpuPercent: number;
  memUsage: number;
  memLimit: number;
  netRx: number;
  netTx: number;
};

export function aggregateStats(
  services: StackInfo['services'],
  statsMap?: Map<string, BulkContainerMetric>,
): AggregatedStats | null {
  if (!statsMap) return null;

  let cpu = 0;
  let mem = 0;
  let memLimit = 0;
  let netRx = 0;
  let netTx = 0;
  let found = false;

  for (const svc of services) {
    if (!svc.containerId) continue;
    const s = statsMap.get(svc.containerId);
    if (!s) continue;
    found = true;
    cpu += s.cpuPercent;
    mem += s.memUsage;
    memLimit += s.memLimit;
    netRx += s.netRx;
    netTx += s.netTx;
  }

  return found ? { cpuPercent: cpu, memUsage: mem, memLimit: memLimit, netRx, netTx } : null;
}

export type StatsHistory = {
  cpu: number[];
  mem: number[];
  netRx: number[];
  netTx: number[];
};

export function useStatsHistory(stats: AggregatedStats | null): StatsHistory {
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

type MiniChartProps = {
  label: string;
  value: string;
  data: number[];
  color: string;
};

export function MiniChart({ label, value, data, color }: MiniChartProps) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between">
        <span className="text-[10px] text-muted-foreground">{label}</span>
        <span className="text-[10px] tabular-nums text-foreground">{value}</span>
      </div>
      <div className="h-5">
        <Sparkline data={data.length >= 2 ? data : [0, 0]} width={120} height={20} color={color} />
      </div>
    </div>
  );
}
