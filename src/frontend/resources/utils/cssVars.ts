// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CSSProperties } from 'react';

/**
 * Build a `style` object that only sets CSS custom properties.
 *
 * Runtime-computed values (percent widths, canvas positions, colour swatches)
 * cannot be expressed as Tailwind classes, so they travel as variables and the
 * class reads them: `style={cssVars({ w: '42%' })}` + `className="w-(--w)"`.
 * Keys are given without the leading `--`.
 */
export function cssVars(vars: Record<string, string | number | undefined>): CSSProperties {
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(vars)) {
    if (value === undefined) continue;
    out[`--${key}`] = value;
  }
  return out as CSSProperties;
}
