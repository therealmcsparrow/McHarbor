// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import type { Translate } from './fields';

const V2_PATH = /^v2\.[0-9]{1,2}$/;

const INT_RANGES: Record<string, [number, number]> = {
  entities_id: [0, Number.MAX_SAFE_INTEGER],
  itilcategories_id: [0, Number.MAX_SAFE_INTEGER],
  default_assignment_group: [0, Number.MAX_SAFE_INTEGER],
  reopen_window_minutes: [0, 1440],
  poll_interval_minutes: [0, 60],
};

function isHttpsUrl(raw: string): boolean {
  try {
    const u = new URL(raw);
    return u.protocol === 'https:' && u.host !== '' && u.username === '' && u.search === '' && u.hash === '';
  } catch {
    return false;
  }
}

function isJsonObject(raw: string): boolean {
  try {
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
  } catch {
    return false;
  }
}

export function validateGlpi(
  value: Record<string, string>,
  isEdit: boolean,
  t: Translate,
): Record<string, string | null> {
  const v = (key: string) => (value[key] ?? '').trim();
  const api = v('api_version') || 'v1';
  const required = (key: string, needed: boolean) => (needed && v(key) === '' ? t('form.required') : null);
  const errors: Record<string, string | null> = {
    name: required('name', true),
    instance_url:
      v('instance_url') === '' ? t('form.required') : isHttpsUrl(v('instance_url')) ? null : t('form.httpsOnly'),
    user_token: required('user_token', api === 'v1' && !isEdit),
    oauth_client_id: required('oauth_client_id', api === 'v2'),
    oauth_client_secret: required('oauth_client_secret', api === 'v2' && !isEdit),
    oauth_username: required('oauth_username', api === 'v2'),
    oauth_password: required('oauth_password', api === 'v2' && !isEdit),
    v2_path: api === 'v2' && v('v2_path') !== '' && !V2_PATH.test(v('v2_path')) ? t('form.v2PathInvalid') : null,
    severity_map: v('severity_map') !== '' && !isJsonObject(v('severity_map')) ? t('form.badJson') : null,
  };
  for (const [key, [min, max]] of Object.entries(INT_RANGES)) {
    const raw = v(key);
    const n = Number(raw);
    errors[key] =
      raw !== '' && (!Number.isInteger(n) || n < min || n > max) ? t('form.outOfRange', { min, max }) : null;
  }
  return errors;
}

export function useGlpiValidation(
  value: Record<string, string>,
  isEdit: boolean,
  t: Translate,
  onError?: (field: string, msg: string | null) => void,
): Record<string, string | null> {
  const errors = useMemo(() => validateGlpi(value, isEdit, t), [value, isEdit, t]);
  useEffect(() => {
    for (const [field, msg] of Object.entries(errors)) {
      onError?.(field, msg);
    }
  }, [errors, onError]);
  return errors;
}
