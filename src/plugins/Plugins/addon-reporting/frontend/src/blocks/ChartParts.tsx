// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { axisFill, fmt } from './chart-helpers';
import type { Chart } from '../types';
import { seriesColor } from './tokens';

export function Grid({
  x0,
  x1,
  y0,
  y1,
  max,
  unit,
}: {
  x0: number;
  x1: number;
  y0: number;
  y1: number;
  max: number;
  unit: string;
}) {
  const steps = 4;
  return (
    <g className="text-muted-foreground">
      {Array.from({ length: steps + 1 }, (_, i) => {
        const y = y1 - ((y1 - y0) * i) / steps;
        return (
          <g key={y}>
            <line x1={x0} y1={y} x2={x1} y2={y} stroke="currentColor" strokeOpacity={i === 0 ? 0.35 : 0.15} />
            <text x={x0 - 6} y={y + 3} textAnchor="end" fontSize={9} fill={axisFill} opacity={0.75}>
              {fmt((max * i) / steps, unit)}
            </text>
          </g>
        );
      })}
    </g>
  );
}

// --- vertical bars ---------------------------------------------------

export function Legend({ chart, y }: { chart: Chart; y: number }) {
  const starts: number[] = [];
  let x = 8;
  for (const s of chart.series) {
    starts.push(x);
    x += 10 + (s.name ?? '').length * 5.4 + 16;
  }
  return (
    <g className="text-muted-foreground">
      {chart.series.map((s, i) => {
        const label = s.name ?? '';
        const at = starts[i] ?? 0;
        return (
          <g key={at}>
            <rect x={at} y={y - 8} width={8} height={8} rx={2} fill={seriesColor(i, s.status)} />
            <text x={at + 12} y={y} fontSize={10} fill={axisFill}>
              {label}
            </text>
          </g>
        );
      })}
    </g>
  );
}

// --- horizontal bars -------------------------------------------------
