// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Chart, ChartPoint } from '../types';
import { W, arcPath, fmt, truncate } from './chart-helpers';
import { seriesColor } from './tokens';

export function Donut({ chart, height }: { chart: Chart; height: number }) {
  const pts = (chart.series[0]?.points ?? []).filter((p) => p.value > 0);
  const total = pts.reduce((s, p) => s + p.value, 0);
  const r = Math.min(height / 2 - 4, 66);
  const cx = r + 8;
  const cy = height / 2;
  const inner = r * 0.62;

  let angle = 0;
  return (
    <g>
      {total <= 0 ? (
        <circle
          cx={cx}
          cy={cy}
          r={(r + inner) / 2}
          fill="none"
          strokeWidth={r - inner}
          className="text-muted"
          stroke="currentColor"
          strokeOpacity={0.4}
        />
      ) : (
        pts.map((p, i) => {
          const sweep = (360 * p.value) / total;
          const d = arcPath(cx, cy, r, inner, angle, sweep);
          angle += sweep;
          return <path key={p.label + i} d={d} fill={seriesColor(i, p.status)} />;
        })
      )}
      <text
        x={cx}
        y={cy + 2}
        textAnchor="middle"
        fontSize={19}
        fontWeight={700}
        fill="currentColor"
        className="text-foreground"
      >
        {fmt(total)}
      </text>
      {chart.unit ? (
        <text x={cx} y={cy + 16} textAnchor="middle" fontSize={9} fill="currentColor" className="text-muted-foreground">
          {chart.unit}
        </text>
      ) : null}
      <DonutLegend pts={pts} total={total} x={r * 2 + 26} height={height} />
    </g>
  );
}

export function DonutLegend({
  pts,
  total,
  x,
  height,
}: {
  pts: ChartPoint[];
  total: number;
  x: number;
  height: number;
}) {
  const rowH = Math.min(20, height / Math.max(pts.length, 1));
  const startY = (height - rowH * pts.length) / 2;
  return (
    <g>
      {pts.map((p, i) => {
        const y = startY + rowH * i + rowH / 2;
        const share = total > 0 ? (100 * p.value) / total : 0;
        return (
          <g key={p.label + i}>
            <rect x={x} y={y - 4} width={8} height={8} rx={2} fill={seriesColor(i, p.status)} />
            <text x={x + 14} y={y + 3.5} fontSize={11} fill="currentColor" className="text-foreground">
              {truncate(p.label, 24)}
            </text>
            <text
              x={W - 54}
              y={y + 3.5}
              fontSize={11}
              fontWeight={600}
              textAnchor="end"
              fill="currentColor"
              className="text-foreground"
            >
              {fmt(p.value)}
            </text>
            <text
              x={W}
              y={y + 3.5}
              fontSize={10}
              textAnchor="end"
              fill="currentColor"
              className="text-muted-foreground"
            >
              {fmt(share)}%
            </text>
          </g>
        );
      })}
    </g>
  );
}

/** Annular sector path; angles are degrees clockwise from twelve o'clock. */
export function Gauge({ chart, height }: { chart: Chart; height: number }) {
  const p = chart.series[0]?.points[0];
  if (!p) return null;
  const max = chart.max && chart.max > 0 ? chart.max : 100;
  const frac = Math.max(0, Math.min(1, p.value / max));
  const r = Math.min(W / 6, height / 1.55);
  const cx = W / 2;
  const cy = r + 2;
  const thickness = Math.max(6, r * 0.2);

  return (
    <g>
      <path
        d={arcPath(cx, cy, r, r - thickness, 240, 240)}
        className="text-muted"
        fill="currentColor"
        fillOpacity={0.4}
      />
      {frac > 0 ? (
        <path d={arcPath(cx, cy, r, r - thickness, 240, 240 * frac)} fill={seriesColor(0, p.status)} />
      ) : null}
      <text
        x={cx}
        y={cy + r * 0.14}
        textAnchor="middle"
        fontSize={r * 0.6}
        fontWeight={700}
        fill="currentColor"
        className="text-foreground"
      >
        {fmt(p.value)}
      </text>
      {p.label ? (
        <text
          x={cx}
          y={cy + r * 0.52}
          textAnchor="middle"
          fontSize={11}
          fill="currentColor"
          className="text-muted-foreground"
        >
          {p.label}
        </text>
      ) : null}
    </g>
  );
}
