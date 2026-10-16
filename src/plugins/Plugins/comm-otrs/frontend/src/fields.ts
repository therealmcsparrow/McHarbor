// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type FieldKind = 'text' | 'secret' | 'number' | 'flavour' | 'bool';

export type FieldDef = {
  key: string;
  kind: FieldKind;
  required: boolean;
  placeholder?: boolean;
  defaultValue?: string;
  min?: number;
  max?: number;
};

export const FIELDS: FieldDef[] = [
  { key: 'name', kind: 'text', required: true, placeholder: true },
  { key: 'flavour', kind: 'flavour', required: true, defaultValue: 'znuny' },
  { key: 'instance_url', kind: 'text', required: true, placeholder: true },
  { key: 'webservice', kind: 'text', required: true, placeholder: true, defaultValue: 'McHarborConnectorREST' },
  { key: 'user_login', kind: 'text', required: true, placeholder: true },
  { key: 'password', kind: 'secret', required: true },
  { key: 'default_assignment_group', kind: 'text', required: true, placeholder: true },
  { key: 'customer_user', kind: 'text', required: true, placeholder: true },
  { key: 'dedup_field', kind: 'text', required: true, placeholder: true, defaultValue: 'McHarborDedupKey' },
  { key: 'ticket_type', kind: 'text', required: false, placeholder: true },
  { key: 'close_state', kind: 'text', required: false, placeholder: true },
  { key: 'resolved_states', kind: 'text', required: false },
  { key: 'cancelled_states', kind: 'text', required: false },
  { key: 'script_path', kind: 'text', required: false, placeholder: true },
  {
    key: 'poll_interval_minutes',
    kind: 'number',
    required: false,
    placeholder: true,
    defaultValue: '5',
    min: 0,
    max: 60,
  },
  {
    key: 'reopen_window_minutes',
    kind: 'number',
    required: false,
    placeholder: true,
    defaultValue: '0',
    min: 0,
    max: 1440,
  },
  { key: 'update_on_repeat', kind: 'bool', required: false, defaultValue: 'true' },
  { key: 'severity_map', kind: 'text', required: false, placeholder: true },
];

export const FLAVOURS = ['znuny', 'otobo', 'otrs'] as const;

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export function readField(value: Record<string, string>, f: FieldDef): string {
  return value[f.key] ?? f.defaultValue ?? '';
}

function isJsonObject(raw: string): boolean {
  try {
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
  } catch {
    return false;
  }
}

export function validateField(f: FieldDef, raw: string, isEdit: boolean, t: Translate): string | null {
  const v = raw.trim();
  if (!v) {
    const optionalSecret = f.kind === 'secret' && isEdit;
    return f.required && !optionalSecret ? t('validation.required') : null;
  }
  if (f.key === 'instance_url' && !v.startsWith('https://')) return t('validation.https');
  if (f.kind === 'number') {
    const n = Number(v);
    if (!Number.isInteger(n) || (f.min !== undefined && n < f.min) || (f.max !== undefined && n > f.max)) {
      return t('validation.range', { min: f.min, max: f.max });
    }
  }
  if (f.key === 'dedup_field' && !/^[A-Za-z0-9]+$/.test(v)) return t('validation.dedupField');
  if (f.key === 'severity_map' && !isJsonObject(v)) return t('validation.json');
  return null;
}
