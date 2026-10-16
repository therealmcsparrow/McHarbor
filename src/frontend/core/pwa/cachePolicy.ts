// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const CACHED_AT_HEADER = 'X-Mcharbor-Cached-At';
export const READ_CACHE = 'mcharbor-read-v1';
export const SHELL_CACHE_PREFIX = 'mcharbor-shell-';
export const CLEAR_READ_CACHE = 'mcharbor:clear-read-cache';

const READ_PATHS: RegExp[] = [
  /^\/api\/auth\/status$/,
  /^\/api\/auth\/session$/,
  /^\/api\/auth\/me\/permissions$/,
  /^\/api\/environments$/,
  /^\/api\/containers$/,
  /^\/api\/containers\/[A-Za-z0-9_.-]+$/,
  /^\/api\/alerts\/events$/,
  /^\/api\/in-app-notifications$/,
  /^\/api\/elevations\/pending$/,
  /^\/api\/push\/subscriptions$/,
  /^\/api\/runbooks\/for$/,
];

const NEVER_CACHED: RegExp[] = [/^\/api\/logs[/]/, /\/stream$/, /\/events\/stream/, /^\/api\/ws/, /\/terminal/];

export function isReadCacheable(method: string, url: URL, originValue: string): boolean {
  if (method !== 'GET' || url.origin !== originValue) return false;
  if (NEVER_CACHED.some((re) => re.test(url.pathname))) return false;
  return READ_PATHS.some((re) => re.test(url.pathname));
}

export function isStoreableResponse(res: Response): boolean {
  if (res.status !== 200 || res.type === 'opaque') return false;
  const type = res.headers.get('Content-Type') ?? '';
  return type.includes('application/json');
}

export function isShellAsset(url: URL, originValue: string): boolean {
  return url.origin === originValue && (url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/'));
}

export function isSessionEnd(res: Response): boolean {
  return res.status === 401;
}
