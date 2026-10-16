// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useThemeStore } from '@resources/stores/theme';

/**
 * The operator-supplied application marks. Every field is a `data:` URI, and
 * an empty string means "fall through" — the four are independent, so an
 * install can replace the wordmark and keep the icon.
 *
 * The plain field is *the* mark; the `*Dark` field is an override that only
 * applies while the dark variant is active. That asymmetry is what makes one
 * upload work everywhere: a single image serves both themes, and the dark
 * slot only has to be filled when that image does not survive the flip.
 */
export type BrandingSettings = {
  /** The wordmark: expanded sidebar and the login card. */
  logoWide: string;
  /** Replaces `logoWide` while the dark variant is active. */
  logoWideDark: string;
  /** The square mark: the 56px collapsed sidebar rail. */
  logoIcon: string;
  /** Replaces `logoIcon` while the dark variant is active. */
  logoIconDark: string;
  /**
   * PNG renditions of the two **light** marks, base64 with no `data:`
   * prefix.
   *
   * Derived, not authored: the branding card produces them at upload time
   * because consumers that write documents rather than pages — the
   * reporting plugin's PDF and DOCX writers — embed raster only, and a logo
   * is usually uploaded as SVG. Light only, because a report prints on
   * white.
   */
  logoWidePng: string;
  logoIconPng: string;
};

/** The two marks a caller can ask for, independent of variant. */
export type BrandMark = 'wide' | 'icon';

/** The shipped marks, used whenever the operator has not uploaded one. They
 *  are the same image in both variants — McHarbor's own mark reads on either
 *  ground, which is why the dark slots start empty. */
export const BRANDING_FALLBACK: Record<BrandMark, string> = {
  wide: '/logo_McHarbor.svg',
  icon: '/icon_McSparrow.svg',
};

/** The empty set, which is also what a fresh install stores. */
export const BRANDING_EMPTY: BrandingSettings = {
  logoWide: '',
  logoWideDark: '',
  logoIcon: '',
  logoIconDark: '',
  logoWidePng: '',
  logoIconPng: '',
};

/** Largest image the backend will accept, mirrored here so the picker can
 *  reject an oversized file before it spends a second encoding it. */
export const BRANDING_MAX_BYTES = 512 * 1024;

/** What the file picker offers, mirroring the backend's MIME allowlist. */
export const BRANDING_ACCEPT = 'image/svg+xml,image/png,image/jpeg,image/webp,image/gif';

/** The query key the sidebar, the login page and the settings card share. */
export const BRANDING_KEY = ['branding'] as const;

/**
 * The custom logo.
 *
 * `GET /branding` is public rather than authenticated: the login page renders
 * the wordmark before anyone has a session, and it carries nothing but the
 * images an operator uploaded to be shown to every visitor anyway.
 *
 * `staleTime` is long for the same reason the appearance defaults' is — this
 * paints on every page and changes about as often as someone rebrands. The
 * save invalidates the key, so an operator still sees their own change at
 * once.
 */
export function useBranding() {
  return useQuery({
    queryKey: BRANDING_KEY,
    queryFn: () => api.get<BrandingSettings>('/branding').then(assertSuccess),
    staleTime: 5 * 60 * 1000,
  });
}

/**
 * Resolves one mark for the *active* variant to something an `<img src>` can
 * render: the dark override, else the plain upload, else the bundled asset.
 *
 * Reading `resolvedTheme` rather than `theme` matters — `theme` can be
 * `'system'`, which is not a ground a logo can be chosen against.
 *
 * `isCustom` matters to the caller because the bundled wordmark needs an
 * explicit aspect ratio (its SVG declares width/height 100%, which a
 * height-only rule can resolve to zero width) and an uploaded one must not
 * inherit that ratio.
 */
export function useBrandLogo(mark: BrandMark) {
  const { data } = useBranding();
  const resolvedTheme = useThemeStore((s) => s.resolvedTheme);
  const custom = resolveBrandMark(data, mark, resolvedTheme);
  return { src: custom || BRANDING_FALLBACK[mark], isCustom: custom !== '' };
}

/**
 * The fallback chain, as a pure function so the settings preview can run it
 * for a variant that is not the one currently on screen.
 */
export function resolveBrandMark(
  branding: BrandingSettings | undefined,
  mark: BrandMark,
  variant: 'light' | 'dark',
): string {
  if (!branding) return '';
  const base = mark === 'wide' ? branding.logoWide : branding.logoIcon;
  if (variant === 'light') return base;
  const dark = mark === 'wide' ? branding.logoWideDark : branding.logoIconDark;
  return dark || base;
}

export function useSaveBranding() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: BrandingSettings) => api.put('/settings/branding', data).then(assertSuccess),
    meta: { success: t('toast.brandingUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: BRANDING_KEY });
    },
  });
}
