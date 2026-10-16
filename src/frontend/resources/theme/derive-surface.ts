// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// deriveSurface — rebuild the surface language from the tokens a user can
// actually edit.
//
// A theme ships two halves. The hand-authored half is the 27 tokens the
// Themes tab exposes — `--background`, `--card`, `--primary`, `--border`
// and friends. Everything else in `public/themes/*.css` is *derived* from
// those by `scripts/theme-tokens.mjs`: the status tones, the chart ramp,
// and the surface language this file is concerned with — the aurora
// washes behind the app, the glass the panels are cut from, the hairlines,
// the shadows, and the two ends of the brand mark's gradient.
//
// That derivation runs at build time, which is correct for a theme and
// wrong for a customisation. Override `--primary` in Settings and the
// buttons move while the aurora behind them — generated from the *theme's*
// primary — does not. Override `--card` and the modals keep the old
// surface. The colours drift apart, and the app stops looking like one
// thing.
//
// So the same maths runs again here, at runtime, from the *effective*
// values: the user's override where there is one, the theme's own value
// where there is not. The constants below are copied from the generator
// deliberately — the two must agree exactly, or a stock theme would visibly
// jump the moment its first override is set.
//
// Keep this file and the `buildBlock` surface-language section of
// `scripts/theme-tokens.mjs` in step.

import { clampChroma, formatClamped, formatOklch, normalizeHue, type Oklch } from './oklch';

/** The editable tokens the surface language is built from. */
export type SurfaceSources = {
  background: Oklch;
  card: Oklch;
  primary: Oklch;
  /** Supplies the *ink hue* — what a shadow or a hairline is made of. */
  mutedForeground: Oklch | null;
};

/** Token name -> value. Written inline on `<html>` by the caller. */
export type DerivedTokens = Record<string, string>;

/**
 * Every token this module produces. The caller clears the ones that come
 * back unset, so a token dropped from the output falls back to the theme
 * stylesheet rather than getting stuck at its last derived value.
 */
export const DERIVED_TOKENS = [
  'canvas-2',
  'glass-solid',
  'hairline',
  'hairline-2',
  'grid-line',
  'scrim',
  'shadow-1',
  'shadow-2',
  'shadow-3',
  'structure',
  'accent-wash',
  'accent-glow',
  'aurora-a',
  'aurora-b',
  'aurora-c',
] as const;

/**
 * The three washes: the primary's own hue, a cool partner 62° off, and a
 * near neighbour 28° back. `[hue offset, chroma, lightness, alpha]`.
 *
 * Wide enough to read as more than one colour, close enough to stay one
 * family. The lightness is an anchor rather than a formula on
 * `--background`: a wash tied to the canvas tracks it into invisibility,
 * and an invisible aurora is the one thing it cannot be - the glass panels
 * have nothing to blur without it. `--background` instead sets a *floor*
 * on how close the wash may come (see `MIN_CANVAS_SEPARATION`).
 */
const AURORA: Record<'light' | 'dark', ReadonlyArray<[number, number, number, number]>> = {
  light: [
    [0, 0.1, 0.86, 0.3],
    [62, 0.09, 0.8, 0.26],
    [-28, 0.08, 0.88, 0.22],
  ],
  dark: [
    [0, 0.15, 0.7, 0.17],
    [62, 0.17, 0.52, 0.26],
    [-28, 0.13, 0.62, 0.11],
  ],
};

/**
 * How far a wash must stay from `--background` to still read as light on
 * the canvas rather than as part of it.
 *
 * Measured, not guessed: across all 18 shipped themes the tightest
 * separation is 0.060 in light (Monokai's third wash on its pale canvas)
 * and 0.300 in dark (Catppuccin Latte's second). The floors sit just under
 * both, so no shipped theme moves - a floor above them would have quietly
 * restyled four of the dark-first themes' light variants.
 *
 * It exists for the case the fixed anchors were never chosen for: a
 * `--background` dragged into the middle of the range, where a wash at a
 * fixed lightness disappears into the canvas it is supposed to light.
 */
const MIN_CANVAS_SEPARATION = { light: 0.055, dark: 0.295 } as const;

function separateFromCanvas(anchorL: number, canvasL: number, dark: boolean): number {
  const floor = dark ? MIN_CANVAS_SEPARATION.dark : MIN_CANVAS_SEPARATION.light;
  if (Math.abs(anchorL - canvasL) >= floor) return anchorL;
  // Push away from the canvas, in the direction the mode already implies.
  return dark ? Math.min(0.97, canvasL + floor) : Math.max(0.06, canvasL - floor);
}

/** `--accent-glow` sits harder in the dark, where it has less to fight. */
const GLOW_ALPHA = { light: 0.26, dark: 0.32 } as const;

/** `--structure` is the deep counterpart to the accent — the second stop
 *  of the brand mark's gradient. 45° off the primary, fixed lightness. */
const STRUCTURE_L = { light: 0.44, dark: 0.56 } as const;

/**
 * The theme's saturation character, normalised against the Tidal default
 * (~0.118). Nord lands ~0.6, Synthwave ~1.5. Every derived chroma is
 * multiplied by it, which is why a muted theme gets a muted aurora and a
 * neon one gets a neon aurora from the same table above.
 */
function chromaScale(primary: Oklch): number {
  return Math.min(1.6, Math.max(0.55, primary[1] / 0.118));
}

export function deriveSurface(sources: SurfaceSources, variant: 'light' | 'dark'): DerivedTokens {
  const { background, card, primary, mutedForeground } = sources;
  const [pL, pC, pH] = primary;
  const [cL, cC, cH] = card;
  const dark = variant === 'dark';
  const scale = chromaScale(primary);

  // What a shadow, a hairline or the modal scrim is *made of*. Ink carries
  // the theme's hue at a fraction of its chroma, so a shadow on Gruvbox is
  // a warm shadow — a neutral grey one would read as borrowed.
  const inkH = mutedForeground ? mutedForeground[2] : pH;
  const ink = (L: number, C: number, a: number) => `oklch(${L} ${C} ${inkH.toFixed(0)} / ${a})`;

  const out: DerivedTokens = {};

  // ── Panels ────────────────────────────────────────────────────────
  out['canvas-2'] = dark
    ? formatClamped(Math.min(0.999, cL + 0.037), cC, cH)
    : formatClamped(Math.min(1, cL + 0.017), cC * 0.4, cH);
  // Light mode's glass is opaque white and owes nothing to the theme; only
  // the dark one is a lifted copy of the card.
  if (dark) out['glass-solid'] = formatClamped(cL + 0.009, cC, cH);

  // ── Hairlines, grid, scrim, shadows ───────────────────────────────
  // In the dark these are white alpha and carry no theme hue, so only the
  // scrim and the shadows are derived.
  if (dark) {
    out['scrim'] = ink(0.1, 0.02, 0.62);
    out['shadow-1'] = `0 1px 2px ${ink(0.1, 0.02, 0.4)}`;
    out['shadow-2'] = `0 2px 5px ${ink(0.1, 0.02, 0.34)}, 0 14px 34px -16px ${ink(0.08, 0.02, 0.72)}`;
    out['shadow-3'] = `0 4px 10px ${ink(0.1, 0.02, 0.38)}, 0 32px 64px -24px ${ink(0.06, 0.02, 0.82)}`;
  } else {
    out['hairline'] = ink(0.52, 0.045, 0.17);
    out['hairline-2'] = ink(0.52, 0.045, 0.3);
    out['grid-line'] = ink(0.42, 0.05, 0.09);
    out['scrim'] = ink(0.42, 0.05, 0.28);
    out['shadow-1'] = `0 1px 2px ${ink(0.45, 0.04, 0.1)}`;
    out['shadow-2'] = `0 2px 5px ${ink(0.45, 0.04, 0.09)}, 0 14px 34px -16px ${ink(0.42, 0.05, 0.2)}`;
    out['shadow-3'] = `0 4px 10px ${ink(0.45, 0.04, 0.1)}, 0 32px 64px -24px ${ink(0.42, 0.05, 0.24)}`;
  }

  // ── The accent and its gradients ──────────────────────────────────
  const pc = clampChroma(pL, pC, pH);
  out['accent-wash'] = formatOklch(pL, pc, normalizeHue(pH), 0.13);
  out['accent-glow'] = formatOklch(pL, pc, normalizeHue(pH), GLOW_ALPHA[variant]);

  const structureH = normalizeHue(pH + 45);
  out['structure'] = formatClamped(STRUCTURE_L[variant], 0.118 * scale, structureH);

  const [bgL] = background;
  AURORA[variant].forEach(([dh, c, l, a], i) => {
    const key = (['a', 'b', 'c'] as const)[i]!;
    const hue = normalizeHue(pH + dh);
    const L = separateFromCanvas(l, bgL, dark);
    out[`aurora-${key}`] = formatOklch(L, clampChroma(L, c * scale, hue), hue, a);
  });

  return out;
}
