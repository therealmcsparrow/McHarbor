// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Chart } from '../types';

export const W = 760;

export function maxOf(chart: Chart, stacked: boolean): number {
  if (chart.max && chart.max > 0) return chart.max;
  let m = 0;
  if (stacked) {
    const n = Math.max(...chart.series.map((s) => s.points.length), 0);
    for (let i = 0; i < n; i += 1) {
      m = Math.max(
        m,
        chart.series.reduce((sum, s) => sum + (s.points[i]?.value ?? 0), 0),
      );
    }
  } else {
    for (const s of chart.series) for (const p of s.points) m = Math.max(m, p.value);
  }
  if (chart.has_target && chart.target) m = Math.max(m, chart.target);
  return niceMax(m);
}

export function niceMax(v: number): number {
  if (v <= 0) return 1;
  const pow = Math.pow(10, Math.floor(Math.log10(v)));
  const f = v / pow;
  if (f <= 1) return pow;
  if (f <= 2) return 2 * pow;
  if (f <= 2.5) return 2.5 * pow;
  if (f <= 4) return 4 * pow;
  if (f <= 5) return 5 * pow;
  if (f <= 8) return 8 * pow;
  return 10 * pow;
}

export function fmt(v: number, unit = ''): string {
  const abs = Math.abs(v);
  let s: string;
  if (abs >= 10000) s = new Intl.NumberFormat('en-GB').format(Math.round(v)).replace(/,/g, ' ');
  else if (Number.isInteger(v)) s = String(v);
  else s = v.toFixed(1);
  return s + unit;
}

export function categories(chart: Chart): string[] {
  const out: string[] = [];
  for (const s of chart.series) {
    s.points.forEach((p, i) => {
      if (out[i] === undefined) out[i] = p.label;
    });
  }
  return out;
}

export const axisFill = 'currentColor';
export function arcPath(cx: number, cy: number, rOuter: number, rInner: number, start: number, sweep: number): string {
  const s = Math.min(sweep, 359.99);
  const rad = (d: number) => ((90 - d) * Math.PI) / 180;
  const p = (radius: number, deg: number) => [cx + radius * Math.cos(rad(deg)), cy - radius * Math.sin(rad(deg))];
  const [x1, y1] = p(rOuter, start);
  const [x2, y2] = p(rOuter, start + s);
  const [x3, y3] = p(rInner, start + s);
  const [x4, y4] = p(rInner, start);
  const large = s > 180 ? 1 : 0;
  return [
    `M ${x1} ${y1}`,
    `A ${rOuter} ${rOuter} 0 ${large} 1 ${x2} ${y2}`,
    `L ${x3} ${y3}`,
    `A ${rInner} ${rInner} 0 ${large} 0 ${x4} ${y4}`,
    'Z',
  ].join(' ');
}

// --- line / area -----------------------------------------------------

export function truncate(s: string, max: number): string {
  return s.length <= max ? s : `${s.slice(0, max - 1)}…`;
}
