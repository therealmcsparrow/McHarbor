// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { MetricsPoint, Stats } from '../types';
import { useStatsHistory } from './useStatsHistory';

const stored = (sampledAt: string, cpuPercent: number): MetricsPoint => ({
  sampledAt,
  cpuPercent,
  memoryPercent: 50,
  diskReadRate: 10,
  diskWriteRate: 0,
  netRxRate: 20,
  netTxRate: 0,
  memoryBytes: 512,
  memoryMaxBytes: 1024,
});

const live = (sampledAt: string, cpuPercent: number, netRxBytes: number): Stats => ({
  vmId: 'vm',
  cpuPercent,
  memoryBytes: 512,
  memoryMaxBytes: 1024,
  diskReadBytes: 0,
  diskWriteBytes: 0,
  netRxBytes,
  netTxBytes: 0,
  sampledAt,
});

describe('useStatsHistory', () => {
  // The bug this guards: the charts were built only from polls taken while the
  // page was open, so every visit started from an empty graph.
  it('opens on the stored history before any live poll', () => {
    const history = [stored('2026-10-02T08:00:30Z', 10), stored('2026-10-02T08:01:00Z', 20)];
    const { result } = renderHook(() => useStatsHistory(undefined, 1024, history));
    expect(result.current.map((point) => point.cpuPercent)).toEqual([10, 20]);
  });

  it('appends live rates newer than the stored history, never older ones', () => {
    const history = [stored('2026-10-02T08:01:00Z', 20)];
    let stats = live('2026-10-02T08:00:50Z', 1, 0);
    const { result, rerender } = renderHook(() => useStatsHistory(stats, 1024, history));

    act(() => {
      stats = live('2026-10-02T08:00:55Z', 99, 500);
    });
    rerender();
    act(() => {
      stats = live('2026-10-02T08:01:05Z', 30, 1_500);
    });
    rerender();

    expect(result.current.map((point) => point.cpuPercent)).toEqual([20, 30]);
    expect(result.current.at(-1)?.netRxRate).toBe(100);
  });
});
