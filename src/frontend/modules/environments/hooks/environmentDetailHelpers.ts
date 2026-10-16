// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const ALL_DAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const;
type WeekDay = (typeof ALL_DAYS)[number];
export type AutoUpdateDay = WeekDay;

export function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) {
    return 80;
  }

  return Math.min(100, Math.max(1, Math.round(value)));
}

export function clampDayCount(value: number): number {
  if (!Number.isFinite(value)) {
    return 7;
  }

  return Math.min(3650, Math.max(1, Math.round(value)));
}

export function clampHours(value: number): number {
  if (!Number.isFinite(value)) {
    return 24;
  }

  return Math.min(8760, Math.max(0, Math.round(value)));
}

export function parseSelectedDays(value: string): Set<AutoUpdateDay> {
  const next = new Set<AutoUpdateDay>();
  for (const raw of value.split(',')) {
    const day = raw.trim().toLowerCase() as AutoUpdateDay;
    if ((ALL_DAYS as readonly string[]).includes(day)) {
      next.add(day);
    }
  }
  return next;
}

export function serializeSelectedDays(selected: Set<AutoUpdateDay>): string {
  return ALL_DAYS.filter((day) => selected.has(day as AutoUpdateDay)).join(',');
}
