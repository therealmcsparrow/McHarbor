// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent, ReactNode } from 'react';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

export type SetField = (key: string, value: string) => void;

export const SECRET_KEYS = ['auth_token', 'auth_pwd'] as const;

const fieldId = (fieldName: string) => `comm-itop-${fieldName}`;

type TextFieldProps = {
  name: string;
  label: string;
  value: string;
  kind?: 'text' | 'secret' | 'number' | 'id';
  required?: boolean;
  description?: ReactNode;
  placeholder?: string;
  error?: string | null;
  onChange: SetField;
};

export function TextField({
  name: fieldName,
  label,
  value,
  kind = 'text',
  required,
  description,
  placeholder,
  error,
  onChange,
}: TextFieldProps) {
  const id = fieldId(fieldName);
  if (kind === 'number') {
    return (
      <Field id={id} label={label} required={required} description={description} error={error ?? undefined}>
        <NumberInput
          id={id}
          nullable
          value={value.trim() === '' ? null : Number(value)}
          placeholder={placeholder}
          onChange={(next) => onChange(fieldName, next === null ? '' : String(next))}
        />
      </Field>
    );
  }
  return (
    <Field id={id} label={label} required={required} description={description} error={error ?? undefined}>
      <Input
        id={id}
        type={kind === 'secret' ? 'password' : kind === 'id' ? 'number' : 'text'}
        value={value}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(fieldName, e.target.value)}
      />
    </Field>
  );
}

type SwitchFieldProps = {
  name: string;
  label: string;
  checked: boolean;
  onChange: SetField;
};

export function SwitchField({ name: fieldName, label, checked, onChange }: SwitchFieldProps) {
  const id = fieldId(fieldName);
  return (
    <Field id={id} label={label}>
      <Switch id={id} checked={checked} onCheckedChange={(on) => onChange(fieldName, on ? 'true' : 'false')} />
    </Field>
  );
}

type ChoiceFieldProps = {
  name: string;
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: SetField;
};

export function ChoiceField({ name: fieldName, label, value, options, onChange }: ChoiceFieldProps) {
  return (
    <Field id={fieldId(fieldName)} label={label}>
      <Select
        value={value}
        options={options}
        searchable={false}
        ariaLabel={label}
        onChange={(v) => onChange(fieldName, v)}
      />
    </Field>
  );
}
