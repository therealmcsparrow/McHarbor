// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export type Mode = 'create' | 'edit';

const REQUIRED = ['name', 'instance_url', 'group', 'customer'] as const;
const JSON_FIELDS = ['status_map', 'severity_map'] as const;
const WATCHED = [...REQUIRED, 'access_token', ...JSON_FIELDS] as const;

function isJsonObject(raw: string): boolean {
  try {
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
  } catch {
    return false;
  }
}

export function validateZammad(value: Record<string, string>, mode: Mode, t: Translate): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const key of REQUIRED) {
    if (!(value[key] ?? '').trim()) errors[key] = t('form.required');
  }
  if (mode === 'create' && !(value.access_token ?? '').trim()) errors.access_token = t('form.required');
  const url = (value.instance_url ?? '').trim();
  if (url && !url.toLowerCase().startsWith('https://')) errors.instance_url = t('form.httpsOnly');
  for (const key of JSON_FIELDS) {
    const raw = (value[key] ?? '').trim();
    if (raw && !isJsonObject(raw)) errors[key] = t('form.badJson');
  }
  return errors;
}

export function useZammadValidation(
  value: Record<string, string>,
  mode: Mode,
  t: Translate,
  onError?: (field: string, msg: string | null) => void,
): Record<string, string> {
  const errors = validateZammad(value, mode, t);
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
