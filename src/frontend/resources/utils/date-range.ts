// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type DateRangePresetId =
  | 'all'
  | 'today'
  | 'yesterday'
  | 'thisWeek'
  | 'lastWeek'
  | 'thisMonth'
  | 'lastMonth'
  | 'thisYear'
  | 'lastYear'
  | 'last15Minutes'
  | 'last1Hour'
  | 'last24Hours'
  | 'last7Days'
  | 'last30Days'
  | 'custom';

export type DateRangePreset = {
  id: DateRangePresetId;
  i18nKey: string;
};

export const DATE_RANGE_PRESETS: readonly DateRangePreset[] = [
  { id: 'all', i18nKey: 'common:dateRange.presetAll' },
  { id: 'last15Minutes', i18nKey: 'common:dateRange.presetLast15Minutes' },
  { id: 'last1Hour', i18nKey: 'common:dateRange.presetLast1Hour' },
  { id: 'last24Hours', i18nKey: 'common:dateRange.presetLast24Hours' },
  { id: 'today', i18nKey: 'common:dateRange.presetToday' },
  { id: 'yesterday', i18nKey: 'common:dateRange.presetYesterday' },
  { id: 'thisWeek', i18nKey: 'common:dateRange.presetThisWeek' },
  { id: 'lastWeek', i18nKey: 'common:dateRange.presetLastWeek' },
  { id: 'last7Days', i18nKey: 'common:dateRange.presetLast7Days' },
  { id: 'thisMonth', i18nKey: 'common:dateRange.presetThisMonth' },
  { id: 'lastMonth', i18nKey: 'common:dateRange.presetLastMonth' },
  { id: 'last30Days', i18nKey: 'common:dateRange.presetLast30Days' },
  { id: 'thisYear', i18nKey: 'common:dateRange.presetThisYear' },
  { id: 'lastYear', i18nKey: 'common:dateRange.presetLastYear' },
  { id: 'custom', i18nKey: 'common:dateRange.presetCustom' },
] as const;

export type DateRange = {
  from: Date | null;
  to: Date | null;
};

export const EMPTY_DATE_RANGE: DateRange = { from: null, to: null };

function startOfDay(d: Date): Date {
  const out = new Date(d);
  out.setHours(0, 0, 0, 0);
  return out;
}

function endOfDay(d: Date): Date {
  const out = new Date(d);
  out.setHours(23, 59, 59, 999);
  return out;
}

function startOfWeek(d: Date): Date {
  const out = startOfDay(d);
  const day = out.getDay();
  const diff = (day + 6) % 7;
  out.setDate(out.getDate() - diff);
  return out;
}

function startOfMonth(d: Date): Date {
  const out = startOfDay(d);
  out.setDate(1);
  return out;
}

function startOfYear(d: Date): Date {
  const out = startOfDay(d);
  out.setMonth(0, 1);
  return out;
}

function addDays(d: Date, days: number): Date {
  const out = new Date(d);
  out.setDate(out.getDate() + days);
  return out;
}

function addMonths(d: Date, months: number): Date {
  const out = new Date(d);
  out.setMonth(out.getMonth() + months);
  return out;
}

function addYears(d: Date, years: number): Date {
  const out = new Date(d);
  out.setFullYear(out.getFullYear() + years);
  return out;
}

export function resolveDateRange(preset: DateRangePresetId, custom: DateRange = EMPTY_DATE_RANGE, now: Date = new Date()): DateRange {
  switch (preset) {
    case 'all':
      return EMPTY_DATE_RANGE;
    case 'last15Minutes':
      return { from: new Date(now.getTime() - 15 * 60 * 1000), to: now };
    case 'last1Hour':
      return { from: new Date(now.getTime() - 60 * 60 * 1000), to: now };
    case 'last24Hours':
      return { from: new Date(now.getTime() - 24 * 60 * 60 * 1000), to: now };
    case 'today':
      return { from: startOfDay(now), to: endOfDay(now) };
    case 'yesterday': {
      const y = addDays(now, -1);
      return { from: startOfDay(y), to: endOfDay(y) };
    }
    case 'thisWeek':
      return { from: startOfWeek(now), to: endOfDay(now) };
    case 'lastWeek': {
      const thisWeekStart = startOfWeek(now);
      const lastWeekStart = addDays(thisWeekStart, -7);
      const lastWeekEnd = addDays(thisWeekStart, -1);
      return { from: startOfDay(lastWeekStart), to: endOfDay(lastWeekEnd) };
    }
    case 'last7Days':
      return { from: startOfDay(addDays(now, -6)), to: endOfDay(now) };
    case 'thisMonth':
      return { from: startOfMonth(now), to: endOfDay(now) };
    case 'lastMonth': {
      const thisMonthStart = startOfMonth(now);
      const lastMonthStart = addMonths(thisMonthStart, -1);
      const lastMonthEnd = addDays(thisMonthStart, -1);
      return { from: startOfDay(lastMonthStart), to: endOfDay(lastMonthEnd) };
    }
    case 'last30Days':
      return { from: startOfDay(addDays(now, -29)), to: endOfDay(now) };
    case 'thisYear':
      return { from: startOfYear(now), to: endOfDay(now) };
    case 'lastYear': {
      const thisYearStart = startOfYear(now);
      const lastYearStart = addYears(thisYearStart, -1);
      const lastYearEnd = addDays(thisYearStart, -1);
      return { from: startOfDay(lastYearStart), to: endOfDay(lastYearEnd) };
    }
    case 'custom':
      return {
        from: custom.from ? startOfDay(custom.from) : null,
        to: custom.to ? endOfDay(custom.to) : null,
      };
    default:
      return EMPTY_DATE_RANGE;
  }
}

export function formatDateInputValue(d: Date | null): string {
  if (!d) return '';
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd}`;
}

export function parseDateInputValue(value: string): Date | null {
  if (!value) return null;
  const parts = value.split('-');
  if (parts.length !== 3) return null;
  const yyyy = Number(parts[0]);
  const mm = Number(parts[1]);
  const dd = Number(parts[2]);
  if (!Number.isFinite(yyyy) || !Number.isFinite(mm) || !Number.isFinite(dd)) return null;
  return new Date(yyyy, mm - 1, dd);
}

export function dateRangeToQuery(range: DateRange): { from?: string; to?: string } {
  const out: { from?: string; to?: string } = {};
  if (range.from) out.from = range.from.toISOString();
  if (range.to) out.to = range.to.toISOString();
  return out;
}

export function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
