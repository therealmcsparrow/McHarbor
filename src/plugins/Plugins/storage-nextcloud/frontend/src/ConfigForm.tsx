// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-nextcloud — config form for Nextcloud. Strings come from the
// host-supplied `t` (namespace = plugin slug).
//
// This plugin owns a single LocationType (nextcloud); every field
// is unconditionally rendered. There is no cfg["location_type"]
// branch.
//
// The refresh_token is written by the host's OAuth consent flow
// (Nextcloud's /apps/oauth2/authorize -> /api/storage-locations/oauth/callback)
// and never shown in the form. Operators only ever see (and need
// to edit) the server URL, username, secret (OAuth client_secret
// or app password), and the optional folder under the user's root.
//
// The host's typed save path persists the standard columns
// (clientId, clientSecret, basePath, etc.) directly to
// first-class columns. Plugin-specific fields like `apiPath` are
// appended to the per-row Config JSON map by the backend's typed
// save handler.

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
};

// Common fields shown for every Nextcloud destination:
//
//   host — base URL of the Nextcloud instance (required).
//   username — Nextcloud login (required).
//   basePath — folder under the user's Files root (optional).
//   clientId / clientSecret — OAuth 2.0 OR app-password credentials.
//   apiPath — WebDAV path override (rare).
//
// clientSecret is intentionally a single field that accepts EITHER
// an OAuth 2.0 client_secret OR an app password. The plugin's
// credential helpers fall back from `client_secret` to `password`
// when needed, so operators only need to fill one field.
export const FIELDS: ConfigField[] = [
  {
    key: 'host',
    labelKey: 'form.host.label',
    type: 'string',
    required: true,
    helpKey: 'form.host.help',
    placeholder: 'https://cloud.example.com',
  },
  {
    key: 'username',
    labelKey: 'form.username.label',
    type: 'string',
    required: true,
    helpKey: 'form.username.help',
    placeholder: 'backups',
  },
  {
    key: 'clientId',
    labelKey: 'form.clientId.label',
    type: 'string',
    required: false,
    helpKey: 'form.clientId.help',
  },
  {
    key: 'clientSecret',
    labelKey: 'fields.clientSecret',
    type: 'secret',
    required: true,
    helpKey: 'form.clientSecret.help',
  },
  {
    key: 'basePath',
    labelKey: 'form.basePath.label',
    type: 'string',
    required: false,
    helpKey: 'form.basePath.help',
    placeholder: 'Backups',
  },
  {
    key: 'apiPath',
    labelKey: 'form.apiPath.label',
    type: 'string',
    required: false,
    helpKey: 'form.apiPath.help',
    placeholder: '/remote.php/dav/files/username',
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
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.introTitle')} help="Nextcloud">
        <p className="text-sm text-foreground">{t('form.introBody')}</p>
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
              autoComplete="off"
              onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
            />
          </Field>
        ))}
      </Grid>
    </Stack>
  );
}
