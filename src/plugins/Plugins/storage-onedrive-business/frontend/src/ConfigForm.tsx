// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-onedrive-business — config form for OneDrive for Business.
//
// Strings come from the host-supplied t (namespace = plugin slug).
//
// This plugin owns a single LocationType (onedrive_business);
// every field is unconditionally rendered. There is no
// cfg["location_type"] branch and no SharePoint site_id field —
// SharePoint lives in storage-sharepoint-online.
//
// The refresh_token and access_token are written by the host's
// OAuth consent flow (login.microsoftonline.com/{tenant_id}/...) and
// never shown in the form. tenant_id is filled in by the operator
// so the consent flow knows which Entra tenant to redirect to.

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

// Common fields shown for every OneDrive for Business destination:
//
//   tenant_id, client_id, client_secret — required (the Entra
//     app registration the operator must set up themselves;
//     no embedded default — McSparrow does not run a public
//     multi-tenant app for work/school tenants).
//   drive_id, base_path, upn — optional. drive_id falls back to
//     /users/{upn}/drive (or /me/drive if upn is blank and a
//     refresh_token is present). base_path is intentionally not
//     prefilled — operators either leave blank (drive root) or
//     enter an explicit Graph path like /Backups/2026.
export const FIELDS: ConfigField[] = [
  {
    key: 'tenant_id',
    labelKey: 'form.tenantId.label',
    type: 'string',
    required: true,
    helpKey: 'form.tenantId.help',
    placeholder: '00000000-0000-0000-0000-000000000000',
  },
  {
    key: 'client_id',
    labelKey: 'form.clientId.label',
    type: 'string',
    required: true,
    helpKey: 'form.clientId.help',
    placeholder: '00000000-0000-0000-0000-000000000000',
  },
  {
    key: 'client_secret',
    labelKey: 'form.clientSecret.label',
    type: 'secret',
    required: true,
    helpKey: 'form.clientSecret.help',
  },
  {
    key: 'drive_id',
    labelKey: 'form.driveId.label',
    type: 'string',
    required: false,
    helpKey: 'form.driveId.help',
    placeholder: 'b!abc123…',
  },
  {
    key: 'base_path',
    labelKey: 'form.basePath.label',
    type: 'string',
    required: false,
    helpKey: 'form.basePath.help',
  },
  {
    key: 'upn',
    labelKey: 'form.upn.label',
    type: 'string',
    required: false,
    helpKey: 'form.upn.help',
    placeholder: 'backups@contoso.onmicrosoft.com',
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
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.intro.title')} help="Microsoft">
        <p className="text-sm text-foreground">{t('form.intro.body')}</p>
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
