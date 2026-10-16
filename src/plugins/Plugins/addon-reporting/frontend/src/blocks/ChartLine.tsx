// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Chart } from '../types';
import { W, axisFill, categories, maxOf } from './chart-helpers';
import { Grid } from './ChartParts';
import { seriesColor } from './tokens';

export function LineChart({ chart, height, filled }: { chart: Chart; height: number; filled: boolean }) {
  const max = maxOf(chart, false);
  const cats = categories(chart);
  const x0 = 46;
  const x1 = W - 8;
  const y0 = 10;
  const y1 = height - 22;
  const step = (x1 - x0) / Math.max(cats.length - 1, 1);

  return (
    <g>
      <Grid x0={x0} x1={x1} y0={y0} y1={y1} max={max} unit={chart.unit ?? ''} />
      {chart.series.map((s, si) => {
        const color = seriesColor(si, s.status);
        const pts = s.points.map((p, i) => ({
          x: x0 + step * i,
          y: max > 0 ? y1 - (y1 - y0) * (Math.max(p.value, 0) / max) : y1,
        }));
        const first = pts[0];
        const last = pts[pts.length - 1];
        if (pts.length < 2 || !first || !last) return null;
        const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
        return (
          <g key={si}>
            {filled ? (
              <path d={`${line} L ${last.x} ${y1} L ${first.x} ${y1} Z`} fill={color} fillOpacity={0.14} />
            ) : null}
            <path d={line} fill="none" stroke={color} strokeWidth={1.8} strokeLinejoin="round" strokeLinecap="round" />
            {pts.length <= 14
              ? pts.map((p) => <circle key={`${p.x}:${p.y}`} cx={p.x} cy={p.y} r={2.4} fill={color} />)
              : null}
          </g>
        );
      })}
      {[0, Math.floor(cats.length / 2), cats.length - 1]
        .filter((i, idx, arr) => i >= 0 && i < cats.length && arr.indexOf(i) === idx)
        .map((cat, k) => (
          <text
            key={cat}
            x={x0 + step * cat}
            y={y1 + 14}
            textAnchor={k === 0 ? 'start' : k === 2 ? 'end' : 'middle'}
            fontSize={9}
            fill={axisFill}
            className="text-muted-foreground"
          >
            {cats[cat]}
          </text>
        ))}
    </g>
  );
}

// --- gauge -----------------------------------------------------------
