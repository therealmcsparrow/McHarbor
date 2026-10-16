// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-truenas — config form. TrueNAS uses local user + password
// over HTTP Basic auth against the WebDAV share. No OAuth, no app
// passwords — the only credential the operator needs is the local
// TrueNAS user that owns the WebDAV share.

import type { ChangeEvent } from 'react';
import { IconInfoCircle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Field, Section } from '@resources/components/ui/Form';

type FieldDef = {
  key: string;
  labelKey: string;
  type: 'string' | 'secret';
  required: boolean;
  helpKey: string;
  placeholder?: string;
};

export const FIELDS: readonly FieldDef[] = [
  {
    key: 'host',
    labelKey: 'form.hostLabel',
    type: 'string',
    required: true,
    helpKey: 'form.hostHelp',
    placeholder: 'https://nas.local',
  },
  {
    key: 'username',
    labelKey: 'fields.username',
    type: 'string',
    required: true,
    helpKey: 'form.usernameHelp',
  },
  {
    key: 'password',
    labelKey: 'fields.password',
    type: 'secret',
    required: true,
    helpKey: 'form.passwordHelp',
  },
  {
    key: 'basePath',
    labelKey: 'form.basePathLabel',
    type: 'string',
    required: false,
    helpKey: 'form.basePathHelp',
    placeholder: 'Backups',
  },
  {
    key: 'apiPath',
    labelKey: 'form.apiPathLabel',
    type: 'string',
    required: false,
    helpKey: 'form.apiPathHelp',
  },
];

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function ConfigForm({ value, onChange, t: hostT }: Props) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  function set(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }

  return (
    <Stack gap={4}>
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.introTitle')} help="TrueNAS">
        <p className="text-sm text-foreground">{t('form.introBody')}</p>
      </Section>

      <Grid columns={2} gap={4}>
        {FIELDS.map((f) => (
          <Field key={f.key} id={f.key} label={t(f.labelKey)} required={f.required} description={t(f.helpKey)}>
            <Input
              id={f.key}
              type={f.type === 'secret' ? 'password' : 'text'}
              value={value[f.key] ?? ''}
              placeholder={f.placeholder}
              autoComplete="off"
              onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
            />
          </Field>
        ))}
      </Grid>
    </Stack>
  );
}
