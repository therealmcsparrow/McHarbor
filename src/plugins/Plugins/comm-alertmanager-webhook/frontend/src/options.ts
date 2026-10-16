// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export const PROFILES = ['generic', 'grafana_irm', 'squadcast', 'zenduty', 'keep'] as const;
export type Profile = (typeof PROFILES)[number];

export const AUTH_MODES = ['none', 'bearer', 'basic'] as const;

export const SECRET_KEYS = ['webhook_url', 'bearer_token', 'basic_password', 'hmac_secret'] as const;

export function profileOf(value: Record<string, string>): Profile {
  const p = value.profile ?? '';
  return (PROFILES as readonly string[]).includes(p) ? (p as Profile) : 'generic';
}

export function authModeOf(value: Record<string, string>): string {
  if (value.auth_mode) return value.auth_mode;
  return profileOf(value) === 'keep' ? 'basic' : 'none';
}

export function urlError(value: Record<string, string>): string | null {
  const raw = (value.webhook_url ?? '').trim();
  if (!raw) return null;
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return 'form.errors.https';
  }
  if (parsed.protocol === 'http:' && value.allow_http !== 'true') return 'form.errors.https';
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return 'form.errors.https';
  if (profileOf(value) === 'grafana_irm' && !parsed.pathname.endsWith('/')) return 'form.errors.trailingSlash';
  return null;
}
