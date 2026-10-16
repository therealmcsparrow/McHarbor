// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// OKLCH maths, shared.
//
// Two callers need the same conversions and they must agree, or a colour
// picked in Settings renders as a different colour than the one derived
// from it:
//
//   * `ThemeColorPicker` — hex <-> OKLCH for the sliders and the swatch.
//   * `useApplyThemeTokens` — reads the effective `--primary` and derives
//     the aurora washes, `--accent-wash`, `--accent-glow` and `--structure`
//     from it, so the background gradients follow a customised theme.
//
// Values are kept as an `[L, C, H]` triple in the same units CSS uses:
// L in 0..1, C in 0..0.4-ish, H in degrees.

/** RGB tuple in 0..255. */
export type RGB = [number, number, number];

/** `[L, C, H]` — lightness 0..1, chroma 0..~0.4, hue in degrees. */
export type Oklch = [number, number, number];

export function hexToRgb(hex: string): RGB | null {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex);
  if (!m) return null;
  const n = parseInt(m[1]!, 16);
  return [(n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff];
}

export function rgbToHex(r: number, g: number, b: number): string {
  const t = (n: number) => n.toString(16).padStart(2, '0');
  return `#${t(r)}${t(g)}${t(b)}`;
}

function srgbToLinear(c: number): number {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function linearToSrgb(c: number): number {
  return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
}

export function rgbToOklab(r: number, g: number, b: number): [number, number, number] {
  const lr = srgbToLinear(r / 255);
  const lg = srgbToLinear(g / 255);
  const lb = srgbToLinear(b / 255);
  const l_ = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m_ = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s_ = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l_ + 0.793617785 * m_ - 0.0040720468 * s_,
    1.9779984951 * l_ - 2.428592205 * m_ + 0.4505937099 * s_,
    0.0259040371 * l_ + 0.7827717662 * m_ - 0.808675766 * s_,
  ];
}

export function oklabToRgb(L: number, a: number, b: number): RGB {
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.291485548 * b;
  const l = l_ * l_ * l_;
  const m = m_ * m_ * m_;
  const s = s_ * s_ * s_;
  const lr = 4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s;
  const lg = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s;
  const lb = -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s;
  return [
    Math.max(0, Math.min(255, Math.round(linearToSrgb(lr) * 255))),
    Math.max(0, Math.min(255, Math.round(linearToSrgb(lg) * 255))),
    Math.max(0, Math.min(255, Math.round(linearToSrgb(lb) * 255))),
  ];
}

export function rgbToOklch(r: number, g: number, b: number): Oklch {
  const [L, a, bb] = rgbToOklab(r, g, b);
  const C = Math.hypot(a, bb);
  let h = (Math.atan2(bb, a) * 180) / Math.PI;
  if (h < 0) h += 360;
  return [L, C, h];
}

export function oklchToRgb(L: number, C: number, h: number): RGB {
  const a = C * Math.cos((h * Math.PI) / 180);
  const bb = C * Math.sin((h * Math.PI) / 180);
  return oklabToRgb(L, a, bb);
}

export function oklchToHex(L: number, C: number, h: number): string {
  const [r, g, b] = oklchToRgb(L, C, h);
  return rgbToHex(r, g, b);
}

export function hexToOklch(hex: string): Oklch | null {
  const rgb = hexToRgb(hex);
  if (!rgb) return null;
  return rgbToOklch(rgb[0], rgb[1], rgb[2]);
}

/**
 * Format an OKLCH triple as a CSS colour value.
 *
 * Always wrapped in `oklch()`: the value is written straight into a custom
 * property with `setProperty()`, and a bare `L C H` triple there is a
 * string, not a colour — the browser drops it silently.
 */
export function formatOklch(L: number, C: number, h: number, alpha?: number): string {
  const base = `${L.toFixed(3)} ${C.toFixed(3)} ${h.toFixed(1)}`;
  return alpha === undefined ? `oklch(${base})` : `oklch(${base} / ${alpha})`;
}

function parsePercent(raw: string): number {
  return raw.endsWith('%') ? parseFloat(raw) / 100 : parseFloat(raw);
}

/**
 * Parse anything a theme token or the picker can hold: `oklch(L C H)`,
 * `oklch(L C H / a)`, a bare `L C H` triple, or a `#rrggbb` hex.
 *
 * Returns `null` for values this app never writes (named colours,
 * `color-mix()`, gradients), so callers can fall back rather than guess.
 */
export function parseOklch(value: string): Oklch | null {
  if (!value) return null;
  // Destructured rather than indexed: the first two groups are not optional
  // in either pattern, and pulling them out as bindings lets TypeScript see
  // that without a non-null assertion per group.
  const m = /oklch\(\s*([0-9.]+%?)\s+([0-9.]+%?)\s+([0-9.]+)?\s*(?:\/\s*[0-9.%]+\s*)?\)/i.exec(value);
  if (m) {
    const [, lightness, chroma, hue] = m;
    if (lightness && chroma) {
      return [parsePercent(lightness), parsePercent(chroma), hue ? parseFloat(hue) : 0];
    }
  }
  const m2 = /^\s*([0-9.]+%?)\s+([0-9.]+%?)\s+([0-9.]+)?\s*$/i.exec(value);
  if (m2) {
    const [, lightness, chroma, hue] = m2;
    if (lightness && chroma) {
      return [parsePercent(lightness), parsePercent(chroma), hue ? parseFloat(hue) : 0];
    }
  }
  const rgb = hexToRgb(value);
  if (rgb) return rgbToOklch(rgb[0], rgb[1], rgb[2]);
  return null;
}

/** Ensure a stored value is a valid CSS colour, wrapping a bare triple. */
export function ensureColor(value: string): string {
  if (!value) return '';
  if (value.startsWith('oklch(') || value.startsWith('rgb(') || value.startsWith('hsl(') || value.startsWith('#')) {
    return value;
  }
  const parsed = parseOklch(value);
  return parsed ? formatOklch(parsed[0], parsed[1], parsed[2]) : value;
}

/** Linear-light sRGB for an OKLCH triple, unclamped so gamut can be tested. */
function oklchToLinearRgb(L: number, C: number, H: number): [number, number, number] {
  const h = (H * Math.PI) / 180;
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

function inGamut([r, g, b]: [number, number, number]): boolean {
  return r >= -0.0001 && r <= 1.0001 && g >= -0.0001 && g <= 1.0001 && b >= -0.0001 && b <= 1.0001;
}

/**
 * Reduce chroma until the colour fits sRGB, keeping hue and lightness.
 *
 * A derived colour can ask for more chroma than sRGB has at that lightness
 * - a very light wash at the primary's own saturation, say. Left alone the
 * browser clips per channel, which shifts the hue. Binary-searching the
 * chroma instead keeps the hue exactly where the derivation put it.
 *
 * Identical to `clampChroma` in `scripts/theme-tokens.mjs`: the generator
 * and this file must produce the same value for the same theme, or a
 * customised theme would jump the moment the first override is set.
 */
export function clampChroma(L: number, C: number, H: number): number {
  if (inGamut(oklchToLinearRgb(L, C, H))) return C;
  let lo = 0;
  let hi = C;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (inGamut(oklchToLinearRgb(L, mid, H))) lo = mid;
    else hi = mid;
  }
  return lo;
}

/** `formatOklch` with the chroma clamped into sRGB first. */
export function formatClamped(L: number, C: number, h: number, alpha?: number): string {
  return formatOklch(L, clampChroma(L, C, h), normalizeHue(h), alpha);
}

/** Wrap a hue into 0..360. Derivations rotate by ±62°, so this is needed. */
export function normalizeHue(h: number): number {
  return ((h % 360) + 360) % 360;
}
