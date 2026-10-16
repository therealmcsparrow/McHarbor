// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** `'theme'` means "whatever primary the active theme ships" - the
 *  default, and the only value that leaves a theme's own identity intact.
 *  The rest replace `--primary` / `--ring` across the app. */
export type ThemeAccent = 'theme' | 'blue' | 'green' | 'purple' | 'orange' | 'rose' | 'cyan';

/** The accents offered in Settings, in the order they are shown.
 *  `'theme'` is not in here - it is rendered as its own leading tile. */
export const ACCENT_CHOICES: readonly Exclude<ThemeAccent, 'theme'>[] = [
  'blue',
  'green',
  'purple',
  'orange',
  'rose',
  'cyan',
];

export type ThemeDensity = 'compact' | 'comfortable' | 'spacious';

/** The interface fonts the picker offers. Mirrors `coreSettings.AppearanceFonts`
 *  on the backend — the two lists have to agree or a saved choice is rejected
 *  on write and dropped on read.
 *
 *  Three tiers, in the order the picker lists them:
 *
 *    1. The faces McHarbor self-hosts under `public/fonts`. Always available.
 *    2. OS-native stacks. Always available, nothing to download.
 *    3. Google Fonts, fetched from Google's CDN the first time one is used.
 *       These are the only entries that need the network, and every one of
 *       their stacks ends in a tier-1 or tier-2 face, so an air-gapped install
 *       renders the fallback instead of failing. */
export type ThemeFont =
  // 1. Bundled
  | 'inter-tight'
  | 'bricolage'
  | 'jetbrains-mono'
  // 2. System
  | 'system'
  | 'serif'
  | 'humanist'
  // 3. Google — sans
  | 'inter'
  | 'roboto'
  | 'open-sans'
  | 'lato'
  | 'montserrat'
  | 'poppins'
  | 'nunito-sans'
  | 'source-sans-3'
  | 'work-sans'
  | 'manrope'
  | 'plus-jakarta-sans'
  | 'dm-sans'
  | 'ibm-plex-sans'
  | 'atkinson-hyperlegible'
  // 3. Google — serif
  | 'source-serif-4'
  | 'merriweather'
  | 'lora'
  // 3. Google — mono
  | 'roboto-mono'
  | 'ibm-plex-mono'
  | 'fira-code';

export const FONT_CHOICES: readonly ThemeFont[] = [
  'inter-tight',
  'bricolage',
  'jetbrains-mono',
  'system',
  'serif',
  'humanist',
  'inter',
  'roboto',
  'open-sans',
  'lato',
  'montserrat',
  'poppins',
  'nunito-sans',
  'source-sans-3',
  'work-sans',
  'manrope',
  'plus-jakarta-sans',
  'dm-sans',
  'ibm-plex-sans',
  'atkinson-hyperlegible',
  'source-serif-4',
  'merriweather',
  'lora',
  'roboto-mono',
  'ibm-plex-mono',
  'fira-code',
];

/** The `family=` fragment of a Google Fonts css2 request, per font id.
 *
 *  Only ids listed here are fetched from the network; membership in this map
 *  is what makes a font "a Google font" everywhere else in the app, so there
 *  is no second list to keep in step.
 *
 *  Weight ranges are the variable axis where the family ships one, and an
 *  explicit weight list where it does not — asking for a range a family does
 *  not have returns a 400 and no stylesheet at all. */
export const GOOGLE_FONT_SPECS: Partial<Record<ThemeFont, string>> = {
  inter: 'Inter:wght@100..900',
  roboto: 'Roboto:wght@100..900',
  'open-sans': 'Open+Sans:wght@300..800',
  lato: 'Lato:wght@300;400;700;900',
  montserrat: 'Montserrat:wght@100..900',
  poppins: 'Poppins:wght@300;400;500;600;700',
  'nunito-sans': 'Nunito+Sans:wght@200..1000',
  'source-sans-3': 'Source+Sans+3:wght@200..900',
  'work-sans': 'Work+Sans:wght@100..900',
  manrope: 'Manrope:wght@200..800',
  'plus-jakarta-sans': 'Plus+Jakarta+Sans:wght@200..800',
  'dm-sans': 'DM+Sans:wght@100..1000',
  'ibm-plex-sans': 'IBM+Plex+Sans:wght@100;200;300;400;500;600;700',
  'atkinson-hyperlegible': 'Atkinson+Hyperlegible:wght@400;700',
  'source-serif-4': 'Source+Serif+4:wght@200..900',
  merriweather: 'Merriweather:wght@300;400;700;900',
  lora: 'Lora:wght@400..700',
  'roboto-mono': 'Roboto+Mono:wght@100..700',
  'ibm-plex-mono': 'IBM+Plex+Mono:wght@100;200;300;400;500;600;700',
  'fira-code': 'Fira+Code:wght@300..700',
};

/** Whether a font id is fetched from Google's CDN rather than shipped or
 *  supplied by the OS — i.e. whether it needs a working internet connection. */
export function isGoogleFont(font: ThemeFont): boolean {
  return font in GOOGLE_FONT_SPECS;
}

/** The stack each font id writes to `--font-sans`.
 *
 *  The first three are the variable faces McHarbor self-hosts under
 *  `public/fonts` and declares in `fonts.css`; the rest are OS-native stacks
 *  that need no download at all. Nothing here reaches a CDN — McHarbor is
 *  frequently deployed air-gapped, which is the same reason the brand faces
 *  ship in the image rather than being pulled at runtime.
 *
 *  Every stack ends in a generic family so a face that fails to load still
 *  renders something sensible. */
export const FONT_STACKS: Record<ThemeFont, string> = {
  // 1. Bundled.
  'inter-tight': "'Inter Tight', system-ui, -apple-system, 'Segoe UI', sans-serif",
  bricolage: "'Bricolage Grotesque', 'Inter Tight', system-ui, sans-serif",
  'jetbrains-mono': "'JetBrains Mono', ui-monospace, 'Cascadia Code', monospace",
  // 2. System.
  system: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif",
  serif: "Georgia, Cambria, 'Times New Roman', Times, serif",
  humanist: "Verdana, Geneva, 'DejaVu Sans', Tahoma, sans-serif",
  // 3. Google. Each falls back to the bundled face of the same kind, so an
  //    install with no route to fonts.gstatic.com renders Inter Tight /
  //    Georgia / JetBrains Mono rather than the browser's default.
  inter: "'Inter', 'Inter Tight', system-ui, sans-serif",
  roboto: "'Roboto', 'Inter Tight', system-ui, sans-serif",
  'open-sans': "'Open Sans', 'Inter Tight', system-ui, sans-serif",
  lato: "'Lato', 'Inter Tight', system-ui, sans-serif",
  montserrat: "'Montserrat', 'Inter Tight', system-ui, sans-serif",
  poppins: "'Poppins', 'Inter Tight', system-ui, sans-serif",
  'nunito-sans': "'Nunito Sans', 'Inter Tight', system-ui, sans-serif",
  'source-sans-3': "'Source Sans 3', 'Inter Tight', system-ui, sans-serif",
  'work-sans': "'Work Sans', 'Inter Tight', system-ui, sans-serif",
  manrope: "'Manrope', 'Inter Tight', system-ui, sans-serif",
  'plus-jakarta-sans': "'Plus Jakarta Sans', 'Inter Tight', system-ui, sans-serif",
  'dm-sans': "'DM Sans', 'Inter Tight', system-ui, sans-serif",
  'ibm-plex-sans': "'IBM Plex Sans', 'Inter Tight', system-ui, sans-serif",
  'atkinson-hyperlegible': "'Atkinson Hyperlegible', 'Inter Tight', system-ui, sans-serif",
  'source-serif-4': "'Source Serif 4', Georgia, Cambria, serif",
  merriweather: "'Merriweather', Georgia, Cambria, serif",
  lora: "'Lora', Georgia, Cambria, serif",
  'roboto-mono': "'Roboto Mono', 'JetBrains Mono', ui-monospace, monospace",
  'ibm-plex-mono': "'IBM Plex Mono', 'JetBrains Mono', ui-monospace, monospace",
  'fira-code': "'Fira Code', 'JetBrains Mono', ui-monospace, monospace",
};

/** Bounds for the root font size, mirroring the backend's
 *  `AppearanceMinBaseFontPx` / `AppearanceMaxBaseFontPx`.
 *
 *  This is the *base* size only. Every relative size in the app — `text-xs`
 *  through `text-2xl`, spacing, radii — is expressed in rem and keeps its own
 *  multiplier; changing this moves all of them together, which is exactly
 *  what "the normal size the others derive from" means.
 *
 *  12px is the smallest the tabular figures in the metrics tables stay
 *  legible at; past 20px the sidebar stops fitting its labels on one line. */
export const MIN_BASE_FONT_PX = 12;
export const MAX_BASE_FONT_PX = 20;
export const DEFAULT_BASE_FONT_PX = 16;

/** Pin a root font size into the supported range. */
export function clampBaseFontPx(px: number): number {
  if (!Number.isFinite(px)) return DEFAULT_BASE_FONT_PX;
  return Math.min(MAX_BASE_FONT_PX, Math.max(MIN_BASE_FONT_PX, Math.round(px)));
}

export const ACCENT_TOKENS: Record<Exclude<ThemeAccent, 'theme'>, { primary: string; ring: string }> = {
  blue: { primary: '0.65 0.20 250', ring: '0.65 0.20 250' },
  green: { primary: '0.65 0.18 145', ring: '0.65 0.18 145' },
  purple: { primary: '0.60 0.22 290', ring: '0.60 0.22 290' },
  orange: { primary: '0.70 0.18 45', ring: '0.70 0.18 45' },
  rose: { primary: '0.65 0.22 5', ring: '0.65 0.22 5' },
  cyan: { primary: '0.70 0.15 200', ring: '0.70 0.15 200' },
};

export const DENSITY_TOKENS: Record<ThemeDensity, { spacing: string; radius: string }> = {
  compact: { spacing: '0.92', radius: '0.375rem' },
  comfortable: { spacing: '1', radius: '0.5rem' },
  spacious: { spacing: '1.08', radius: '0.75rem' },
};

/** Wrap a bare oklch triple ("L C H") in `oklch(...)` so it is a
 *  valid CSS color value when written to a custom property. */
