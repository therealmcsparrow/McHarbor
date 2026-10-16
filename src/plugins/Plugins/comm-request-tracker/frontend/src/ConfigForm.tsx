// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { Alert } from '@resources/components/ui/Alert';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Heading } from '@resources/components/ui/Typography';
import { useRtValidation, type Mode, type Translate } from './useRtValidation';

export type { Translate };

type FieldKind = 'string' | 'secret' | 'int' | 'bool';

type FieldDecl = {
  key: string;
  label: string;
  kind: FieldKind;
  required?: boolean;
  help?: string;
  defaultValue?: string;
  placeholder?: string;
  min?: number;
  max?: number;
  wide?: boolean;
};

export const FIELDS: FieldDecl[] = [
  { key: 'name', label: 'form.name', kind: 'string', required: true },
  {
    key: 'instance_url',
    label: 'form.instanceUrl',
    kind: 'string',
    required: true,
    help: 'form.instanceUrlHelp',
    placeholder: 'https://rt.example.edu/rt',
  },
  { key: 'auth_token', label: 'form.authToken', kind: 'secret', required: true, help: 'form.authTokenHelp' },
  {
    key: 'queue',
    label: 'form.queue',
    kind: 'string',
    required: true,
    help: 'form.queueHelp',
    placeholder: 'Operations',
  },
  {
    key: 'dedup_cf_name',
    label: 'form.dedupCf',
    kind: 'string',
    help: 'form.dedupCfHelp',
    defaultValue: 'McHarbor Dedup Key',
  },
  {
    key: 'resolve_status',
    label: 'form.resolveStatus',
    kind: 'string',
    help: 'form.resolveStatusHelp',
    defaultValue: 'resolved',
  },
  { key: 'requestor', label: 'form.requestor', kind: 'string', help: 'form.requestorHelp' },
  { key: 'default_assignment_group', label: 'form.owner', kind: 'string', help: 'form.ownerHelp' },
  {
    key: 'reopen_window_minutes',
    label: 'form.reopenWindow',
    kind: 'int',
    help: 'form.reopenWindowHelp',
    defaultValue: '0',
    min: 0,
    max: 1440,
  },
  {
    key: 'poll_interval_minutes',
    label: 'form.pollInterval',
    kind: 'int',
    help: 'form.pollIntervalHelp',
    defaultValue: '5',
    min: 0,
    max: 60,
  },
  {
    key: 'update_on_repeat',
    label: 'form.updateOnRepeat',
    kind: 'bool',
    help: 'form.updateOnRepeatHelp',
    defaultValue: 'true',
  },
  {
    key: 'severity_map',
    label: 'form.severityMap',
    kind: 'string',
    help: 'form.severityMapHelp',
    placeholder: '{"critical":90,"warning":50,"info":20}',
    wide: true,
  },
  {
    key: 'status_map',
    label: 'form.statusMap',
    kind: 'string',
    help: 'form.statusMapHelp',
    placeholder: '{"waiting":"acknowledged"}',
    wide: true,
  },
];

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  mode?: Mode;
  t: Translate;
};

export function ConfigForm({ value, onChange, onError, mode = 'create', t }: Props) {
  const errors = useRtValidation(value, mode, t, onError);
  const read = (f: FieldDecl) => value[f.key] ?? f.defaultValue ?? '';
  const set = (key: string, v: string) => onChange({ ...value, [key]: v });

  return (
    <Stack gap={4}>
      <Stack gap={2}>
        <Heading as="h3" size="base" weight="semibold">
          {t('form.title')}
        </Heading>
        <Alert>{t('form.intro')}</Alert>
      </Stack>
      <Grid columns="grid-cols-1 sm:grid-cols-2" gap={4}>
        {FIELDS.map((f) => {
          const id = `comm-request-tracker-${f.key}`;
          const label = t(f.label);
          const secretKeep = f.kind === 'secret' && mode === 'edit';
          const description = secretKeep ? t('form.secretKeep') : f.help ? t(f.help) : undefined;
          if (f.kind === 'bool') {
            return (
              <Field key={f.key} id={id} label={label} description={description}>
                <Switch
                  id={id}
                  checked={read(f) === 'true'}
                  onCheckedChange={(on) => set(f.key, on ? 'true' : 'false')}
                />
              </Field>
            );
          }
          return (
            <div key={f.key} className={f.wide ? 'sm:col-span-2' : undefined}>
              <Field
                id={id}
                label={label}
                required={f.required && !secretKeep}
                description={description}
                error={errors[f.key]}
              >
                {f.kind === 'int' ? (
                  <NumberInput
                    id={id}
                    nullable
                    value={read(f).trim() === '' ? null : Number(read(f))}
                    placeholder={f.placeholder}
                    min={f.min}
                    max={f.max}
                    allowOutOfRange
                    onChange={(next) => set(f.key, next === null ? '' : String(next))}
                  />
                ) : (
                  <Input
                    id={id}
                    type={f.kind === 'secret' ? 'password' : 'text'}
                    value={read(f)}
                    placeholder={f.placeholder}
                    autoComplete="off"
                    spellCheck={false}
                    onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
                  />
                )}
              </Field>
            </div>
          );
        })}
      </Grid>
    </Stack>
  );
}
