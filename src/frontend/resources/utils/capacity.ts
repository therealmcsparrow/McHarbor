// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const CAPACITY_AMBER_PERCENT = 75;
export const CAPACITY_RED_PERCENT = 90;

export function capacityTone(percent: number): 'destructive' | 'warning' | 'primary' {
  if (percent >= CAPACITY_RED_PERCENT) return 'destructive';
  if (percent >= CAPACITY_AMBER_PERCENT) return 'warning';
  return 'primary';
}

export function capacityFillClass(percent: number): string {
  switch (capacityTone(percent)) {
    case 'destructive':
      return 'bg-destructive';
    case 'warning':
      return 'bg-warning';
    default:
      return 'bg-primary';
  }
}
