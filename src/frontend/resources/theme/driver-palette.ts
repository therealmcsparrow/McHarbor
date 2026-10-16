// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChartStep } from './status-palette';

// ── Driver colours ────────────────────────────────────────────────
//
// Network drivers, container runtimes, and storage drivers each need a
// single accent colour for badges. Telling `bridge` from `overlay` at a
// glance is a real capability, so all 13 are kept — they are simply
// re-pointed at the themeable chart ramp.
//
// The three categories are independent: a network badge and a runtime
// badge never appear in the same row. The largest category has 6
// members, so an 8-step ramp covers all three with no *within-category*
// collision, which is the only collision that matters:
//
//   network   bridge · host · overlay · macvlan · ipvlan · none   → 1…6
//   runtime   runc · containerd · cri-o · kata-runtime            → 1…4
//   storage   overlay2 · btrfs · zfs                              → 1…3
//
// `driverColor()` takes an optional category so the same step can
// safely serve `bridge` and `runc`.

export type DriverCategory = 'network' | 'runtime' | 'storage';

export type DriverToken = {
  bg: string;
  text: string;
  border: string;
  dot: string;
  /** Raw CSS custom-property reference, for non-class consumers. */
  cssVar: string;
  /** Which ramp step this driver occupies within its category. */
  step: ChartStep;
};

function driverToken(step: ChartStep): DriverToken {
  return {
    bg: `bg-chart-${step}/15`,
    text: `text-chart-${step}`,
    border: `border-chart-${step}/30`,
    dot: `bg-chart-${step}`,
    cssVar: `var(--chart-${step})`,
    step,
  };
}

export const DRIVER_CATEGORIES: Record<DriverCategory, Record<string, DriverToken>> = {
  network: {
    bridge: driverToken(1),
    host: driverToken(2),
    overlay: driverToken(3),
    macvlan: driverToken(4),
    ipvlan: driverToken(5),
    none: driverToken(6),
  },
  runtime: {
    runc: driverToken(1),
    containerd: driverToken(2),
    'cri-o': driverToken(3),
    'kata-runtime': driverToken(4),
  },
  storage: {
    overlay2: driverToken(1),
    btrfs: driverToken(2),
    zfs: driverToken(3),
  },
};

/**
 * Flat view over all three categories. Driver names are globally unique
 * today, so a lookup without a category still resolves — the category
 * argument exists so that a future collision (a network driver and a
 * storage driver sharing a name) resolves correctly rather than by
 * whichever category happened to be declared first.
 */
export const DRIVER_COLORS: Record<string, DriverToken> = {
  ...DRIVER_CATEGORIES.network,
  ...DRIVER_CATEGORIES.runtime,
  ...DRIVER_CATEGORIES.storage,
};

export const FALLBACK_DRIVER: DriverToken = {
  bg: 'bg-muted',
  text: 'text-muted-foreground',
  border: 'border-border',
  dot: 'bg-muted-foreground',
  cssVar: 'var(--muted-foreground)',
  step: 8,
};

export function driverColor(driver: string, category?: DriverCategory): DriverToken {
  if (!driver) return FALLBACK_DRIVER;
  const key = driver.toLowerCase();
  if (category) return DRIVER_CATEGORIES[category][key] ?? FALLBACK_DRIVER;
  return DRIVER_COLORS[key] ?? FALLBACK_DRIVER;
}

export function driverClasses(driver: string, extra?: string, category?: DriverCategory): string {
  const tone = driverColor(driver, category);
  return [tone.bg, tone.text, tone.border, extra].filter(Boolean).join(' ');
}
