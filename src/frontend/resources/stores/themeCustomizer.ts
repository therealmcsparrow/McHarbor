// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { DEFAULT_THEME_ID, type ThemeId, type ThemeVariant } from '../../src/themes';

import {
  DEFAULT_BASE_FONT_PX,
  clampBaseFontPx,
  type ThemeAccent,
  type ThemeDensity,
  type ThemeFont,
} from './themeCustomizerTokens';

export * from './themeCustomizerTokens';
function toColorValue(value: string): string {
  if (!value) return '';
  if (value.startsWith('oklch(') || value.startsWith('rgb(') || value.startsWith('hsl(') || value.startsWith('#')) {
    return value;
  }
  // Bare triple — wrap it.
  return `oklch(${value})`;
}

/** Every CSS custom property the app reads from the theme stylesheet.
 *  The user can override any of these per theme+variant. The values
 *  are stored as bare oklch triples (e.g. "0.65 0.20 250") so the
 *  hook can append opacity. */
export const THEME_VARS = [
  'background',
  'foreground',
  'card',
  'card-foreground',
  'popover',
  'popover-foreground',
  'primary',
  'primary-foreground',
  'secondary',
  'secondary-foreground',
  'muted',
  'muted-foreground',
  'accent',
  'accent-foreground',
  'destructive',
  'destructive-foreground',
  'border',
  'input',
  'ring',
  'sidebar',
  'sidebar-foreground',
  'sidebar-primary',
  'sidebar-primary-foreground',
  'sidebar-accent',
  'sidebar-accent-foreground',
  'sidebar-border',
  'sidebar-ring',
] as const;

export type ThemeVarName = (typeof THEME_VARS)[number];

/** A user override of one CSS variable. Empty string means "no
 *  override — use whatever the linked theme stylesheet says". */
export type ThemeVarOverride = Record<ThemeVarName, string>;

type ThemeState = {
  themeId: ThemeId;
  variant: ThemeVariant;
  accent: ThemeAccent;
  density: ThemeDensity;
  font: ThemeFont;
  baseFontPx: number;
  /** Per-theme per-variant variable overrides. Keyed by
   *  `${themeId}:${variant}` so a switch between light/dark keeps
   *  the two independent. */
  overrides: Record<string, ThemeVarOverride>;
  setTheme: (id: ThemeId, variant?: ThemeVariant) => void;
  setVariant: (v: ThemeVariant) => void;
  setAccent: (a: ThemeAccent) => void;
  setDensity: (d: ThemeDensity) => void;
  setFont: (f: ThemeFont) => void;
  setBaseFontPx: (px: number) => void;
  setOverride: (name: ThemeVarName, value: string) => void;
  resetOverrides: () => void;
  reset: () => void;
};

export function overrideKey(themeId: ThemeId, variant: ThemeVariant): string {
  return `${themeId}:${variant}`;
}

function emptyOverride(): ThemeVarOverride {
  const o = {} as ThemeVarOverride;
  for (const v of THEME_VARS) o[v] = '';
  return o;
}

const DEFAULT_STATE: Omit<
  ThemeState,
  | 'setTheme'
  | 'setVariant'
  | 'setAccent'
  | 'setDensity'
  | 'setFont'
  | 'setBaseFontPx'
  | 'setOverride'
  | 'resetOverrides'
  | 'reset'
> = {
  themeId: DEFAULT_THEME_ID,
  variant: 'dark',
  accent: 'theme',
  density: 'comfortable',
  font: 'inter-tight',
  baseFontPx: DEFAULT_BASE_FONT_PX,
  overrides: {},
};

export const useThemeCustomizerStore = create<ThemeState>()(
  persist(
    (set) => ({
      ...DEFAULT_STATE,
      setTheme: (id, variant) =>
        set((s) => ({
          themeId: id,
          variant: variant ?? s.variant,
        })),
      setVariant: (v) => set({ variant: v }),
      setAccent: (a) => set({ accent: a }),
      setDensity: (d) => set({ density: d }),
      setFont: (f) => set({ font: f }),
      setBaseFontPx: (px) => set({ baseFontPx: clampBaseFontPx(px) }),
      setOverride: (itemName, value) =>
        set((s) => {
          const key = overrideKey(s.themeId, s.variant);
          const current = s.overrides[key] ?? emptyOverride();
          return {
            overrides: {
              ...s.overrides,
              [key]: { ...current, [itemName]: toColorValue(value) },
            },
          };
        }),
      resetOverrides: () =>
        set((s) => {
          const key = overrideKey(s.themeId, s.variant);
          const next = { ...s.overrides };
          delete next[key];
          return { overrides: next };
        }),
      reset: () => set({ ...DEFAULT_STATE, overrides: {} }),
    }),
    {
      name: 'mcharbor-theme-customizer-v3',
      version: 5,
      // Versions 0-3 share the `overrides` / `themeId` / `variant`
      // shape, so they are accepted verbatim - without this hook
      // Zustand warns on every boot when an older envelope is loaded.
      //
      // The one thing v4 has to rewrite is `accent`. It defaulted to
      // `'blue'` and was never applied to anything, so every existing
      // install has `'blue'` persisted while still rendering its own
      // theme's primary. Now that the accent is live, honouring that
      // stored value would turn every theme blue on the first load
      // after the upgrade. Anything from before v4 means "never chose
      // an accent".
      //
      // v5 replaces the three-step `scale` multiplier with a real root font
      // size in pixels, because "medium" never answered the question people
      // were actually asking — what size am I reading at. The three old
      // values were 0.94 / 1 / 1.08 rem, so they land on 15 / 16 / 17px.
      migrate: (persistedState, version) => {
        let state = persistedState as ThemeState & { scale?: string };
        if (!state) return state;
        if (version < 4) {
          state = { ...state, accent: 'theme' as ThemeAccent };
        }
        if (version < 5) {
          const fromScale: Record<string, number> = { small: 15, medium: 16, large: 17 };
          const { scale, ...rest } = state;
          state = {
            ...(rest as ThemeState),
            font: 'inter-tight',
            baseFontPx: fromScale[scale ?? 'medium'] ?? DEFAULT_BASE_FONT_PX,
          } as ThemeState & { scale?: string };
        }
        return state;
      },
    },
  ),
);
