// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export type FieldProps = {
  value: Record<string, string>;
  set: (key: string, next: string) => void;
  errors: Record<string, string | null>;
  t: Translate;
  isEdit: boolean;
};

type TextFieldProps = {
  name: string;
  label: string;
  kind?: 'text' | 'secret' | 'int' | 'id';
  help?: string;
  placeholder?: string;
  required?: boolean;
  fallback?: string;
  ctx: FieldProps;
};

export function TextField({
  name: fieldName,
  label,
  kind = 'text',
  help,
  placeholder,
  required,
  fallback = '',
  ctx,
}: TextFieldProps) {
  const id = `comm-glpi-${fieldName}`;
  const secretHint = kind === 'secret' && ctx.isEdit ? ctx.t('form.secretKeep') : undefined;
  const current = ctx.value[fieldName] ?? fallback;
  if (kind === 'int') {
    return (
      <Field id={id} label={label} description={help} error={ctx.errors[fieldName] ?? undefined} required={required}>
        <NumberInput
          id={id}
          nullable
          min={0}
          value={current.trim() === '' ? null : Number(current)}
          placeholder={placeholder}
          onChange={(next) => ctx.set(fieldName, next === null ? '' : String(next))}
        />
      </Field>
    );
  }
  return (
    <Field id={id} label={label} description={help} error={ctx.errors[fieldName] ?? undefined} required={required}>
      <Input
        id={id}
        type={kind === 'secret' ? 'password' : kind === 'id' ? 'number' : 'text'}
        min={kind === 'id' ? 0 : undefined}
        value={ctx.value[fieldName] ?? fallback}
        placeholder={secretHint ?? placeholder}
        autoComplete="off"
        spellCheck={false}
        onChange={(e: ChangeEvent<HTMLInputElement>) => ctx.set(fieldName, e.target.value)}
      />
    </Field>
  );
}

type ToggleFieldProps = {
  name: string;
  label: string;
  help?: string;
  fallback: boolean;
  ctx: FieldProps;
};

export function ToggleField({ name: fieldName, label, help, fallback, ctx }: ToggleFieldProps) {
  const id = `comm-glpi-${fieldName}`;
  const raw = ctx.value[fieldName];
  const checked = raw === undefined || raw === '' ? fallback : raw === 'true';
  return (
    <Field id={id} label={label} description={help}>
      <Switch id={id} checked={checked} onCheckedChange={(on: boolean) => ctx.set(fieldName, on ? 'true' : 'false')} />
    </Field>
  );
}
