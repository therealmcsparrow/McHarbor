// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n from '@core/i18n/i18n';

function getLocale(): string {
  return i18n.language || 'en';
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

export function formatDate(dateStr: string | undefined | null): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(getLocale(), {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export function formatDateOnly(dateStr: string | undefined | null): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(getLocale(), {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
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
