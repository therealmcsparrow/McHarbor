// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { Chart } from '../types';
import { W } from './chart-helpers';
import { Bars, HBar } from './ChartBars';
import { LineChart } from './ChartLine';
import { Donut, Gauge } from './ChartRound';

// The on-screen chart renderer.
//
// Inline SVG, mirroring the semantics the Go chart engine draws into the
// PDF and Word exports: zero baselines, no 3-D, direct labelling, and the
// same six-colour ramp. It is a separate implementation rather than a
// shared one — the geometry lives in Go — so the rule is that both sides
// answer the same questions the same way, not that they share code.

type Props = { chart: Chart };

export function ChartBlock({ chart }: Props) {
  const { t } = useTranslation('reporting');
  const chartLabel = t('chart.aria');
  const height = Math.min(Math.max(chart.height ?? 132, 90), 320);

  const body = useMemo(() => {
    switch (chart.kind) {
      case 'donut':
        return <Donut chart={chart} height={height} />;
      case 'hbar':
        return <HBar chart={chart} height={height} />;
      case 'gauge':
        return <Gauge chart={chart} height={height} />;
      case 'line':
      case 'area':
        return <LineChart chart={chart} height={height} filled={chart.kind === 'area'} />;
      default:
        return <Bars chart={chart} height={height} stacked={chart.kind === 'stackedbar'} />;
    }
  }, [chart, height]);

  return (
    <figure className="my-5">
      {chart.title ? (
        <figcaption className="mb-2 text-[11px] font-semibold tracking-wider text-muted-foreground uppercase">
          {chart.title}
        </figcaption>
      ) : null}
      <div className="overflow-x-auto">
        <svg
          viewBox={`0 0 ${W} ${height}`}
          className="h-auto w-full min-w-[420px]"
          aria-label={chart.title || chartLabel}
        >
          <title>{chart.title || chartLabel}</title>
          {body}
        </svg>
      </div>
      {chart.caption ? <figcaption className="mt-2 text-xs text-muted-foreground">{chart.caption}</figcaption> : null}
    </figure>
  );
}

// --- shared helpers --------------------------------------------------
