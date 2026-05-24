// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { CronExpressionParser } from 'cron-parser';
import { normalizeTimezone } from '@modules/environments/timezones';

type CronSchedulePreviewProps = {
  expression: string;
  timezone?: string | null;
  className?: string;
};

function resolvePreviewTimezone(timezone?: string | null): string {
  if (timezone && timezone.trim()) {
    return normalizeTimezone(timezone);
  }

  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

function formatOccurrence(date: Date, timezone: string): string {
  return new Intl.DateTimeFormat(undefined, {
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    month: 'short',
    timeZoneName: 'short',
    timeZone: timezone,
    year: 'numeric',
  }).format(date);
}

function getOffsetToken(date: Date, timezone: string): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    timeZoneName: 'shortOffset',
    hour: '2-digit',
  }).formatToParts(date);

  return parts.find((part) => part.type === 'timeZoneName')?.value ?? timezone;
}

function describeSchedule(expression: string, t: ReturnType<typeof useTranslation>['t']) {
  const parts = expression.trim().split(/\s+/);
  if (parts.length !== 5) {
    return t('workflows.summaryCustom');
  }

  const minute = parts[0] ?? '*';
  const hour = parts[1] ?? '*';
  const dayOfMonth = parts[2] ?? '*';
  const month = parts[3] ?? '*';
  const dayOfWeek = parts[4] ?? '*';
  if (minute === '*' && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('workflows.summaryEveryMinute');
  }

  if (/^\*\/\d+$/.test(minute) && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('workflows.summaryEveryNMinutes', { count: minute.slice(2) });
  }

  if (/^\d+$/.test(minute) && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('workflows.summaryHourlyAt', { minute: minute.padStart(2, '0') });
  }

  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('workflows.summaryDailyAt', {
      time: `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`,
    });
  }

  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && dayOfMonth === '*' && month === '*' && /^\d+$/.test(dayOfWeek)) {
    const date = new Date(Date.UTC(2026, 0, 4 + Number(dayOfWeek), 12, 0, 0));
    const weekday = new Intl.DateTimeFormat(undefined, { weekday: 'long' }).format(date);
    return t('workflows.summaryWeeklyOn', {
      weekday,
      time: `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`,
    });
  }

  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && /^\d+$/.test(dayOfMonth) && month === '*' && dayOfWeek === '*') {
    return t('workflows.summaryMonthlyOn', {
      day: dayOfMonth,
      time: `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`,
    });
  }

  return t('workflows.summaryCustom');
}

export function CronSchedulePreviewImpl({
  expression,
  timezone,
  className,
}: CronSchedulePreviewProps) {
  const { t } = useTranslation('common');

  const preview = useMemo(() => {
    const trimmed = expression.trim();
    const resolvedTimezone = resolvePreviewTimezone(timezone);

    try {
      const parsed = CronExpressionParser.parse(trimmed, {
        currentDate: new Date(),
        tz: resolvedTimezone,
      });

      const dates = parsed.take(5).map((run) => run.toDate());
      const runs = dates.map((run) => formatOccurrence(run, resolvedTimezone));
      const offsets = new Set(dates.map((run) => getOffsetToken(run, resolvedTimezone)));

      return {
        timezone: resolvedTimezone,
        runs,
        error: null as string | null,
        summary: describeSchedule(trimmed, t),
        dstWarning: offsets.size > 1 ? t('workflows.dstChangeWarning') : null,
      };
    } catch {
      return {
        timezone: resolvedTimezone,
        runs: [],
        error: t('workflows.invalidCronExpression'),
        summary: null as string | null,
        dstWarning: null as string | null,
      };
    }
  }, [expression, t, timezone]);

  return (
    <div className={className ?? 'mt-2 rounded-md border border-border/60 bg-muted/30 p-2'}>
      <div className="flex items-center justify-between gap-2">
        <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('workflows.nextRuns')}
        </p>
        <p className="text-[10px] text-muted-foreground">
          {t('workflows.timezonePreview', { timezone: preview.timezone })}
        </p>
      </div>

      {preview.error ? (
        <p className="mt-1 text-xs text-destructive">{preview.error}</p>
      ) : (
        <div className="mt-1 space-y-1">
          {preview.summary && (
            <p className="text-xs text-muted-foreground">{t('workflows.scheduleSummary', { summary: preview.summary })}</p>
          )}
          {preview.dstWarning && (
            <p className="text-xs text-amber-500">{preview.dstWarning}</p>
          )}
          {preview.runs.map((run) => (
            <div key={run} className="font-mono text-[11px] text-foreground">
              {run}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
