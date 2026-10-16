// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { GenerateRequest, ReportType } from '../types';

export const PERIODS = [7, 30, 90, 180];
export const CUSTOM = 'custom';

/**
 * The period half of the request.
 *
 * `resolvePeriod` on the backend prefers an explicit `from`/`to` over
 * `days`, and requires `to` to be strictly after `from` — so `to` is sent
 * as the end of the chosen day rather than its midnight, which is both
 * what a reader means by "to the 28th" and what makes a single-day range
 * work instead of silently falling back to the default window.
 */
export function periodFields(
  type: ReportType,
  period: string,
  from: string,
  to: string,
): Pick<GenerateRequest, 'days' | 'from' | 'to'> {
  if (type.point_in_time) return {};
  if (period === CUSTOM) return { from, to: `${to}T23:59:59` };
  return { days: Number(period) };
}

/**
 * The window length a schedule should re-use, in days.
 *
 * A schedule cannot store the absolute range the dialog shows — every run
 * must cover the period ending at its own run time — so a custom range is
 * converted to the number of days it spans and a preset passes its own
 * value straight through.
 */
export function periodDays(period: string, from: string, to: string): number {
  if (period !== CUSTOM) return Number(period);
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  if (Number.isNaN(start) || Number.isNaN(end) || end <= start) return 30;
  return Math.max(1, Math.round((end - start) / 86_400_000));
}

/** `YYYY-MM-DD`, n days before today, in the viewer's own timezone. */
export function isoDaysAgo(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() - days);
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${month}-${day}`;
}
