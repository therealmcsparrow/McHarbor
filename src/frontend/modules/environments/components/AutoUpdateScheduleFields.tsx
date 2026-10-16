// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { AutoUpdateDay } from '../hooks/useEnvironmentDetailState';

const ALL_DAYS = [
  { value: 'mon', short: 'Mon' },
  { value: 'tue', short: 'Tue' },
  { value: 'wed', short: 'Wed' },
  { value: 'thu', short: 'Thu' },
  { value: 'fri', short: 'Fri' },
  { value: 'sat', short: 'Sat' },
  { value: 'sun', short: 'Sun' },
] as const;

type DayChipProps = {
  day: (typeof ALL_DAYS)[number];
  selected: boolean;
  disabled: boolean;
  onToggle: (value: string) => void;
  labels: Record<string, string>;
};

function DayChip({ day, selected, disabled, onToggle, labels }: DayChipProps) {
  return (
    <Button
      type="button"
      variant={selected ? 'default' : 'outline'}
      size="sm"
      onClick={() => onToggle(day.value)}
      disabled={disabled}
      aria-pressed={selected}
      className={cn(
        'inline-flex min-w-[3rem] items-center justify-center rounded-lg border px-3 text-xs font-semibold tracking-[0.12em] uppercase transition-colors',
        selected
          ? 'border-chart-1/40 bg-chart-1/15 text-chart-1'
          : 'border-border bg-card/40 text-muted-foreground hover:border-border/80 hover:text-foreground',
        disabled && 'cursor-not-allowed opacity-60',
      )}
    >
      {labels[day.value] ?? day.short}
    </Button>
  );
}

type AutoUpdateScheduleFieldsProps = {
  enabled: boolean;
  isSaving: boolean;
  windowStart: string;
  windowEnd: string;
  daysSelected: Set<AutoUpdateDay>;
  dayLabels: Record<string, string>;
  onWindowStartChange: (value: string) => void;
  onWindowEndChange: (value: string) => void;
  onDayToggle: (day: string) => void;
};

export function AutoUpdateScheduleFields({
  enabled,
  isSaving,
  windowStart,
  windowEnd,
  daysSelected,
  dayLabels,
  onWindowStartChange,
  onWindowEndChange,
  onDayToggle,
}: AutoUpdateScheduleFieldsProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
            {t('detail.retention.autoUpdateWindowStartLabel')}
          </label>
          <Input
            type="time"
            value={windowStart}
            disabled={!enabled || isSaving}
            onChange={(e) => onWindowStartChange(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
            {t('detail.retention.autoUpdateWindowEndLabel')}
          </label>
          <Input
            type="time"
            value={windowEnd}
            disabled={!enabled || isSaving}
            onChange={(e) => onWindowEndChange(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <label className="block text-xs font-semibold tracking-[0.14em] text-muted-foreground uppercase">
          {t('detail.retention.autoUpdateDaysLabel')}
        </label>
        <div className="flex flex-wrap gap-2">
          {ALL_DAYS.map((day) => (
            <DayChip
              key={day.value}
              day={day}
              selected={daysSelected.has(day.value)}
              disabled={!enabled || isSaving}
              onToggle={onDayToggle}
              labels={dayLabels}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
