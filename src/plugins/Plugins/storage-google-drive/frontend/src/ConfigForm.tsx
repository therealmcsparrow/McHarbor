// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-google-drive — config form for Google Drive.
//
// Strings come from the host-supplied `t` (namespace = plugin slug).
//
// This plugin owns a single LocationType (google_drive); every
// field is unconditionally rendered. There is no cfg["location_type"]
// branch.
//
// The refresh_token is written by the host's OAuth consent flow
// (accounts.google.com/o/oauth2/v2/auth -> /api/storage-locations/oauth/callback)
// and never shown in the form. Operators only ever see (and need
// to edit) the OAuth client credentials + an optional Drive
// folder ID.
//
// The host's storage dialog stores plugin-owned values in its
// typed columns: `clientId` -> first-class `client_id` (encrypted),
// `clientSecret` -> first-class `client_secret` (encrypted),
// `basePath` -> first-class `base_path`. The plugin's backend
// client reads `base_path` as a folder ID when the value looks
// like a Drive ID (Google's documented 25-44 char shape), and
// falls back to Drive root otherwise. So the plugin's ConfigForm
// writes its `folderId` field to the host's `basePath` column
// directly — no per-row Config JSON round-trip is needed.

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

// Common fields shown for every Google Drive destination:
//
//   client_id, client_secret — required (the Google Cloud console
//     OAuth client the operator must set up themselves; no
//     embedded default — McSparrow does not run a public
//     multi-tenant app for Drive).
//   folder_id — optional. Defaults to My Drive root. Operators
//     either leave blank (uploads land in My Drive root) or paste
//     a specific folder ID from drive.google.com (the tail end of
//     the folder URL after /folders/).
export const FIELDS: ConfigField[] = [
  {
    key: 'clientId',
    labelKey: 'form.clientId.label',
    type: 'string',
    required: true,
    helpKey: 'form.clientId.help',
    placeholder: '000000000000-xxxxxxxxxxxxxxxxxxxxxxxx.apps.googleusercontent.com',
  },
  {
    key: 'clientSecret',
    labelKey: 'form.clientSecret.label',
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
    placeholder: '1AbC2dEf3GhI4jKl5mNo6pQr7StU8vWxY',
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
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.introTitle')} help="Google">
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
