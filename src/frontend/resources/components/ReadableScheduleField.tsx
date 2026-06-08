// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { CronSchedulePreview } from '@resources/components/CronSchedulePreview';
import {
  type ScheduleUnit,
  cronFromReadableSchedule,
  describeCron,
  localDateTimeValue,
} from '@resources/utils/schedule';

type ReadableScheduleFieldProps = {
  value: string;
  onChange: (value: string) => void;
  label?: string;
  hint?: string;
  timezone?: string | null;
};

function parseReadableCron(value: string) {
  const fields = value.trim().split(/\s+/);
  if (fields.length !== 5) {
    return { advanced: true, interval: 1, unit: 'day' as ScheduleUnit, startsAt: localDateTimeValue() };
  }
  const [minute, hour, day, month, weekday] = fields as [string, string, string, string, string];
  const now = new Date();
  let interval = 1;
  let unit: ScheduleUnit = 'day';
  let startsAt = new Date(now.getFullYear(), now.getMonth(), now.getDate(), Number(hour) || 0, Number(minute) || 0);

  if (/^\d+$/.test(minute) && hour === '*' && day === '*' && month === '*' && weekday === '*') {
    unit = 'hour';
    startsAt = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours(), Number(minute));
  } else if (/^\d+$/.test(minute) && hour.startsWith('*/') && day === '*' && month === '*' && weekday === '*') {
    unit = 'hour';
    interval = Number(hour.slice(2)) || 1;
    startsAt = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours(), Number(minute));
  } else if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day === '*' && month === '*' && weekday === '*') {
    unit = 'day';
  } else if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day.startsWith('*/') && month === '*' && weekday === '*') {
    unit = 'day';
    interval = Number(day.slice(2)) || 1;
  } else if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day === '*' && month === '*' && /^\d+$/.test(weekday)) {
    unit = 'week';
  } else if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && /^\d+$/.test(day) && month === '*' && weekday === '*') {
    unit = 'month';
    startsAt = new Date(now.getFullYear(), now.getMonth(), Number(day), Number(hour), Number(minute));
  } else if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && /^\d+$/.test(day) && /^\d+$/.test(month) && weekday === '*') {
    unit = 'year';
    startsAt = new Date(now.getFullYear(), Number(month) - 1, Number(day), Number(hour), Number(minute));
  } else {
    return { advanced: true, interval: 1, unit: 'day' as ScheduleUnit, startsAt: localDateTimeValue() };
  }

  return { advanced: false, interval, unit, startsAt: localDateTimeValue(startsAt) };
}

export function ReadableScheduleField({ value, onChange, label, hint, timezone }: ReadableScheduleFieldProps) {
  const { t } = useTranslation('common');
  const parsed = useMemo(() => parseReadableCron(value), [value]);
  const [advanced, setAdvanced] = useState(parsed.advanced);
  const [interval, setInterval] = useState(parsed.interval);
  const [unit, setUnit] = useState<ScheduleUnit>(parsed.unit);
  const [startsAt, setStartsAt] = useState(parsed.startsAt);

  useEffect(() => {
    setAdvanced(parsed.advanced);
    setInterval(parsed.interval);
    setUnit(parsed.unit);
    setStartsAt(parsed.startsAt);
  }, [parsed.advanced, parsed.interval, parsed.unit, parsed.startsAt]);

  useEffect(() => {
    if (unit === 'year' && interval !== 1) {
      setInterval(1);
    }
  }, [interval, unit]);

  useEffect(() => {
    if (advanced) return;
    onChange(cronFromReadableSchedule(interval, unit, startsAt));
    // onChange is intentionally omitted because most callers pass inline patch callbacks.
    // Re-running on callback identity changes would cause redundant parent updates.
  }, [advanced, interval, unit, startsAt]);

  const unitOptions = [
    { value: 'hour', label: t('schedule.units.hour') },
    { value: 'day', label: t('schedule.units.day') },
    { value: 'week', label: t('schedule.units.week') },
    { value: 'month', label: t('schedule.units.month') },
    { value: 'year', label: t('schedule.units.year') },
  ];

  return (
    <div className="space-y-2">
      {label && <Label className="mb-1 text-xs text-muted-foreground">{label}</Label>}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant={!advanced ? 'default' : 'outline'}
          onClick={() => setAdvanced(false)}
        >
          {t('schedule.readableMode')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={advanced ? 'default' : 'outline'}
          onClick={() => setAdvanced(true)}
        >
          {t('schedule.advancedMode')}
        </Button>
      </div>
      {advanced ? (
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder="0 3 * * *"
          variant="outline"
        />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(120px,0.45fr)_minmax(150px,0.55fr)_minmax(220px,1fr)]">
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">{t('schedule.every')}</Label>
            <NumberInput value={interval} onChange={setInterval} min={1} max={unit === 'year' ? 1 : 31} />
          </div>
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">{t('schedule.unit')}</Label>
            <Select
              value={unit}
              onChange={(next) => setUnit(next as ScheduleUnit)}
              options={unitOptions}
              variant="outline"
              ariaLabel={t('schedule.unit')}
            />
          </div>
          <div>
            <Label className="mb-1 text-xs text-muted-foreground">{t('schedule.startingAt')}</Label>
            <Input
              type="datetime-local"
              value={startsAt}
              onChange={(event) => setStartsAt(event.target.value)}
              variant="outline"
            />
          </div>
        </div>
      )}
      <p className="text-xs text-muted-foreground">
        {hint ?? t('schedule.generatedCron', { value })}
      </p>
      <p className="text-xs text-muted-foreground">{describeCron(value, t)}</p>
      <CronSchedulePreview expression={value} timezone={timezone} className="rounded-md border border-border/60 bg-muted/30 p-2" />
    </div>
  );
}
