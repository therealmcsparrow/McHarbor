// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useAuth } from '@core/auth/useAuth';
import { useAppearanceSettings } from '@resources/hooks/useAppearanceSettings';
import type { AppearanceSettings } from '@resources/types/settings';
import {
  DEFAULT_BASE_FONT_PX,
  clampBaseFontPx,
  useThemeCustomizerStore,
  type ThemeAccent,
  type ThemeFont,
} from '@resources/stores/themeCustomizer';
import { DEFAULT_THEME_ID } from '../../src/themes';

/**
 * What the shipped install looks like before anything is configured, and what
 * the app paints on the login screen where the settings query is not allowed
 * to run. These are the values the frontend hardcoded before appearance
 * became configurable, so a first paint looks identical to how it always has.
 */
export const APPEARANCE_FALLBACK: AppearanceSettings = {
  themeId: DEFAULT_THEME_ID,
  accent: 'theme',
  font: 'inter-tight',
  baseFontPx: DEFAULT_BASE_FONT_PX,
  allowUserOverride: true,
};

/** Everything the token applier needs, plus who decided it. */
export type ResolvedAppearance = AppearanceSettings & {
  /** True when this account's own overrides are in effect. */
  overridden: boolean;
  /**
   * The app-wide layer this was merged from, before any override. Kept
   * alongside the merged answer because "reset" on the personal card means
   * "go back to what the operator set", and the merged value cannot say what
   * that was once an override has replaced it.
   */
  defaults: AppearanceSettings;
};

/**
 * Merges the app-wide appearance defaults with this account's overrides.
 *
 * The same two rules the toast resolver uses decide the answer:
 *
 *   1. An override only counts while the operator allows overrides at all.
 *      Revoking the permission does not delete anyone's choices, so a user's
 *      columns can be set and still not apply — the check is on the flag, not
 *      on whether values exist.
 *   2. An override is per-field. A user who only changed the font keeps
 *      following the default theme, and keeps following it when the operator
 *      later changes that default.
 *
 * While the query is still in flight the *locally cached* answer stands in
 * rather than the shipped default. Without that, every reload would paint the
 * stock theme for one frame before snapping to the real one — the cache is
 * written by `useCacheAppearance` below precisely so the cold boot is right.
 */
export function useResolvedAppearance(): ResolvedAppearance {
  const user = useAuth((s) => s.user);
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  const { data } = useAppearanceSettings({ enabled: isAuthenticated });

  const cachedThemeId = useThemeCustomizerStore((s) => s.themeId);
  const cachedAccent = useThemeCustomizerStore((s) => s.accent);
  const cachedFont = useThemeCustomizerStore((s) => s.font);
  const cachedBaseFontPx = useThemeCustomizerStore((s) => s.baseFontPx);

  const base: AppearanceSettings = data ?? {
    themeId: cachedThemeId,
    accent: cachedAccent,
    font: cachedFont,
    baseFontPx: cachedBaseFontPx,
    allowUserOverride: APPEARANCE_FALLBACK.allowUserOverride,
  };

  if (!base.allowUserOverride || !user) {
    return { ...base, overridden: false, defaults: base };
  }

  return {
    themeId: user.appearanceThemeId ?? base.themeId,
    accent: (user.appearanceAccent as ThemeAccent | undefined) ?? base.accent,
    font: (user.appearanceFont as ThemeFont | undefined) ?? base.font,
    baseFontPx: clampBaseFontPx(user.appearanceBaseFontPx ?? base.baseFontPx),
    allowUserOverride: base.allowUserOverride,
    defaults: base,
    overridden:
      user.appearanceThemeId !== undefined ||
      user.appearanceAccent !== undefined ||
      user.appearanceFont !== undefined ||
      user.appearanceBaseFontPx !== undefined,
  };
}

/**
 * Mirrors the resolved appearance into the persisted store so the *next* cold
 * boot paints it before the session and the settings query have answered.
 *
 * The store is a cache here, not a source of truth: nothing reads these four
 * fields back except `useResolvedAppearance`, and only while `data` is
 * undefined. Writing them cannot loop — the effect compares first and the
 * store's own value is what it would write.
 */
export function useCacheAppearance(resolved: ResolvedAppearance): void {
  const isAuthenticated = useAuth((s) => s.isAuthenticated);

  useEffect(() => {
    // Before sign-in the resolved values *are* the cache, so writing them back
    // would only re-affirm what is already there.
    if (!isAuthenticated) return;
    const store = useThemeCustomizerStore.getState();
    if (
      store.themeId === resolved.themeId &&
      store.accent === resolved.accent &&
      store.font === resolved.font &&
      store.baseFontPx === resolved.baseFontPx
    ) {
      return;
    }
    useThemeCustomizerStore.setState({
      themeId: resolved.themeId,
      accent: resolved.accent,
      font: resolved.font,
      baseFontPx: resolved.baseFontPx,
    });
  }, [isAuthenticated, resolved.themeId, resolved.accent, resolved.font, resolved.baseFontPx]);
}
