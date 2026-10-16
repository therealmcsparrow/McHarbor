// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export type Mode = 'create' | 'edit';

const REQUIRED = ['name', 'instance_url', 'queue'] as const;
const NAME_FIELDS = ['queue', 'dedup_cf_name', 'resolve_status'] as const;
const JSON_FIELDS = ['status_map', 'severity_map'] as const;
const RANGES: Record<string, [number, number]> = {
  reopen_window_minutes: [0, 1440],
  poll_interval_minutes: [0, 60],
};
const WATCHED = [
  'name',
  'instance_url',
  'auth_token',
  'queue',
  'dedup_cf_name',
  'resolve_status',
  'requestor',
  ...JSON_FIELDS,
  ...Object.keys(RANGES),
];

const SAFE_NAME = /^[\p{L}\p{N} _.-]{1,200}$/u;
const EMAIL = /^[^\s@,;<>"']+@[^\s@,;<>"']+\.[^\s@,;<>"']+$/;

function isJsonObject(raw: string): boolean {
  try {
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
  } catch {
    return false;
  }
}

export function validateRt(value: Record<string, string>, mode: Mode, t: Translate): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const key of NAME_FIELDS) {
    const raw = (value[key] ?? '').trim();
    if (raw && !SAFE_NAME.test(raw)) errors[key] = t('form.badName');
  }
  for (const key of REQUIRED) {
    if (!(value[key] ?? '').trim()) errors[key] = t('form.required');
  }
  if (mode === 'create' && !(value.auth_token ?? '').trim()) errors.auth_token = t('form.required');
  const url = (value.instance_url ?? '').trim();
  if (url && !url.toLowerCase().startsWith('https://')) errors.instance_url = t('form.httpsOnly');
  const requestor = (value.requestor ?? '').trim();
  if (requestor && !EMAIL.test(requestor)) errors.requestor = t('form.badEmail');
  for (const key of JSON_FIELDS) {
    const raw = (value[key] ?? '').trim();
    if (raw && !isJsonObject(raw)) errors[key] = t('form.badJson');
  }
  for (const [key, [min, max]] of Object.entries(RANGES)) {
    const raw = (value[key] ?? '').trim();
    const n = Number(raw);
    if (raw && (!Number.isInteger(n) || n < min || n > max)) errors[key] = t('form.range', { min, max });
  }
  return errors;
}

export function useRtValidation(
  value: Record<string, string>,
  mode: Mode,
  t: Translate,
  onError?: (field: string, msg: string | null) => void,
): Record<string, string> {
  const errors = validateRt(value, mode, t);
  const signature = JSON.stringify(errors);
  const report = useRef(onError);
  useEffect(() => {
    report.current = onError;
  });
  useEffect(() => {
    const current: Record<string, string> = JSON.parse(signature);
    for (const key of WATCHED) report.current?.(key, current[key] ?? null);
  }, [signature]);
  return errors;
}
