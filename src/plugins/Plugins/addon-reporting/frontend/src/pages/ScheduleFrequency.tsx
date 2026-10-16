// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { cn } from '@resources/utils/cn';
import { Field } from './ScheduleField';
import { draftCron, draftCronValid } from './schedule-draft';
import type { Frequency, ScheduleDraft } from './schedule-draft';

const CRON_EXAMPLE = '0 7 * * 1';

export function FrequencyFields({
  draft,
  onChange,
}: {
  draft: ScheduleDraft;
  onChange: (next: ScheduleDraft) => void;
}) {
  const { t } = useTranslation('reporting');
  const hours = useMemo(
    () =>
      Array.from({ length: 24 }, (_, h) => ({
        value: String(h),
        label: `${String(h).padStart(2, '0')}:00 UTC`,
      })),
    [],
  );
  // 1–28 only. The 29th, 30th and 31st silently skip months that are
  // shorter, which turns a "monthly" report into a "most months" report.
  const days = useMemo(
    () => Array.from({ length: 28 }, (_, i) => ({ value: String(i + 1), label: String(i + 1) })),
    [],
  );
  const weekdays = useMemo(
    () =>
      Array.from({ length: 7 }, (_, d) => ({
        value: String(d),
        label: t(`schedule.weekday.${d}`),
      })),
    [t],
  );

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label={t('schedule.frequency')}>
        <Select
          value={draft.frequency}
          onChange={(value) => onChange({ ...draft, frequency: value as Frequency })}
          ariaLabel={t('schedule.frequency')}
          options={[
            { value: 'daily', label: t('schedule.frequencyDaily') },
            { value: 'weekly', label: t('schedule.frequencyWeekly') },
            { value: 'monthly', label: t('schedule.frequencyMonthly') },
            { value: 'custom', label: t('schedule.frequencyCustom') },
          ]}
        />
      </Field>

      {draft.frequency === 'custom' ? (
        <Field label={t('schedule.cron')}>
          <Input
            value={draft.customCron}
            onChange={(e) => onChange({ ...draft, customCron: e.target.value })}
            placeholder={CRON_EXAMPLE}
            aria-label={t('schedule.cron')}
            className="font-mono"
          />
        </Field>
      ) : (
        <Field label={t('schedule.time')}>
          <Select
            value={String(draft.hour)}
            onChange={(value) => onChange({ ...draft, hour: Number(value) })}
            ariaLabel={t('schedule.time')}
            options={hours}
          />
        </Field>
      )}

      {draft.frequency === 'weekly' ? (
        <Field label={t('schedule.weekdayLabel')}>
          <Select
            value={String(draft.weekday)}
            onChange={(value) => onChange({ ...draft, weekday: Number(value) })}
            ariaLabel={t('schedule.weekdayLabel')}
            options={weekdays}
          />
        </Field>
      ) : null}

      {draft.frequency === 'monthly' ? (
        <Field label={t('schedule.dayOfMonth')}>
          <Select
            value={String(draft.dayOfMonth)}
            onChange={(value) => onChange({ ...draft, dayOfMonth: Number(value) })}
            ariaLabel={t('schedule.dayOfMonth')}
            options={days}
          />
        </Field>
      ) : null}

      <div className="sm:col-span-2">
        <p className={cn('text-xs', draftCronValid(draft) ? 'text-muted-foreground' : 'text-danger')}>
          {draftCronValid(draft) ? t('schedule.cronPreview', { cron: draftCron(draft) }) : t('schedule.cronInvalid')}
        </p>
      </div>
    </div>
  );
}
