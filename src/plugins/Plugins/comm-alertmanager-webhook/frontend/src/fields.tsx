// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import type { Translate } from './options';

type TextFieldProps = {
  name: string;
  value: string;
  secret?: boolean;
  required?: boolean;
  help?: boolean;
  error?: string | null;
  t: Translate;
  onChange: (name: string, value: string) => void;
};

const fieldId = (fieldName: string) => `alertmanager-webhook-${fieldName}`;

export function TextField({ name: fieldName, value, secret, required, help, error, t, onChange }: TextFieldProps) {
  const id = fieldId(fieldName);
  const parts: string[] = [];
  if (help) parts.push(t(`form.${fieldName}.help`));
  if (secret) parts.push(t('form.secretKeepHint'));
  return (
    <Field
      id={id}
      label={t(`form.${fieldName}.label`)}
      required={required}
      description={parts.length > 0 ? parts.join(' ') : undefined}
      error={error ?? undefined}
    >
      <Input
        id={id}
        type={secret ? 'password' : 'text'}
        value={value}
        placeholder={t(`form.${fieldName}.placeholder`)}
        autoComplete="off"
        spellCheck={false}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(fieldName, e.target.value)}
      />
    </Field>
  );
}

type SwitchFieldProps = {
  name: string;
  checked: boolean;
  help?: boolean;
  t: Translate;
  onChange: (name: string, value: string) => void;
};

export function SwitchField({ name: fieldName, checked, help, t, onChange }: SwitchFieldProps) {
  const id = fieldId(fieldName);
  return (
    <Field id={id} label={t(`form.${fieldName}.label`)} description={help ? t(`form.${fieldName}.help`) : undefined}>
      <Switch id={id} checked={checked} onCheckedChange={(on) => onChange(fieldName, on ? 'true' : 'false')} />
    </Field>
  );
}
