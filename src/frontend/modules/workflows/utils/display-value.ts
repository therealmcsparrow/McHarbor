// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** Renders an arbitrary config or payload value as text; objects become JSON rather than "[object Object]". */
export function displayValue(value: unknown): string {
  if (value === undefined || value === null) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value as number | boolean | bigint | symbol);
}
