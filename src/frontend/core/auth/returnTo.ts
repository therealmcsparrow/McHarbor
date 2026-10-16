// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const AUTH_PAGES = ['/login', '/setup', '/force-password-change', '/reset-password'];

export function safeReturnTo(value: string | null | undefined): string | null {
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) return null;
  let parsed: URL;
  try {
    parsed = new URL(value, 'https://mcharbor.invalid');
  } catch {
    return null;
  }
  if (parsed.origin !== 'https://mcharbor.invalid') return null;
  if (AUTH_PAGES.some((page) => parsed.pathname === page || parsed.pathname.startsWith(`${page}/`))) return null;
  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
}

export function loginPathFor(loc: { pathname: string; search?: string; hash?: string }): string {
  const next = safeReturnTo(`${loc.pathname}${loc.search ?? ''}${loc.hash ?? ''}`);
  if (!next || next === '/' || next === '/dashboard') return '/login';
  return `/login?next=${encodeURIComponent(next)}`;
}
