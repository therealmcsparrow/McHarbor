// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function toNumberOrNull(text: string | number | null | undefined): number | null {
  if (text === null || text === undefined) return null;
  if (typeof text === 'number') return Number.isFinite(text) ? text : null;
  if (text.trim() === '') return null;
  const parsed = Number(text);
  return Number.isFinite(parsed) ? parsed : null;
}

export function toNumberText(value: number | null): string {
  return value === null ? '' : String(value);
}
