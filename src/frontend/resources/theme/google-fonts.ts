// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { GOOGLE_FONT_SPECS, type ThemeFont } from '@resources/stores/themeCustomizer';

/**
 * On-demand loader for the Google Fonts entries in the appearance picker.
 *
 * The three bundled faces are declared in `public/fonts/fonts.css` and ship in
 * the image. These do not: they are fetched from Google's CDN the first time
 * someone actually selects or previews one, which is the only reason the app
 * ever reaches the network for a font.
 *
 * Two properties matter more than speed here:
 *
 *   * **Offline is normal, not exceptional.** McHarbor is frequently deployed
 *     air-gapped. Nothing is requested while the browser reports itself
 *     offline; the family is queued and an `online` listener flushes it later.
 *     Meanwhile every Google stack in `FONT_STACKS` ends in a bundled face, so
 *     the UI renders Inter Tight / Georgia / JetBrains Mono rather than the
 *     browser's default.
 *   * **A failed request must not poison the retry.** `navigator.onLine`
 *     reporting true says the machine has *a* network, not that it can reach
 *     fonts.gstatic.com — a captive portal or an egress firewall both look
 *     online. A `<link>` that errors removes itself and re-queues, so coming
 *     back later works instead of finding a dead tag and skipping.
 */

const LINK_PREFIX = 'mc-google-font-';
const PRECONNECT_ID = 'mc-google-font-preconnect';

/** Families asked for while offline (or after a failed load), awaiting a retry. */
const queued = new Set<ThemeFont>();
let onlineListenerAttached = false;

function linkID(font: ThemeFont): string {
  return `${LINK_PREFIX}${font}`;
}

/**
 * Warm the connection to the font host once. The stylesheet comes from
 * fonts.googleapis.com but the woff2 files come from fonts.gstatic.com, and
 * that second origin is only discovered after the CSS parses — which is
 * exactly the round trip a preconnect removes.
 */
function preconnect(): void {
  if (document.getElementById(PRECONNECT_ID)) return;
  const link = document.createElement('link');
  link.id = PRECONNECT_ID;
  link.rel = 'preconnect';
  link.href = 'https://fonts.gstatic.com';
  link.crossOrigin = 'anonymous';
  document.head.appendChild(link);
}

function inject(font: ThemeFont): void {
  const spec = GOOGLE_FONT_SPECS[font];
  if (!spec) return;
  if (document.getElementById(linkID(font))) return;

  preconnect();

  const link = document.createElement('link');
  link.id = linkID(font);
  link.rel = 'stylesheet';
  // `display=swap` so text paints in the fallback immediately and reflows when
  // the face lands, rather than sitting invisible behind a slow request.
  link.href = `https://fonts.googleapis.com/css2?family=${spec}&display=swap`;
  link.addEventListener('error', () => {
    link.remove();
    queued.add(font);
    attachOnlineListener();
  });
  document.head.appendChild(link);
  queued.delete(font);
}

function flushQueue(): void {
  // oxlint-disable-next-line unicorn/no-useless-spread -- inject() deletes from queued, so this iterates a snapshot
  for (const font of [...queued]) inject(font);
}

function attachOnlineListener(): void {
  if (onlineListenerAttached || typeof window === 'undefined') return;
  onlineListenerAttached = true;
  // Deliberately never removed: it is one listener for the life of the
  // document, and the queue it drains can be refilled at any point by a user
  // picking a font while the connection is down.
  window.addEventListener('online', flushQueue);
}

/**
 * Make sure `font` is loadable, if it is one of the Google families and the
 * browser looks online. A no-op for the bundled and system stacks, and a no-op
 * for a family whose `<link>` is already in the document.
 */
export function ensureGoogleFont(font: ThemeFont): void {
  if (typeof document === 'undefined') return;
  if (!(font in GOOGLE_FONT_SPECS)) return;

  // `false` is a reliable "definitely offline". `true` is not a promise of
  // reachability, which is what the error handler above is for.
  if (typeof navigator !== 'undefined' && navigator.onLine === false) {
    queued.add(font);
    attachOnlineListener();
    return;
  }

  inject(font);
}
