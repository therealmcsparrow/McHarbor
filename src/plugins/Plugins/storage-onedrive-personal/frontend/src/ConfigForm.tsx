// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-onedrive-personal — config form for OneDrive Personal.
//
// Surfaces the four fields the operator can enter up front
// (drive_id, base_path, client_id, client_secret). All four are
// optional — the plugin ships an embedded McSparrow-owned
// client_id so the operator only needs to click "Sign in with
// Microsoft" for the default experience. The host's OAuth
// consent flow writes the refresh_token into the cfg map silently
// — the form never shows it.
//
// Public-client support: matches Home Assistant's OneDrive
// pattern. The Azure AD app registration is created as a "Mobile
// and desktop applications" public client under "Personal
// Microsoft accounts only" — Microsoft issues no client_secret
// for that path. The form therefore renders client_secret as an
// optional field. Confidential ("Web") registrations still work:
// leave client_secret blank if the registration is public, fill
// it in for confidential. The Go client (onedrive_personal_client.go)
// omits client_secret from the OAuth /token request when blank.
//
// Tenant is no longer user-facing. The plugin hardcodes the
// consumer endpoint (login.microsoftonline.com/consumers) — that's
// the only Microsoft endpoint that works for live.com / hotmail.com /
// outlook.com accounts. Operators who hit a mixed-tenant edge
// case can override client_id to a multi-tenant app registration.

import type { ChangeEvent } from 'react';
import { IconInfoCircle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Field, Section } from '@resources/components/ui/Form';

export type ConfigFieldType = 'string' | 'secret';

export type ConfigField = {
  key: string;
  labelKey: string;
  type: ConfigFieldType;
  required: boolean;
  helpKey?: string;
  placeholder?: string;
  default?: string;
};

// Field order matches the storage dialog: optional identifiers
// first, then credentials. `refresh_token` is written by the
// host's OAuth callback and never surfaced in the form.
export const FIELDS: ConfigField[] = [
  {
    key: 'drive_id',
    labelKey: 'form.driveId.label',
    type: 'string',
    required: false,
    helpKey: 'form.driveId.help',
  },
  {
    key: 'base_path',
    labelKey: 'form.basePath.label',
    type: 'string',
    required: false,
    helpKey: 'form.basePath.help',
  },
  {
    key: 'client_id',
    labelKey: 'form.clientId.label',
    type: 'string',
    required: false,
    placeholder: '00000000-0000-0000-0000-000000000000',
    helpKey: 'form.clientId.help',
  },
  {
    key: 'client_secret',
    labelKey: 'form.clientSecret.label',
    type: 'secret',
    required: false,
    helpKey: 'form.clientSecret.help',
  },
];

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

const LIVE_HOST = 'login.live.com';
const SCOPES = ['Files.ReadWrite', 'User.Read', 'offline_access'];

export function ConfigForm({ value, onChange, t: hostT }: Props) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;
  function set(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }
  return (
    <Stack gap={4}>
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.intro.title')} help="live.com">
        <p className="text-sm text-foreground">
          {t('form.intro.save')}
          <strong>{t('form.intro.signIn')}</strong>
          {t('form.intro.redirect')}
          <strong>{LIVE_HOST}</strong>
          {t('form.intro.account')}
          <strong>{SCOPES[0]}</strong>
          {t('form.intro.comma')}
          <strong>{SCOPES[1]}</strong>
          {t('form.intro.and')}
          <strong>{SCOPES[2]}</strong>
          {t('form.intro.tail')}
        </p>
      </Section>

      <Grid columns={2} gap={4}>
        {FIELDS.map((f) => (
          <Field
            key={f.key}
            id={f.key}
            label={t(f.labelKey)}
            required={f.required}
            description={f.helpKey ? t(f.helpKey) : undefined}
          >
            <Input
              id={f.key}
              type={f.type === 'secret' ? 'password' : 'text'}
              value={value[f.key] ?? ''}
              placeholder={f.placeholder}
              autoComplete={f.type === 'secret' ? 'off' : undefined}
              onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
            />
          </Field>
        ))}
      </Grid>
    </Stack>
  );
}
