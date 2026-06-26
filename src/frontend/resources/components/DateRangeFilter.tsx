// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import * as Popover from '@radix-ui/react-popover';
import { IconCalendar, IconChevronDown, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { cn } from '@resources/utils/cn';
import {
  DATE_RANGE_PRESETS,
  EMPTY_DATE_RANGE,
  formatDateInputValue,
  parseDateInputValue,
  resolveDateRange,
  type DateRange,
  type DateRangePresetId,
} from '@resources/utils/date-range';

export type DateRangeFilterValue = {
  preset: DateRangePresetId;
  custom: DateRange;
};

export const DEFAULT_DATE_RANGE_VALUE: DateRangeFilterValue = {
  preset: 'all',
  custom: EMPTY_DATE_RANGE,
};

type DateRangeFilterProps = {
  value: DateRangeFilterValue;
  onChange: (value: DateRangeFilterValue) => void;
  className?: string;
};

export function DateRangeFilter({ value, onChange, className }: DateRangeFilterProps) {
  const { t, i18n } = useTranslation('common');
  const [open, setOpen] = useState(false);

  const resolved = useMemo(
    () => resolveDateRange(value.preset, value.custom),
    [value.preset, value.custom],
  );

  const isActive = resolved.from !== null || resolved.to !== null;

  const label = useMemo(() => {
    if (!isActive) {
      return t('dateRange.allTime');
    }
    if (value.preset === 'custom' || value.preset === 'all') {
      const fmt = new Intl.DateTimeFormat(i18n.language, { month: 'short', day: 'numeric', year: 'numeric' });
      const fromLabel = resolved.from ? fmt.format(resolved.from) : '…';
      const toLabel = resolved.to ? fmt.format(resolved.to) : '…';
      return `${fromLabel} → ${toLabel}`;
    }
    const preset = DATE_RANGE_PRESETS.find((p) => p.id === value.preset);
    const key = preset ? preset.i18nKey.replace(/^[^:]+:/, '') : 'dateRange.allTime';
    return t(key);
  }, [isActive, resolved, value.preset, i18n.language, t]);

  const handlePresetChange = (id: string) => {
    const preset = id as DateRangePresetId;
    onChange({ preset, custom: value.custom });
    if (preset !== 'custom') {
      setOpen(false);
    }
  };

  const handleCustomFrom = (raw: string) => {
    const from = parseDateInputValue(raw);
    onChange({ preset: 'custom', custom: { from, to: value.custom.to } });
  };

  const handleCustomTo = (raw: string) => {
    const to = parseDateInputValue(raw);
    onChange({ preset: 'custom', custom: { from: value.custom.from, to } });
  };

  const handleReset = () => {
    onChange(DEFAULT_DATE_RANGE_VALUE);
  };

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open]);

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <Button
          variant={isActive ? 'default' : 'outline'}
          size="sm"
          className={cn('min-w-44 justify-between', className)}
          aria-label={t('dateRange.label')}
        >
          <span className="flex items-center gap-2 truncate">
            <IconCalendar className="size-4 shrink-0" />
            <span className="truncate">{label}</span>
          </span>
          <IconChevronDown className={cn('ml-2 size-4 shrink-0 transition-transform', open && 'rotate-180')} />
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          sideOffset={6}
          align="start"
          className="z-50 w-72 rounded-lg border border-border bg-card shadow-lg animate-in fade-in-0 zoom-in-95"
        >
          <div className="p-2">
            <div className="mb-1 px-2 pt-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t('dateRange.title')}
            </div>
            <div className="max-h-72 overflow-y-auto">
              {DATE_RANGE_PRESETS.map((preset) => {
                const key = preset.i18nKey.replace(/^[^:]+:/, '');
                return (
                  <button
                    key={preset.id}
                    type="button"
                    onClick={() => handlePresetChange(preset.id)}
                    className={cn(
                      'flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/50',
                      value.preset === preset.id
                        ? 'bg-muted/70 font-medium text-foreground'
                        : 'text-foreground',
                    )}
                  >
                    <span>{t(key)}</span>
                    {value.preset === preset.id && <span className="text-xs text-primary">✓</span>}
                  </button>
                );
              })}
            </div>

            {value.preset === 'custom' && (
              <div className="mt-2 space-y-2 border-t border-border px-2 pb-1 pt-3">
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t('dateRange.from')}
                  </label>
                  <Input
                    type="date"
                    value={formatDateInputValue(value.custom.from)}
                    onChange={(e) => handleCustomFrom(e.target.value)}
                    max={formatDateInputValue(value.custom.to) || undefined}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t('dateRange.to')}
                  </label>
                  <Input
                    type="date"
                    value={formatDateInputValue(value.custom.to)}
                    onChange={(e) => handleCustomTo(e.target.value)}
                    min={formatDateInputValue(value.custom.from) || undefined}
                  />
                </div>
              </div>
            )}

            <div className="mt-2 flex items-center justify-between border-t border-border px-2 pt-2">
              <Button variant="ghost" size="sm" onClick={handleReset} disabled={!isActive}>
                <IconX className="size-3.5" />
                {t('dateRange.reset')}
              </Button>
              <Button variant="outline" size="sm" onClick={() => setOpen(false)}>
                {t('dateRange.close')}
              </Button>
            </div>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
