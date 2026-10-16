// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// APIKeyUsageChart — request counts over time for one key, in the same
// visual language as container resource history.
//
// The backend returns zero-filled buckets on purpose: a sparse series would
// let the area chart draw a straight line between two distant samples,
// sloping through hours in which the key did nothing at all.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { IconChartLine } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { formatDate, formatDateCustom } from '@resources/utils/format';
import { formatCount } from '../utils/formatCount';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatsAreaChart } from '@resources/components/StatsAreaChart';
import { useAPIKeyUsageSeries, USAGE_RANGES, type UsageRange } from '../hooks/useAPIKeyUsage';

type APIKeyUsageChartProps = {
  apiKeyId: string;
};

type ChartRow = {
  label: string;
  count: number;
};

export function APIKeyUsageChart({ apiKeyId }: APIKeyUsageChartProps) {
  const { t } = useTranslation('security');
  const [range, setRange] = useState<UsageRange>('24h');
  const { data: series, isLoading, isError, error } = useAPIKeyUsageSeries(apiKeyId, range);

  const rows = useMemo<ChartRow[]>(
    () =>
      (series?.points ?? []).map((point) => ({
        label: formatPointLabel(point.timestamp, range),
        count: point.count,
      })),
    [series, range],
  );

  const summary = [
    { key: 'total', value: series?.total ?? 0 },
    { key: 'success', value: series?.successCount ?? 0 },
    { key: 'clientErrors', value: series?.clientErrorCount ?? 0 },
    { key: 'serverErrors', value: series?.serverErrorCount ?? 0 },
    { key: 'uniqueClients', value: series?.uniqueClients ?? 0 },
  ] as const;

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
            <IconChartLine className="size-4 text-muted-foreground" />
            {t('apiKeys.usage.title')}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">{t('apiKeys.usage.description')}</p>
        </div>

        <fieldset className="m-0 flex min-w-0 flex-wrap gap-1 border-0 p-0" aria-label={t('apiKeys.usage.rangeLabel')}>
          {USAGE_RANGES.map((option) => (
            <Button
              key={option}
              type="button"
              size="sm"
              variant={option === range ? 'default' : 'ghost'}
              onClick={() => setRange(option)}
              aria-pressed={option === range}
            >
              {t(`apiKeys.usage.range.${option}`)}
            </Button>
          ))}
        </fieldset>
      </div>

      <dl className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {summary.map((card) => (
          <div key={card.key} className="rounded-md border border-border p-3">
            <dt className="text-xs text-muted-foreground">{t(`apiKeys.usage.${card.key}`)}</dt>
            <dd className="mt-1 text-lg font-semibold text-foreground">{formatCount(card.value)}</dd>
          </div>
        ))}
        <div className="rounded-md border border-border p-3">
          <dt className="text-xs text-muted-foreground">{t('apiKeys.usage.averageDuration')}</dt>
          <dd className="mt-1 text-lg font-semibold text-foreground">
            {formatCount(Math.round(series?.averageDurationMs ?? 0))}
            <span className="ml-1 text-xs font-normal text-muted-foreground">{t('apiKeys.usage.ms')}</span>
          </dd>
        </div>
      </dl>

      {isLoading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Spinner size="sm" />
          {t('apiKeys.usage.loading')}
        </div>
      ) : isError ? (
        <ErrorState variant="banner" title={t('apiKeys.usage.error')} error={error} />
      ) : (series?.total ?? 0) === 0 ? (
        <div className="rounded-md border border-dashed border-border px-4 py-8 text-center">
          <p className="text-sm text-muted-foreground">{t('apiKeys.usage.empty')}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t('apiKeys.usage.emptyHint')}</p>
        </div>
      ) : (
        // A definite height, not a min-height: the measured responsive
        // container sizes itself with h-full and withholds its content until
        // it measures a non-zero box, so an auto-height parent stays blank.
        <div className="flex h-72 flex-col">
          <StatsAreaChart
            data={rows}
            xAxisKey="label"
            dataKey="count"
            formatValue={(value) => formatCount(Math.round(value))}
            label={t('apiKeys.usage.requests')}
          />
        </div>
      )}

      {series?.coveredFrom && (
        <p className="mt-3 text-xs text-muted-foreground">
          {t('apiKeys.usage.coverage', {
            from: formatDate(series.coveredFrom),
            to: formatDate(series.coveredTo ?? series.coveredFrom),
          })}
        </p>
      )}
    </div>
  );
}

/**
 * Long ranges get a date: a bare clock time repeats every day and would make
 * a 30-day axis unreadable.
 */
function formatPointLabel(timestamp: string, range: UsageRange): string {
  const parsed = new Date(timestamp);
  if (Number.isNaN(parsed.getTime())) return timestamp;

  const longRange = range === '7d' || range === '30d' || range === '90d';
  return longRange
    ? formatDateCustom(parsed, { month: 'short', day: 'numeric', hour: '2-digit' })
    : formatDateCustom(parsed, { hour: '2-digit', minute: '2-digit' });
}
