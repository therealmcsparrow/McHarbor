// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Theme manifest for McHarbor.
//
// Each entry points at a CSS file in the project root
// `themes/` directory. The CSS file overrides the same set of
// CSS custom properties declared in `src/frontend/app.css`
// (--background, --foreground, --primary, --card, --border, etc.)
// for both the :root and .dark selectors.
//
// To add a new theme:
//   1. Drop a new .css file in <project-root>/themes/.
//   2. Add an entry below with the filename + display metadata.
//   3. Run `npm run sync:themes` (or just `npm run dev` /
//      `npm run build`) — the script copies the CSS into
//      public/themes/ so Vite serves it as a static asset at
//      /themes/<id>.css.
//
// The frontend loads the active theme at runtime via a
// <link rel="stylesheet"> tag injected by the
// useApplyThemeTokens hook.

export type ThemeId = string;

export type ThemeVariant = 'light' | 'dark';

export type ThemeManifestEntry = {
  id: ThemeId;
  /** Display name shown in the picker. */
  name: string;
  /** One-line description. */
  description: string;
  /** Path to the CSS file, relative to the web origin. */
  cssPath: string;
  /** Hue used to render the swatch in the picker (oklch H value). */
  swatchHue: number;
  /** Recommended variant shown as the default preview. */
  defaultVariant: ThemeVariant;
  /** Whether the theme ships with a tuned dark variant. */
  hasDark: boolean;
  /** Whether the theme ships with a tuned light variant. */
  hasLight: boolean;
};

import { THEMES } from './manifest-data';
export { THEMES };

const pluginThemes: ThemeManifestEntry[] = [];

export const DEFAULT_THEME_ID: ThemeId = 'mcharbor-default';

export function registerPluginThemes(themes: ThemeManifestEntry[]) {
  pluginThemes.splice(0, pluginThemes.length, ...themes);
}

export function getAllThemes(): ThemeManifestEntry[] {
  return [...THEMES, ...pluginThemes];
}

export function getTheme(id: ThemeId | (string & {}) | null | undefined): ThemeManifestEntry {
  return getAllThemes().find((t) => t.id === id) ?? (THEMES[0] as ThemeManifestEntry);
}

const THEME_STORAGE_KEY = 'mcharbor-theme-customizer-v3';
const VARIANT_STORAGE_KEY = 'mcharbor-theme';
const LOADED_THEMES = new Set<string>();

/** Read the persisted theme + variant straight from localStorage.
 *  Returns sensible defaults if storage is empty or invalid. */
function readPersisted() {
  if (typeof window === 'undefined') {
    return { themeId: DEFAULT_THEME_ID, variant: 'dark' as ThemeVariant };
  }
  let themeId: ThemeId = DEFAULT_THEME_ID;
  let variant: ThemeVariant = 'dark';
  try {
    const raw = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      const state = parsed?.state ?? parsed;
      if (state && typeof state.themeId === 'string') {
        themeId = state.themeId as ThemeId;
        if (state.variant === 'light' || state.variant === 'dark') {
          variant = state.variant;
        }
      }
    }
  } catch {
    // ignore parse errors, fall back to default
  }
  try {
    const raw2 = window.localStorage.getItem(VARIANT_STORAGE_KEY);
    if (raw2) {
      const parsed2 = JSON.parse(raw2);
      const state2 = parsed2?.state ?? parsed2;
      if (state2 && (state2.theme === 'light' || state2.theme === 'dark' || state2.theme === 'system')) {
        const resolved =
          state2.theme === 'system'
            ? window.matchMedia('(prefers-color-scheme: dark)').matches
              ? 'dark'
              : 'light'
            : state2.theme;
        if (resolved === 'light' || resolved === 'dark') {
          variant = resolved;
        }
      }
    }
  } catch {
    // A malformed persisted variant leaves the default variant in place.
  }
  return { themeId, variant };
}

/**
 * Make `themeId` the winning theme stylesheet, and the only one.
 *
 * A theme is a flat set of `:root` / `.dark` custom properties, so which
 * theme the app renders is decided purely by **cascade order** — the last
 * matching declaration wins. Two things follow, and the previous version
 * of this function got both wrong:
 *
 *   1. Every other theme's `<link>` has to go. Leaving them means the
 *      winner is whichever was appended most recently, not the one the
 *      user picked.
 *   2. The active `<link>` has to be re-appended even when it is already
 *      in the document, so it lands after the app bundle and after any
 *      stylesheet Vite injected during HMR. Re-appending an existing
 *      element *moves* it — no refetch, no flash of unstyled content.
 *
 * Skipping the work when the theme is already last keeps the common case
 * (a re-render that did not change the theme) free.
 *
 * `onReady` fires once the stylesheet is actually applied, so a caller
 * that derives values from the theme's own tokens reads them after the
 * swap rather than before it.
 */
export function syncThemeStylesheet(themeId: ThemeId, onReady?: () => void) {
  if (typeof document === 'undefined') return;
  const theme = getTheme(themeId);
  const href = theme.cssPath.startsWith('/api/') ? theme.cssPath : `/${theme.cssPath.replace(/^\/+/, '')}`;
  const absolute = new URL(href, document.baseURI).href;

  const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[data-theme-stylesheet]'));
  const active = links.find((l) => l.dataset.themeId === theme.id) ?? null;

  const dropOthers = () => {
    for (const link of links) {
      if (link === active || link.dataset.themeId === theme.id) continue;
      link.remove();
      LOADED_THEMES.delete(link.dataset.themeId ?? '');
    }
  };

  if (active && active.href === absolute) {
    if (active !== document.head.lastElementChild) document.head.appendChild(active);
    dropOthers();
    onReady?.();
    return;
  }

  // The new sheet is attached *before* the old one is dropped, and the old
  // one is only dropped once the new one has actually loaded. Removing
  // first left a window with no theme applied at all, during which the app
  // fell back to the app.css defaults - which is what "switching a theme
  // sometimes does not refresh the screen" looked like: a repaint to the
  // wrong colours, then a second repaint whenever the sheet arrived.
  const link = active ?? document.createElement('link');
  link.rel = 'stylesheet';
  link.id = `mcharbor-theme-${theme.id}`;
  link.dataset.themeStylesheet = 'true';
  link.dataset.themeId = theme.id;

  const settle = () => {
    dropOthers();
    onReady?.();
  };
  // `error` settles too: a plugin theme whose CSS 404s must not leave the
  // previous theme's stylesheet stranded in the document forever.
  link.addEventListener('load', settle, { once: true });
  link.addEventListener('error', settle, { once: true });
  link.href = href;

  if (!active) document.head.appendChild(link);
  else if (link !== document.head.lastElementChild) document.head.appendChild(link);
  LOADED_THEMES.add(theme.id);
}

/** Remove every theme stylesheet. Exported for tests / debugging. */
export function clearThemeStylesheets() {
  if (typeof document === 'undefined') return;
  document.querySelectorAll<HTMLLinkElement>('link[data-theme-stylesheet]').forEach((link) => link.remove());
  LOADED_THEMES.clear();
}

/** Apply the persisted theme + variant synchronously. Safe to call
 *  on the server (no-op when document is undefined). Called from
 *  `main.tsx` so the first paint already has the correct theme
 *  instead of flashing the app.css defaults. */
export function applyInitialTheme() {
  if (typeof document === 'undefined') return;
  const { themeId, variant } = readPersisted();
  syncThemeStylesheet(themeId);
  document.documentElement.classList.toggle('dark', variant === 'dark');
}

export const _THEME_STORAGE_KEY = THEME_STORAGE_KEY;
