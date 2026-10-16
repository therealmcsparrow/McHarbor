// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

type HostWindow = Window & { McharborHost?: { i18n?: { language?: string } } };

/**
 * formatDateWith renders a date under the application's language rather than the
 * browser's. It reads the host's live i18n instance because importing the host's
 * own formatter would compile the whole host i18n bootstrap into this bundle.
 */
export function formatDateWith(value: Date, options: Intl.DateTimeFormatOptions): string {
  const language = (window as HostWindow).McharborHost?.i18n?.language;
  return new Intl.DateTimeFormat(language || undefined, options).format(value);
}
