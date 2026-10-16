// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Chart } from '../types';
import { W, axisFill, categories, fmt, maxOf, truncate } from './chart-helpers';
import { Grid, Legend } from './ChartParts';
import { seriesColor } from './tokens';

export function Bars({ chart, height, stacked }: { chart: Chart; height: number; stacked: boolean }) {
  const max = maxOf(chart, stacked);
  const cats = categories(chart);
  const legendH = chart.show_legend && chart.series.length > 1 ? 20 : 0;
  const x0 = 46;
  const x1 = W - 8;
  const y0 = 12;
  const y1 = height - 22 - legendH;
  const slot = (x1 - x0) / Math.max(cats.length, 1);
  const groupW = slot * 0.68;
  const barW = stacked ? groupW : groupW / Math.max(chart.series.length, 1);

  return (
    <g>
      <Grid x0={x0} x1={x1} y0={y0} y1={y1} max={max} unit={chart.unit ?? ''} />
      {chart.has_target && chart.target !== undefined && chart.target <= max ? (
        <line
          x1={x0}
          y1={y1 - (y1 - y0) * (chart.target / max)}
          x2={x1}
          y2={y1 - (y1 - y0) * (chart.target / max)}
          stroke="currentColor"
          strokeDasharray="4 3"
          strokeOpacity={0.55}
          className="text-foreground"
        />
      ) : null}
      {cats.map((cat, ci) => {
        const cx = x0 + slot * ci + slot / 2;
        let stackTop = y1;
        return (
          <g key={cat + ci}>
            {chart.series.map((s, si) => {
              const v = Math.max(s.points[ci]?.value ?? 0, 0);
              const h = max > 0 ? (y1 - y0) * (v / max) : 0;
              const bx = stacked ? cx - groupW / 2 : cx - groupW / 2 + barW * si;
              const by = stacked ? stackTop - h : y1 - h;
              if (stacked) stackTop = by;
              return (
                <rect
                  key={si}
                  x={bx}
                  y={by}
                  width={Math.max(barW - 1.5, 1)}
                  height={Math.max(h, v > 0 ? 1 : 0)}
                  rx={stacked ? 0 : 2}
                  fill={seriesColor(si, s.status, s.points[ci]?.status)}
                />
              );
            })}
            <text
              x={cx}
              y={y1 + 13}
              textAnchor="middle"
              fontSize={9}
              fill={axisFill}
              className="text-muted-foreground"
              opacity={0.85}
            >
              {cat}
            </text>
          </g>
        );
      })}
      {legendH ? <Legend chart={chart} y={height - 6} /> : null}
    </g>
  );
}

export function HBar({ chart, height }: { chart: Chart; height: number }) {
  const pts = chart.series[0]?.points ?? [];
  const max = maxOf(chart, false);
  const labelW = 150;
  const valueW = chart.show_values ? 52 : 8;
  const x0 = labelW + 10;
  const x1 = W - valueW;
  const rowH = height / Math.max(pts.length, 1);
  const barH = Math.min(rowH * 0.55, 16);

  return (
    <g>
      {pts.map((p, i) => {
        const cy = rowH * i + rowH / 2;
        const w = max > 0 ? (x1 - x0) * (p.value / max) : 0;
        return (
          <g key={p.label + i}>
            <text
              x={labelW}
              y={cy + 3.5}
              textAnchor="end"
              fontSize={10.5}
              fill="currentColor"
              className="text-foreground"
            >
              {truncate(p.label, 26)}
            </text>
            <rect
              x={x0}
              y={cy - barH / 2}
              width={x1 - x0}
              height={barH}
              rx={barH / 2}
              className="text-muted"
              fill="currentColor"
              fillOpacity={0.35}
            />
            <rect
              x={x0}
              y={cy - barH / 2}
              width={Math.max(w, p.value > 0 ? barH * 0.6 : 0)}
              height={barH}
              rx={barH / 2}
              fill={seriesColor(0, p.status, chart.series[0]?.status)}
            />
            {chart.show_values ? (
              <text
                x={x1 + 6}
                y={cy + 3.5}
                fontSize={10.5}
                fontWeight={600}
                fill="currentColor"
                className="text-muted-foreground"
              >
                {fmt(p.value, chart.unit ?? '')}
              </text>
            ) : null}
          </g>
        );
      })}
    </g>
  );
}

// --- donut -----------------------------------------------------------
