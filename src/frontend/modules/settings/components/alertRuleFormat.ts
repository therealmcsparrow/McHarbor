// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AlertRule } from '@resources/hooks/useAlerts';

/** Renders a second count as a compact duration ("5m", "1h", "1d"). */
export function formatSeconds(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400}d`;
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

/**
 * A percent sign binds to its number; a word does not. Without this,
 * "5 attempts" renders as "5attempts" while "80%" would gain a stray
 * space. Mirrors formatComparison on the backend so the card and the
 * delivered message read identically.
 */
export function formatUnit(unit: string | undefined): string {
  if (!unit) return '';
  if (unit === '%') return unit;
  return ` ${unit}`;
}

/** Renders the structured condition the way the engine reads it. */
export function formatCondition(rule: AlertRule): string {
  const parts: string[] = [];
  const { operator, threshold, unit, forSeconds, windowSeconds } = rule.condition ?? {};

  if (operator && threshold !== undefined) {
    parts.push(`${operator} ${threshold}${formatUnit(unit)}`);
  }
  if (windowSeconds) {
    parts.push(`in ${formatSeconds(windowSeconds)}`);
  }
  if (forSeconds) {
    parts.push(`for ${formatSeconds(forSeconds)}`);
  }

  return parts.join(' ');
}
