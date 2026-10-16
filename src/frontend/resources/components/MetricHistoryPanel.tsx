// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChartLine } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { formatBytes, formatDateCustom } from '@resources/utils/format';
import { HISTORY_RANGES, type HistoryRange, type MetricSeries } from '@resources/hooks/useMetricsHistory';
import {
  HISTORY_METRICS,
  METRIC_CHARTS,
  type ChartRow,
  type HistoryMetric,
} from '@resources/components/history-metrics';

type MetricHistoryPanelProps = {
  /** The stored series to draw. Undefined while loading or on error. */
  series: MetricSeries | undefined;
  isLoading: boolean;
  range: HistoryRange;
  onRangeChange: (range: HistoryRange) => void;
  metric: HistoryMetric;
  onMetricChange?: (metric: HistoryMetric) => void;
  /**
   * Render the in-panel metric switcher. Wanted when there are no live stat
   * tiles to click - a stopped container, or the host panel, which has no
   * tiles of its own.
   */
  showMetricSwitcher?: boolean;
  className?: string;
};

/**
 * The rendering half of a stored-metric history panel: range switcher, optional
 * metric switcher, and the chart.
 *
 * Extracted from `ContainerHistoryPanel` so the host series can be drawn by the
 * same code. Everything here was already generic over a `MetricSeries`; only
 * the hook call was container-specific, so the two callers now differ by which
 * query they run and nothing else.
 *
 * The `containers` namespace is used deliberately: every `history.*` key
 * already lives there, and copying fifteen of them into another namespace to
 * satisfy a module boundary would be worse than borrowing one.
 */
export function MetricHistoryPanel({
  series,
  isLoading,
  range,
  onRangeChange,
  metric,
  onMetricChange,
  showMetricSwitcher = false,
  className,
}: MetricHistoryPanelProps) {
  const { t } = useTranslation('containers');

  const rows = useMemo<ChartRow[]>(() => {
    if (!series?.points) return [];
    return series.points.map((p) => ({
      label: formatPointLabel(p.timestamp, range),
      cpu: round(p.cpuPercent),
      memory: p.memUsed,
      // Throughput, not the cumulative counter: plotting netRx itself draws a
      // line that only ever rises and says nothing about current activity.
      netRx: p.netRxDelta,
      netTx: p.netTxDelta,
      blockRead: p.blockReadDelta,
      blockWrite: p.blockWriteDelta,
      gpu: p.gpuPercent,
    }));
  }, [series, range]);

  // A metric is offered only when the series carries data for it. GPU is the
  // only one that can be genuinely absent - the host either has GPU telemetry
  // or it does not - so the filter is scoped to it rather than hiding a CPU
  // tile during a slow first load.
  const hasGpuData = rows.some((row) => typeof row.gpu === 'number');
  const availableMetrics = HISTORY_METRICS.filter((option) => option !== 'gpu' || hasGpuData);

  const chart = METRIC_CHARTS[metric];

  const bucketLabel =
    series && series.bucketSeconds > 60
      ? t('history.resolutionMinutes', { minutes: Math.round(series.bucketSeconds / 60) })
      : t('history.resolutionSeconds', { seconds: series?.bucketSeconds ?? 30 });

  return (
    <div className={`mc-glass rounded-lg p-6 ${className ?? ''}`}>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
            <IconChartLine className="size-4 text-muted-foreground" />
            {t('history.title')}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {rows.length > 0 ? bucketLabel : t('history.description')}
          </p>
        </div>

        <div className="flex flex-col items-end gap-2">
          {showMetricSwitcher && onMetricChange && (
            <fieldset className="m-0 flex min-w-0 flex-wrap gap-1 border-0 p-0" aria-label={t('history.metricLabel')}>
              {availableMetrics.map((option) => (
                <Button
                  key={option}
                  type="button"
                  size="sm"
                  variant={option === metric ? 'default' : 'ghost'}
                  onClick={() => onMetricChange(option)}
                  aria-pressed={option === metric}
                >
                  {t(METRIC_CHARTS[option].titleKey)}
                </Button>
              ))}
            </fieldset>
          )}

          <fieldset className="m-0 flex min-w-0 flex-wrap gap-1 border-0 p-0" aria-label={t('history.rangeLabel')}>
            {HISTORY_RANGES.map((option) => (
              <Button
                key={option}
                type="button"
                size="sm"
                variant={option === range ? 'default' : 'ghost'}
                onClick={() => onRangeChange(option)}
                aria-pressed={option === range}
              >
                {t(`history.range.${option}`)}
              </Button>
            ))}
          </fieldset>
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Spinner size="sm" />
          {t('history.loading')}
        </div>
      ) : rows.length === 0 ? (
        <div className="rounded-md border border-dashed border-border px-4 py-8 text-center">
          <p className="text-sm text-muted-foreground">{t('history.empty')}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t('history.emptyHint')}</p>
        </div>
      ) : (
        // A definite height, not a min-height: MeasuredResponsiveContainer sizes
        // itself with h-full and withholds its content until it measures a
        // non-zero box, so an auto-height parent leaves it permanently blank.
        <div className="flex h-72 flex-col">
          <p className="mb-2 text-xs font-medium text-muted-foreground">{t(chart.titleKey)}</p>
          <StatsAreaChart
            data={rows}
            xAxisKey="label"
            dataKey={chart.dataKey}
            secondaryDataKey={chart.secondaryDataKey}
            color={chart.color}
            secondaryColor={chart.secondaryColor}
            formatValue={chart.bytes ? formatBytes : (v) => `${v.toFixed(1)}%`}
            label={t(chart.labelKey)}
            secondaryLabel={chart.secondaryLabelKey ? t(chart.secondaryLabelKey) : undefined}
          />
        </div>
      )}
    </div>
  );
}

/**
 * Stored timestamps are UTC in the database's own layout rather than RFC3339,
 * so they are normalised before parsing. Long ranges get a date because a bare
 * clock time repeats every day and would make a 30-day axis unreadable.
 */
function formatPointLabel(timestamp: string, range: HistoryRange): string {
  const parsed = new Date(timestamp.includes('T') ? timestamp : `${timestamp.replace(' ', 'T')}Z`);
  if (Number.isNaN(parsed.getTime())) return timestamp;

  const longRange = range === '7d' || range === '30d' || range === '90d';
  return longRange
    ? formatDateCustom(parsed, { month: 'short', day: 'numeric', hour: '2-digit' })
    : formatDateCustom(parsed, { hour: '2-digit', minute: '2-digit' });
}

function round(value: number): number {
  return Math.round(value * 10) / 10;
}
