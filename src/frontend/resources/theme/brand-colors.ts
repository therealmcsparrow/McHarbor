// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// BRAND_COLORS — third-party brand identities. The one documented
// exemption from the theme token contract.
//
// Every other colour in McHarbor is theme-addressable, because every
// other colour is *ours* to restyle. These are not. Discord blurple is
// Discord's, and a Discord icon rendered in Gruvbox orange is not a
// themed icon, it is a wrong icon — the colour is part of how the user
// recognises the destination in a list of channels.
//
// The same reasoning that keeps proper nouns out of `.instructions/
// donottranslate.md` keeps these hex values out of the token contract.
//
// Rules for this file:
//   * It holds brand marks only — never a status, a metric or a surface.
//   * Values are the vendor's published brand hex, not an approximation.
//   * Anything added here needs a source and a reason in the comment.
//
// Contrast note: these are used as icon tint on `--card`, never as a
// background behind text, so they are exempt from the AA floor the
// status tones must clear.

export const BRAND_COLORS = {
  /** Discord — brand guidelines, "Blurple". */
  discord: '#5865F2',
  /** Microsoft Teams — brand guidelines, "Teams Purple". */
  teams: '#6264A7',
  /** Telegram — brand assets, "Telegram Blue". */
  telegram: '#26A5E4',
  /** Signal — brand assets. */
  signal: '#3A76F0',
  /** WhatsApp — brand guidelines, "WhatsApp Green". */
  whatsapp: '#25D366',
  /** Microsoft — Entra ID / Azure AD identity provider. */
  microsoft: '#0078D4',
  /** Google — Workspace / Google identity provider. */
  google: '#4285F4',
} as const;

export type BrandKey = keyof typeof BRAND_COLORS;

/** Tailwind arbitrary text-colour class for a brand mark. */
export function brandText(key: BrandKey): string {
  return `text-[${BRAND_COLORS[key]}]`;
}
