// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-sharepoint-online — config form for SharePoint Online.
// Strings are keys resolved by the host-supplied `t` (namespace = plugin slug).
//
// This plugin owns a single LocationType (sharepoint); every field
// is unconditionally rendered. There is no cfg["location_type"]
// branch and no OneDrive for Business upn field — OneDrive for
// Business lives in storage-onedrive-business.
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

// Common fields shown for every SharePoint Online destination:
//
//   tenant_id, client_id, client_secret — required (the Entra
//     app registration the operator must set up themselves;
//     no embedded default — McSparrow does not run a public
//     multi-tenant app for work/school tenants).
//   drive_id, base_path, site_id — optional. drive_id falls back
//     to /sites/{site_id}/drive. base_path is intentionally not
//     prefilled — operators either leave blank (drive root) or
//     enter an explicit Graph path like /Backups/2026.
//
//   Sites.ReadWrite.All is required on the Entra app for
//   /sites/{site_id}/drive. The form's help text tells the
//   operator to grant both Files.ReadWrite.All and
//   Sites.ReadWrite.All.
export const FIELDS: ConfigField[] = [
  {
    key: 'tenant_id',
    labelKey: 'form.tenant_id.label',
    type: 'string',
    required: true,
    helpKey: 'form.tenant_id.help',
    placeholder: '00000000-0000-0000-0000-000000000000',
  },
  {
    key: 'client_id',
    labelKey: 'form.client_id.label',
    type: 'string',
    required: true,
    helpKey: 'form.client_id.help',
    placeholder: '00000000-0000-0000-0000-000000000000',
  },
  {
    key: 'client_secret',
    labelKey: 'form.client_secret.label',
    type: 'secret',
    required: true,
    helpKey: 'form.client_secret.help',
  },
  {
    key: 'drive_id',
    labelKey: 'form.drive_id.label',
    type: 'string',
    required: false,
    helpKey: 'form.drive_id.help',
    placeholder: 'b!abc123…',
  },
  {
    key: 'base_path',
    labelKey: 'form.base_path.label',
    type: 'string',
    required: false,
    helpKey: 'form.base_path.help',
  },
  {
    key: 'site_id',
    labelKey: 'form.site_id.label',
    type: 'string',
    required: false,
    helpKey: 'form.site_id.help',
    placeholder: 'contoso.sharepoint.com,guid1,guid2',
  },
];

const INTRO_TITLE_KEY = 'form.introTitle';
const INTRO_BODY_KEY = 'form.introBody';

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
      <Section icon={<IconInfoCircle className="size-4" />} title={t(INTRO_TITLE_KEY)} help="Microsoft">
        <p className="text-sm text-foreground">{t(INTRO_BODY_KEY)}</p>
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
