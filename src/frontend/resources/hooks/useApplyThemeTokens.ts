// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { useThemeStore } from '@resources/stores/theme';
import {
  ACCENT_TOKENS,
  DENSITY_TOKENS,
  FONT_STACKS,
  THEME_VARS,
  overrideKey,
  useThemeCustomizerStore,
  type ThemeAccent,
  type ThemeVarOverride,
} from '@resources/stores/themeCustomizer';
import { useCacheAppearance, useResolvedAppearance } from '@resources/hooks/useResolvedAppearance';
import { DERIVED_TOKENS, deriveSurface, type SurfaceSources } from '@resources/theme/derive-surface';
import { parseOklch, type Oklch } from '@resources/theme/oklch';
import { bumpThemeTokenVersion } from '@resources/theme/token-version';
import { ensureGoogleFont } from '@resources/theme/google-fonts';
import { syncThemeStylesheet } from '../../src/themes';

/** Every property this hook owns on `<html>`. Anything in here without a
 *  value this pass is removed, so the theme stylesheet shows through. */
const MANAGED_VARS: readonly string[] = [...THEME_VARS, ...DERIVED_TOKENS];

/**
 * The editable tokens the surface language is built from. An override to
 * any of these has to re-derive the aurora, the glass and the shadows —
 * that is the whole point of the derivation.
 */
const SOURCE_VARS = ['background', 'card', 'primary', 'muted-foreground'] as const;

function setVar(itemName: string, value: string | undefined) {
  const root = document.documentElement;
  if (!value) {
    root.style.removeProperty(itemName);
    return;
  }
  // The store wraps bare oklch triples on write, but a stale value from an
  // earlier build may still be sitting in localStorage. Wrap it now, or the
  // property holds a string the browser will not parse as a colour.
  let v = value;
  if (
    !v.startsWith('oklch(') &&
    !v.startsWith('rgb(') &&
    !v.startsWith('hsl(') &&
    !v.startsWith('#') &&
    /^\s*[0-9.]+%?\s+[0-9.]+%?\s+[0-9.]+/.test(v)
  ) {
    v = `oklch(${v})`;
  }
  root.style.setProperty(itemName, v);
}

/**
 * The value the app is *actually* rendering for a token: the inline
 * override if this pass set one, otherwise whatever the active theme
 * stylesheet resolves to.
 *
 * Custom properties are not computed to a colour, so `getPropertyValue`
 * hands back the literal token text — `oklch(0.75 0.11 340)` — which is
 * exactly what `parseOklch` wants.
 */
function effective(itemName: string, inline: Map<string, string>): Oklch | null {
  const own = inline.get(itemName);
  if (own) return parseOklch(own);
  if (typeof document === 'undefined') return null;
  return parseOklch(getComputedStyle(document.documentElement).getPropertyValue(`--${itemName}`).trim());
}

/** The accent layer: a preset that replaces the theme's primary. */
function applyAccent(accent: ThemeAccent, out: Map<string, string>) {
  if (accent === 'theme') return;
  const tokens = ACCENT_TOKENS[accent];
  out.set('primary', `oklch(${tokens.primary})`);
  out.set('ring', `oklch(${tokens.ring})`);
  out.set('sidebar-primary', `oklch(${tokens.primary})`);
  out.set('sidebar-ring', `oklch(${tokens.ring})`);
}

/**
 * Applies the active theme to the document: the stylesheet, the accent,
 * the user's variable overrides, and everything the three of them imply.
 *
 * Called once from `ThemeProvider`. The `.dark` class is deliberately not
 * touched here — `useThemeStore` owns light/dark, and having two writers
 * meant a topbar toggle and a Settings change could each undo the other.
 * This hook only mirrors the resolved variant into the customizer store so
 * overrides are keyed against the variant that is actually on screen.
 */
export function useApplyThemeTokens() {
  // Theme, accent, font and base font size are governed: the operator sets an
  // app-wide default and each account may override it while that is allowed.
  // `useResolvedAppearance` is where those two layers become one answer; the
  // store is only consulted through it, as the cold-boot cache.
  const appearance = useResolvedAppearance();
  useCacheAppearance(appearance);
  const { themeId, accent, font, baseFontPx } = appearance;

  // Density and the per-variable colour overrides stay local. They are power
  // tools on the Themes tab rather than part of the governed set, and nothing
  // an operator would want to reset across an install.
  const variant = useThemeCustomizerStore((s) => s.variant);
  const density = useThemeCustomizerStore((s) => s.density);
  const overrides = useThemeCustomizerStore((s) => s.overrides);
  const setVariant = useThemeCustomizerStore((s) => s.setVariant);
  const resolvedTheme = useThemeStore((s) => s.resolvedTheme);

  // Bumped when a theme stylesheet finishes loading. The derivation reads
  // the theme's own tokens for anything the user has not overridden, and
  // those are only readable once the sheet is applied — without this, the
  // first derivation after a theme switch would read the *previous*
  // theme's values.
  const [sheetTick, setSheetTick] = useState(0);
  const onSheetReady = useCallback(() => setSheetTick((n) => n + 1), []);

  // 1. Keep the customizer's variant in step with the resolved light/dark
  //    mode, so `overrideKey` names the variant the user is looking at.
  useEffect(() => {
    if (resolvedTheme !== variant) setVariant(resolvedTheme);
  }, [resolvedTheme, variant, setVariant]);

  // 2. Make the selected theme the winning stylesheet.
  useEffect(() => {
    syncThemeStylesheet(themeId, onSheetReady);
  }, [themeId, onSheetReady]);

  // 3. Write the inline layer — accent, overrides, and the surface
  //    language derived from both. One pass, so nothing depends on the
  //    order two effects happen to run in.
  useEffect(() => {
    if (typeof document === 'undefined') return;
    void sheetTick;

    // Clear first. `effective()` below reads the theme's own value for
    // anything the user has not overridden, and an inline property beats
    // the stylesheet - so leaving the previous pass's values in place
    // would derive the new theme's aurora from the old theme's primary.
    // Nothing paints between here and the write at the end of the effect.
    for (const itemName of MANAGED_VARS) {
      document.documentElement.style.removeProperty(`--${itemName}`);
    }

    const current: Partial<ThemeVarOverride> = overrides[overrideKey(themeId, resolvedTheme)] ?? {};
    const inline = new Map<string, string>();

    applyAccent(accent, inline);
    // An explicit override always beats the accent — the user reached past
    // the preset to name a colour.
    for (const itemName of THEME_VARS) {
      const value = current[itemName];
      if (value) inline.set(itemName, value);
    }

    // Nothing customised means the theme's own generated surface language
    // is already right for it. Re-deriving would only round its values.
    const customised = accent !== 'theme' || SOURCE_VARS.some((itemName) => inline.has(itemName));

    if (customised) {
      const background = effective('background', inline);
      const card = effective('card', inline);
      const primary = effective('primary', inline);
      if (background && card && primary) {
        const sources: SurfaceSources = {
          background,
          card,
          primary,
          mutedForeground: effective('muted-foreground', inline),
        };
        for (const [itemName, value] of Object.entries(deriveSurface(sources, resolvedTheme))) {
          inline.set(itemName, value);
        }
      }
    }

    for (const itemName of MANAGED_VARS) {
      setVar(`--${itemName}`, inline.get(itemName));
    }

    // The values on <html> are now true. Anything that reads them back
    // with getComputedStyle - the Themes tab's swatches - can re-read.
    bumpThemeTokenVersion();
  }, [themeId, resolvedTheme, accent, overrides, sheetTick]);

  // 4. Density sits outside the colour system.
  useEffect(() => {
    const densityTokens = DENSITY_TOKENS[density];
    setVar('--density-scale', densityTokens.spacing);
    setVar('--radius', densityTokens.radius);
    document.documentElement.classList.toggle('theme-compact', density === 'compact');
  }, [density]);

  // 5. Type: the interface font and the root size everything else is
  //    measured from. Set in px rather than rem — a rem here would compound
  //    against itself on every pass, and px is also the number the picker
  //    shows, so what the user chose is literally what lands on <html>.
  //    Tailwind's own steps (text-xs … text-2xl) are rem and keep their
  //    multipliers, so they follow this one value.
  useEffect(() => {
    // A Google family has to be fetched before it can render. The stack is
    // written either way — it ends in a bundled face, so an offline install
    // paints that instead of waiting on a request that will not arrive.
    ensureGoogleFont(font);
    setVar('--font-sans', FONT_STACKS[font]);
    document.documentElement.style.fontSize = `${baseFontPx}px`;
  }, [font, baseFontPx]);
}
