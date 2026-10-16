// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect, useRef } from 'react';
import { Alert } from '@resources/components/ui/Alert';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { FIELDS, FLAVOURS, readField, validateField, type FieldDef, type Translate } from './fields';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t: Translate;
  mode?: 'create' | 'edit';
};

type ControlProps = {
  f: FieldDef;
  id: string;
  current: string;
  isEdit: boolean;
  t: Translate;
  set: (key: string, next: string) => void;
};

function FieldControl({ f, id, current, isEdit, t, set }: ControlProps) {
  if (f.kind === 'flavour') {
    return (
      <Select
        ariaLabel={t('fields.flavour.label')}
        value={current}
        onChange={(v: string) => set('flavour', v)}
        searchable={false}
        options={FLAVOURS.map((fl) => ({ value: fl, label: t(`flavour.${fl}`), description: t(`flavour.${fl}Help`) }))}
      />
    );
  }
  if (f.kind === 'bool') {
    return (
      <Switch
        id={id}
        checked={current === 'true'}
        onCheckedChange={(on: boolean) => set(f.key, on ? 'true' : 'false')}
      />
    );
  }
  const placeholder =
    f.kind === 'secret' && isEdit ? t('secretKeepHint') : f.placeholder ? t(`fields.${f.key}.placeholder`) : undefined;
  if (f.kind === 'number') {
    return (
      <NumberInput
        id={id}
        nullable
        value={current.trim() === '' ? null : Number(current)}
        placeholder={placeholder}
        onChange={(next) => set(f.key, next === null ? '' : String(next))}
      />
    );
  }
  return (
    <Input
      id={id}
      type={f.kind === 'secret' ? 'password' : 'text'}
      value={current}
      placeholder={placeholder}
      autoComplete={f.kind === 'secret' ? 'new-password' : 'off'}
      spellCheck={false}
      onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
    />
  );
}

export function ConfigForm({ value, onChange, onError, t, mode }: Props) {
  const isEdit = mode === 'edit';
  const seeded = useRef(false);

  useEffect(() => {
    if (seeded.current || isEdit) return;
    seeded.current = true;
    const missing = FIELDS.filter((f) => f.defaultValue !== undefined && value[f.key] === undefined);
    if (missing.length === 0) return;
    const next = { ...value };
    for (const f of missing) next[f.key] = f.defaultValue ?? '';
    onChange(next);
  }, [isEdit, value, onChange]);

  useEffect(() => {
    for (const f of FIELDS) {
      onError?.(f.key, validateField(f, readField(value, f), isEdit, t));
    }
  }, [value, isEdit, onError, t]);

  const set = (key: string, next: string) => onChange({ ...value, [key]: next });

  return (
    <Stack gap={4}>
      <p className="text-sm text-muted-foreground">{t('description')}</p>
      <Grid columns={2} gap={4}>
        {FIELDS.map((f) => {
          const id = `comm-otrs-${f.key}`;
          return (
            <Field
              key={f.key}
              id={id}
              label={t(`fields.${f.key}.label`)}
              required={f.required && !(f.kind === 'secret' && isEdit)}
              description={t(`fields.${f.key}.help`)}
            >
              <FieldControl f={f} id={id} current={readField(value, f)} isEdit={isEdit} t={t} set={set} />
            </Field>
          );
        })}
      </Grid>
      {(value.flavour ?? 'znuny') === 'otrs' && <Alert variant="warning">{t('otrs8Notice')}</Alert>}
    </Stack>
  );
}
