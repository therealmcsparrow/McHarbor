// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from '@core/i18n/i18n';

function getLocale(): string {
  return i18n.language || 'en';
}

// LocaleFormat is the per-user time/date format preference the
// formatters below read when the optional `prefs` argument is
// supplied. Use the `useLocaleFormat` hook in components so the
// `format*` helpers stay pure and testable.
export type LocaleFormat = {
  timeFormat: '12h' | '24h';
  dateFormat: 'ddmmyyyy' | 'mmddyyyy';
};

const DEFAULT_FORMAT: LocaleFormat = {
  timeFormat: '24h',
  dateFormat: 'ddmmyyyy',
};

function normalizeFormat(input: Partial<LocaleFormat> | null | undefined): LocaleFormat {
  if (!input) return DEFAULT_FORMAT;
  return {
    timeFormat: input.timeFormat === '12h' ? '12h' : '24h',
    dateFormat: input.dateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy',
  };
}

export function formatBytes(bytes: number, decimals = 1): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`;
}

export function splitBytes(bytes: number, decimals = 1): { value: string; unit: string } {
  if (!bytes || bytes === 0) return { value: '0', unit: 'B' };
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return { value: String(parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))), unit: sizes[i] ?? 'B' };
}

export function formatUptime(seconds: number): string {
  if (!seconds) return '0m';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

// formatDate renders an ISO 8601 timestamp as the user's chosen
// date layout (DD/MM/YYYY or MM/DD/YYYY) plus the locale's
// preferred time format. Pass `prefs` to honor the per-user time
// format; omit it to fall back to defaults (24h + ddmmyyyy).
export function formatDate(
  dateStr: string | undefined | null,
  prefs?: Partial<LocaleFormat>,
): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '-';
  const { timeFormat, dateFormat } = normalizeFormat(prefs);
  const locale = getLocale();
  const timePart = formatTimeOfDate(date, locale, timeFormat);
  return `${dateLayout(dateFormat, date, locale)} ${timePart}`;
}

// formatDateOnly renders just the calendar date using the user's
// chosen DD/MM vs MM/DD layout.
export function formatDateOnly(
  dateStr: string | undefined | null,
  prefs?: Partial<LocaleFormat>,
): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '-';
  const { dateFormat } = normalizeFormat(prefs);
  const locale = getLocale();
  return dateLayout(dateFormat, date, locale);
}

// formatTime renders the time-of-day using the user's chosen 12h
// or 24h notation.
export function formatTime(
  dateStr: string | undefined | null,
  prefs?: Partial<LocaleFormat>,
): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '-';
  const { timeFormat } = normalizeFormat(prefs);
  return formatTimeOfDate(date, getLocale(), timeFormat);
}

function formatTimeOfDate(date: Date, locale: string, timeFormat: '12h' | '24h'): string {
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: timeFormat === '12h',
  }).format(date);
}

// dateLayout picks a date layout that matches the user's
// `dateFormat` preference. We support two layouts: DD/MM/YYYY
// (default) and MM/DD/YYYY. The Intl API only knows about the
// locale's calendar conventions (e.g. en-US is "M/d/yyyy" and
// en-GB is "d/MM/yyyy"), so for the two arbitrary choices we
// format the parts ourselves and join with the requested
// separators. Locale date order is preserved as the "fallback"
// when a caller has not yet picked a preference.
function dateLayout(
  dateFormat: 'ddmmyyyy' | 'mmddyyyy',
  date: Date,
  locale: string,
): string {
  const localeFormatter = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
  const parts = localeFormatter.formatToParts(date);
  const day = parts.find((p) => p.type === 'day')?.value ?? '';
  const month = parts.find((p) => p.type === 'month')?.value ?? '';
  const year = parts.find((p) => p.type === 'year')?.value ?? '';
  if (dateFormat === 'mmddyyyy') {
    return `${month}/${day}/${year}`;
  }
  return `${day}/${month}/${year}`;
}

export function timeAgo(dateStr: string | undefined | null): string {
  if (!dateStr) return '-';
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  if (isNaN(then)) return '-';
  const diff = Math.floor((now - then) / 1000);

  const rtf = new Intl.RelativeTimeFormat(getLocale(), { numeric: 'auto' });

  if (diff < 0) return rtf.format(0, 'second');
  if (diff < 60) return rtf.format(0, 'second');
  if (diff < 3600) return rtf.format(-Math.floor(diff / 60), 'minute');
  if (diff < 86400) return rtf.format(-Math.floor(diff / 3600), 'hour');
  return rtf.format(-Math.floor(diff / 86400), 'day');
}

export function truncateId(id: string | undefined | null, len = 12): string {
  if (!id) return '';
  return id.slice(0, len);
}
