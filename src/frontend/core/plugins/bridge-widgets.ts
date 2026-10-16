// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** `6x3` → { w: 6, h: 3 }; anything unparseable → the fallback. */
export function parsePluginWidgetSize(
  spec: string | undefined,
  fallback: { w: number; h: number },
): { w: number; h: number } {
  const match = /^(\d+)x(\d+)$/.exec(spec ?? '');
  if (!match) return fallback;
  const w = Number(match[1]);
  const h = Number(match[2]);
  if (!Number.isFinite(w) || !Number.isFinite(h) || w < 1 || h < 1) return fallback;
  return { w: Math.min(12, w), h };
}
