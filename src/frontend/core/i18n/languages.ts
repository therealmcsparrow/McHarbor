// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const supportedLanguages = ['en', 'nl', 'de', 'es', 'fr', 'pt'] as const;
export type SupportedLanguage = (typeof supportedLanguages)[number];

export const languageLabels: Record<SupportedLanguage, string> = {
  en: 'English',
  nl: 'Nederlands',
  de: 'Deutsch',
  es: 'Español',
  fr: 'Français',
  pt: 'Português',
};

export function normalizeLanguage(value: string | null | undefined): SupportedLanguage {
  const short = value?.substring(0, 2).toLowerCase();
  return supportedLanguages.includes(short as SupportedLanguage) ? (short as SupportedLanguage) : 'en';
}

let cachedDefaultLanguage: string | null = null;

function setCachedDefaultLanguage(value: string | undefined): void {
  if (value && value.length > 0) {
    cachedDefaultLanguage = value;
  }
}

export function getCachedDefaultLanguage(): string | null {
  return cachedDefaultLanguage;
}

export async function detectInitialLanguage(): Promise<SupportedLanguage> {
  // 1. localStorage override set by the user previously.
  try {
    const stored = window.localStorage.getItem('mcharbor-i18next-lang');
    if (stored) {
      return normalizeLanguage(stored);
    }
  } catch {
    // Ignore localStorage access failures.
  }

  // 2. App-wide default set by an admin in Settings → General.
  //    The status endpoint is fetched once at boot and cached.
  if (cachedDefaultLanguage === null) {
    try {
      const res = await fetch('/api/auth/status', { credentials: 'omit' });
      if (res.ok) {
        const data = (await res.json()) as { defaultLanguage?: string };
        if (data?.defaultLanguage) {
          setCachedDefaultLanguage(data.defaultLanguage);
        }
      }
    } catch {
      // Network failure falls through to the browser / hard-coded
      // defaults. The auth-module's checkSession will refresh the
      // value on the next login round-trip.
    }
    if (cachedDefaultLanguage === null) {
      cachedDefaultLanguage = 'en';
    }
  }
  if (cachedDefaultLanguage) {
    return normalizeLanguage(cachedDefaultLanguage);
  }

  // 3. Browser language as a final fallback.
  try {
    return normalizeLanguage(window.navigator.language);
  } catch {
    return 'en';
  }
}
