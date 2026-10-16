// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// widget-donut — the donut-plus-legend body three widgets share.
//
// The old version was a fixed 140px ring beside a legend, and in a 4×3
// tile that is a ring in the top-left and 200px of nothing. This one lets
// the ring take the whole height (radii are percentages, the container is
// measured) and turns the legend into rows that carry their own share bar,
// so the legend reads on its own even when the ring is small.
//
// Kept apart from widget-kit so plugin bundles that import the kit do not
// also pull recharts.

import { cssVars } from '@resources/utils/cssVars';
import { PlainButton } from '@resources/components/ui/PlainButton';
import type { ReactNode } from 'react';
import { PieChart, Pie, Cell } from 'recharts';
import { MeasuredResponsiveContainer } from '@resources/components/MeasuredResponsiveContainer';
import { cn } from '@resources/utils/cn';
import { TONE_SOLID, type Tone } from './widget-kit';

export type DonutSegment = {
  key: string;
  label: string;
  value: number;
  tone: Tone;
  /** What to print for the value; defaults to the number. */
  display?: string;
  onClick?: () => void;
};

/** CSS colour for a tone, for the SVG cells. */
const TONE_VAR: Record<Tone, string> = {
  success: 'var(--status-success)',
  warning: 'var(--status-warning)',
  danger: 'var(--status-danger)',
  critical: 'var(--status-critical)',
  info: 'var(--status-info)',
  neutral: 'var(--status-neutral)',
  primary: 'var(--primary)',
};

export function DonutPanel({
  segments,
  centerValue,
  centerLabel,
}: {
  segments: DonutSegment[];
  centerValue: ReactNode;
  centerLabel: string;
}) {
  const visible = segments.filter((s) => s.value > 0);
  const total = visible.reduce((sum, s) => sum + s.value, 0);

  return (
    <div className="flex min-h-0 flex-1 items-stretch gap-4 px-4 pb-3">
      <div className="relative min-h-0 min-w-0 flex-1">
        <MeasuredResponsiveContainer>
          <PieChart>
            <Pie
              data={visible}
              cx="50%"
              cy="50%"
              innerRadius="62%"
              outerRadius="92%"
              paddingAngle={2}
              dataKey="value"
              stroke="none"
              isAnimationActive={false}
            >
              {visible.map((s) => (
                <Cell key={s.key} fill={TONE_VAR[s.tone]} />
              ))}
            </Pie>
          </PieChart>
        </MeasuredResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-xl leading-none font-semibold text-foreground tabular-nums">{centerValue}</span>
          <span className="mt-1 text-[10px] tracking-wide text-muted-foreground uppercase">{centerLabel}</span>
        </div>
      </div>

      <ul className="flex w-[46%] shrink-0 flex-col justify-center gap-2">
        {visible.map((s) => {
          const share = total > 0 ? s.value / total : 0;
          const row = (
            <>
              <span className="flex items-center justify-between gap-2 text-xs">
                <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
                  <span aria-hidden="true" className={cn('size-2 shrink-0 rounded-full', TONE_SOLID[s.tone])} />
                  <span className="truncate">{s.label}</span>
                </span>
                <span className="shrink-0 font-medium text-foreground tabular-nums">{s.display ?? s.value}</span>
              </span>
              <span className="mt-1 block h-1 w-full overflow-hidden rounded-full bg-glass-2" aria-hidden="true">
                <span
                  className={cn('block h-full w-(--w) rounded-full', TONE_SOLID[s.tone])}
                  // The fill is the data; it travels as a CSS variable the class reads.
                  style={cssVars({ w: `${Math.round(share * 100)}%` })}
                />
              </span>
            </>
          );
          return (
            <li key={s.key}>
              {s.onClick ? (
                <PlainButton
                  type="button"
                  onClick={s.onClick}
                  className="block w-full rounded-md px-1 py-0.5 text-left transition-colors hover:bg-glass-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  {row}
                </PlainButton>
              ) : (
                <div className="px-1 py-0.5">{row}</div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
