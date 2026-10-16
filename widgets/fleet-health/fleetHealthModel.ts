// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { IconAlertTriangle } from '@tabler/icons-react';

/** A snapshot that has not run in this long is overdue. Matches needs-attention. */
export const SNAPSHOT_OVERDUE_MS = 36 * 60 * 60 * 1000;

/** The banner shows two cards; the headline still counts every item. */
export const MAX_CARDS = 2;

export type SegmentKey = 'running' | 'unhealthy' | 'paused' | 'exited';

export type Segment = {
  key: SegmentKey;
  count: number;
  /** Facet the container list should open with. */
  facet: string;
  /** Solid fill for the bar. */
  barClass: string;
  /** Readable text on that fill. */
  barTextClass: string;
  /** Legend dot. */
  dotClass: string;
};

export type AttentionItem = {
  id: string;
  /** Lower sorts first — the most consequential thing leads. */
  rank: number;
  icon: typeof IconAlertTriangle;
  /** Tint applied to both the icon and its rounded tile. */
  tone: 'danger' | 'warning' | 'info' | 'neutral' | 'success';
  title: string;
  detail: string;
  to: string;
};

export type FleetHealth = {
  isLoading: boolean;
  isError: boolean;
  total: number;
  /** Containers Docker reports as running, unhealthy ones included. */
  running: number;
  segments: Segment[];
  /** Hard down — dead or stuck restarting. Drives the first headline clause. */
  down: number;
  /** Three short facts, already joined-ready. */
  facts: string[];
  items: AttentionItem[];
};

export function containerName(names: string[] | undefined, id: string): string {
  return names?.[0]?.replace(/^\//, '') ?? id;
}

/** Milliseconds since the plan last ran, or null when it never has. */
export function ageSince(iso: string | undefined): number | null {
  if (!iso) return null;
  const delta = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(delta) || delta < 0) return null;
  return delta;
}

/** Builds the four bar segments; the counts must add up to the fleet total. */
export function buildSegments(running: number, unhealthy: number, paused: number, exited: number): Segment[] {
  return [
    {
      key: 'running',
      // Unhealthy containers are still running; the bar shows them apart
      // so the four segments still add up to the total.
      count: Math.max(running - unhealthy, 0),
      facet: 'running',
      barClass: 'bg-success',
      barTextClass: 'text-success-foreground',
      dotClass: 'bg-success',
    },
    {
      key: 'unhealthy',
      count: unhealthy,
      facet: 'unhealthy',
      barClass: 'bg-warning',
      barTextClass: 'text-warning-foreground',
      dotClass: 'bg-warning',
    },
    {
      key: 'paused',
      count: paused,
      facet: 'paused',
      // No paired -fg on the chart ramp; --background inverts the same way.
      barClass: 'bg-chart-3',
      barTextClass: 'text-background',
      dotClass: 'bg-chart-3',
    },
    {
      key: 'exited',
      count: exited,
      facet: 'exited',
      barClass: 'bg-neutral',
      barTextClass: 'text-neutral-foreground',
      dotClass: 'bg-neutral',
    },
  ];
}
