// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// resolveToken — read a theme token as a concrete colour value.
//
// Most of the UI consumes theme colour as a Tailwind class, which is the
// right answer: the browser resolves it and a theme swap needs no
// JavaScript. A few consumers cannot take a class, because they paint
// into a canvas or build their own stylesheet from a JS object:
//
//   * xterm.js terminals (ITheme)
//   * CodeMirror editor themes
//   * collaboration cursor colours
//
// Those used to hold hex literals, which is why a terminal stayed dark
// slate on every one of the 18 themes. They now read the same tokens
// everything else uses, so the terminal is themed like the rest of the app.

import { CHART_STEPS, type ChartStep, type StatusTone } from './status-palette';

/**
 * Resolve a CSS custom property to its computed value.
 *
 * Returns `fallback` during SSR, in tests without a DOM, or when the
 * token is undefined — a caller painting a canvas needs *some* colour,
 * and failing to a readable default beats painting `''`.
 */
export function resolveToken(itemName: string, fallback = '#000000'): string {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return fallback;
  }
  const prop = itemName.startsWith('--') ? itemName : `--${itemName}`;
  const value = getComputedStyle(document.documentElement).getPropertyValue(prop).trim();
  return value || fallback;
}

/** Resolve a token and apply an alpha channel, staying in OKLCH. */
export function resolveTokenAlpha(itemName: string, alpha: number, fallback = '#000000'): string {
  const value = resolveToken(itemName, fallback);
  if (!value) return fallback;
  return `color-mix(in oklch, ${value} ${Math.round(alpha * 100)}%, transparent)`;
}

export function resolveStatus(tone: StatusTone, fallback?: string): string {
  return resolveToken(`--status-${tone}`, fallback);
}

export function resolveChart(step: ChartStep, fallback?: string): string {
  return resolveToken(`--chart-${step}`, fallback);
}

/** The full chart ramp as computed values — for palette-driven consumers. */
export function resolveChartRamp(): string[] {
  return CHART_STEPS.map((s) => resolveChart(s));
}

/**
 * The surface + text tokens a terminal or editor theme needs, resolved
 * in one pass. Call this on mount and again when the theme changes.
 */
export type SurfaceTokens = {
  background: string;
  foreground: string;
  card: string;
  border: string;
  muted: string;
  mutedForeground: string;
  primary: string;
  selection: string;
  cursor: string;
};

export function resolveSurfaceTokens(): SurfaceTokens {
  return {
    background: resolveToken('--background', '#111418'),
    foreground: resolveToken('--foreground', '#e8eef2'),
    card: resolveToken('--card', '#171c22'),
    border: resolveToken('--border', '#2a323b'),
    muted: resolveToken('--muted', '#1f262d'),
    mutedForeground: resolveToken('--muted-foreground', '#8a97a3'),
    primary: resolveToken('--primary', '#26b0c6'),
    selection: resolveTokenAlpha('--primary', 0.28, 'rgba(38,176,198,0.28)'),
    cursor: resolveToken('--primary', '#26b0c6'),
  };
}

/**
 * ANSI 16 for terminal emulators, mapped onto the status tones and the
 * chart ramp so a terminal restyles with its theme. Bright variants take
 * the same token — the tokens are already tuned for their mode, and
 * doubling them up would reintroduce the light/dark drift the token
 * contract exists to remove.
 */
export function resolveAnsiPalette(): Record<string, string> {
  const surface = resolveSurfaceTokens();
  const black = surface.background;
  const white = surface.foreground;
  const red = resolveStatus('danger', '#d44953');
  const green = resolveStatus('success', '#31ab6a');
  const yellow = resolveStatus('warning', '#cb7e00');
  const blue = resolveChart(5, '#257ecc');
  const magenta = resolveChart(3, '#816cd5');
  const cyan = resolveChart(1, '#00a6b4');

  return {
    black,
    red,
    green,
    yellow,
    blue,
    magenta,
    cyan,
    white: surface.mutedForeground,
    brightBlack: surface.border,
    brightRed: red,
    brightGreen: green,
    brightYellow: yellow,
    brightBlue: blue,
    brightMagenta: magenta,
    brightCyan: cyan,
    brightWhite: white,
  };
}
