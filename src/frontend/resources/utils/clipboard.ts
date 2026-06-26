// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Copies text to the clipboard with a manual-copy fallback for non-secure
 * contexts.
 *
 * The async Clipboard API (`navigator.clipboard.writeText`) is only reliably
 * available in secure contexts (HTTPS or localhost). When McHarbor is served
 * over plain HTTP on a non-loopback address — e.g. a self-hosted LAN install
 * behind a reverse proxy that terminates TLS, or a homelab IP — that API
 * either throws `NotAllowedError` or (more commonly) resolves silently
 * without writing anything to the clipboard. The UI would report success
 * while the clipboard stays empty.
 *
 * The legacy `document.execCommand('copy')` path is equally unreliable in
 * modern browsers on non-secure contexts: it can return `true` without
 * actually copying. Trusting either API on HTTP non-loopback produces the
 * silent-failure the operator reported.
 *
 * This helper therefore treats any non-secure context as `manual-required`
 * up front: it skips both clipboard APIs entirely and asks the caller to
 * show the ManualCopyDialog so the operator can press Ctrl/Cmd+C
 * themselves. On secure contexts it uses the async API as normal.
 */

export type CopyResult = {
  ok: boolean;
  reason: 'clipboard-api' | 'manual-required' | 'unsupported';
};

/**
 * Returns the best copy result for the current environment.
 *
 * - On secure contexts (HTTPS or localhost): uses `navigator.clipboard.writeText`
 *   and reports whether it succeeded.
 * - On non-secure contexts (HTTP non-loopback): reports `manual-required`
 *   without trying any clipboard API, because both `navigator.clipboard` and
 *   `document.execCommand('copy')` are unreliable there.
 */
export async function copyToClipboard(text: string): Promise<CopyResult> {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return { ok: false, reason: 'unsupported' };
  }

  if (!isSecureClipboardContext()) {
    return { ok: false, reason: 'manual-required' };
  }

  if (!navigator.clipboard?.writeText) {
    return { ok: false, reason: 'unsupported' };
  }

  try {
    await navigator.clipboard.writeText(text);
    return { ok: true, reason: 'clipboard-api' };
  } catch {
    return { ok: false, reason: 'manual-required' };
  }
}

/**
 * Returns `true` if the page is currently served from `localhost` (any
 * loopback name: `localhost`, `127.0.0.0/8`, `[::1]`, or `*.localhost`).
 *
 * Used to tell apart two cases that `window.isSecureContext` does not
 * distinguish on its own:
 *
 * - `window.isSecureContext === true` on HTTPS *or* on plain-HTTP localhost.
 *   Localhost is treated as secure even though the connection is not TLS, so
 *   browsers allow `navigator.clipboard.writeText` there.
 * - `window.isSecureContext === false` on plain HTTP to a non-loopback
 *   hostname or IP. Browsers refuse the async clipboard API and may also
 *   silently no-op `document.execCommand('copy')`, which is why copy
 *   "succeeds" but the clipboard stays empty.
 */
export function isLocalhost(): boolean {
  if (typeof window === 'undefined') return false;
  const host = window.location.hostname.toLowerCase();
  if (host === 'localhost' || host === '127.0.0.1' || host === '[::1]' || host === '::1') {
    return true;
  }
  if (host.endsWith('.localhost')) {
    return true;
  }
  // Loopback range 127.0.0.0/8.
  if (/^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) {
    return true;
  }
  return false;
}

/**
 * Returns `true` if the async Clipboard API is guaranteed to work, i.e. the
 * page is either on HTTPS or served from a loopback host. Plain HTTP on a
 * non-loopback address returns `false` even if `window.isSecureContext` is
 * `true` for some other reason.
 */
export function isSecureClipboardContext(): boolean {
  if (typeof window === 'undefined') return false;
  if (window.isSecureContext !== true) return false;
  // Localhost is always treated as secure; non-localhost needs HTTPS.
  if (isLocalhost()) return true;
  return window.location.protocol === 'https:' || window.location.protocol === 'wss:';
}