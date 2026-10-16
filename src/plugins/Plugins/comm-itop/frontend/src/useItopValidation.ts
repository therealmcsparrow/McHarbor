// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef } from 'react';

export type ItopErrors = Record<string, string | null>;

type IntRule = { key: string; min: number; max: number; required: boolean };

const INT_RULES: IntRule[] = [
  { key: 'team_id', min: 1, max: 1073741824, required: true },
  { key: 'agent_id', min: 1, max: 1073741824, required: true },
  { key: 'service_id', min: 1, max: 1073741824, required: true },
  { key: 'default_assignment_group', min: 0, max: 1073741824, required: false },
  { key: 'reopen_window_minutes', min: 0, max: 1440, required: false },
  { key: 'poll_interval_minutes', min: 0, max: 60, required: false },
];

const REQUIRED_TEXT = ['name', 'instance_url', 'org_name'];

function intError(raw: string, rule: IntRule): string | null {
  const v = raw.trim();
  if (v === '') return rule.required ? 'form.required' : null;
  if (!/^\d+$/.test(v)) return 'form.wholeNumber';
  const n = Number(v);
  return n < rule.min || n > rule.max ? 'form.wholeNumber' : null;
}

function jsonError(raw: string): string | null {
  const v = raw.trim();
  if (v === '') return null;
  try {
    const parsed: unknown = JSON.parse(v);
    return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) ? null : 'form.badJson';
  } catch {
    return 'form.badJson';
  }
}

export function validateItop(value: Record<string, string>): ItopErrors {
  const errors: ItopErrors = {};
  for (const key of REQUIRED_TEXT) {
    errors[key] = (value[key] ?? '').trim() === '' ? 'form.required' : null;
  }
  const url = (value.instance_url ?? '').trim();
  if (url !== '' && !/^https:\/\/[^\s/?#@]+/i.test(url)) errors.instance_url = 'form.httpsOnly';
  for (const rule of INT_RULES) errors[rule.key] = intError(value[rule.key] ?? '', rule);
  errors.severity_map = jsonError(value.severity_map ?? '');
  return errors;
}

export function useItopValidation(
  value: Record<string, string>,
  onError: ((field: string, msg: string | null) => void) | undefined,
  t: (key: string) => string,
): ItopErrors {
  const errors = useMemo(() => validateItop(value), [value]);
  const messages = useMemo(() => {
    const out: ItopErrors = {};
    for (const [key, code] of Object.entries(errors)) out[key] = code ? t(code) : null;
    return out;
  }, [errors, t]);
  const report = useRef(onError);
  const last = useRef<ItopErrors>({});
  useEffect(() => {
    report.current = onError;
  });
  useEffect(() => {
    for (const [key, msg] of Object.entries(messages)) {
      if (last.current[key] !== msg) report.current?.(key, msg);
    }
    last.current = messages;
  }, [messages]);
  return messages;
}
