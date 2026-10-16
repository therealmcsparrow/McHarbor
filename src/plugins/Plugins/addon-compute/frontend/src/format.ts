// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { formatDate, formatDateOnly } from '@resources/utils/format';

/**
 * Formatting shared across the compute pages.
 *
 * Bytes are rendered in binary units because that is what every tool an
 * operator will cross-check against reports — `zfs list`, `df`, `qemu-img
 * info`. Showing 1.07 GB where `zfs list` says 1.00G would make the two look
 * like different numbers for the same thing.
 */

const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'] as const;

/** Renders a byte count in binary units. */
export function formatBytes(bytes: number | undefined | null, fractionDigits = 1): string {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) return '—';
  if (bytes === 0) return '0 B';

  const negative = bytes < 0;
  let value = Math.abs(bytes);
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  // Whole bytes never want a decimal point.
  const digits = unit === 0 ? 0 : fractionDigits;
  return `${negative ? '-' : ''}${value.toFixed(digits)} ${UNITS[unit]}`;
}

/** Percentage of a pool that is used, clamped and safe against a zero capacity. */
export function usedPercent(used: number, capacity: number): number {
  if (!capacity || capacity <= 0) return 0;
  return Math.min(100, Math.max(0, Math.round((used / capacity) * 100)));
}

/**
 * Whether a pool is provisioned beyond its real capacity.
 *
 * This is normal on a thin backend and not in itself a problem — but a thin
 * pool that actually fills pauses every guest on it, so the UI says so rather
 * than leaving the operator to discover it.
 */
export function isOvercommitted(allocated: number, capacity: number): boolean {
  return capacity > 0 && allocated > capacity;
}

/** A relative time for "checked 5 minutes ago". */
export function formatRelative(iso: string | undefined, locale?: string): string {
  if (!iso) return '—';
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '—';

  const seconds = Math.round((then - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });

  const thresholds: [number, Intl.RelativeTimeFormatUnit][] = [
    [60, 'second'],
    [3600, 'minute'],
    [86400, 'hour'],
    [604800, 'day'],
  ];
  let magnitude = Math.abs(seconds);
  for (const [limit, unit] of thresholds) {
    if (magnitude < limit) {
      const divisor = limit === 60 ? 1 : limit === 3600 ? 60 : limit === 86400 ? 3600 : 86400;
      return formatter.format(Math.round(seconds / divisor), unit);
    }
    magnitude = Math.abs(seconds);
  }
  return formatDateOnly(iso);
}

/** An absolute timestamp for tooltips and detail rows. */
export function formatAbsolute(iso: string | undefined): string {
  if (!iso) return '—';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return formatDate(iso);
}
