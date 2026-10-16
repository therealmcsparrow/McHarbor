// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// storage-dropbox — config form for Dropbox.
//
// Text comes from the host-supplied `t` (namespace = plugin slug).
//
// This plugin owns a single LocationType (dropbox); every field is
// unconditionally rendered. There is no cfg["location_type"] branch.
//
// Dropbox destinations authenticate with a long-lived access token
// generated from a Dropbox app. McHarbor does not run a Dropbox app
// for the operator — each McHarbor installation uses its own. The
// optional base_path is the folder inside Dropbox where backups
// land (default: app root).

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

// Common fields shown for every Dropbox destination:
//
//   access_token — required. The "Generated access token" from a
//     Dropbox app's settings page (App folder or Full Dropbox —
//     pick whichever the app is configured for). McHarbor stores
//     the token encrypted at rest. Tokens expire after the
//     Dropbox app's expiration setting (or never, for apps that
//     opt out of expiration).
//
//   base_path — optional. Folder inside Dropbox where backups
//     land. Leave blank to use the app root (or the user's
//     Dropbox root for Full Dropbox apps). Example: /Backups/2026.
//     Not prefilled on purpose — operators either leave blank
//     (root) or enter an explicit path.
export const FIELDS: ConfigField[] = [
  {
    key: 'access_token',
    labelKey: 'fields.accessToken',
    type: 'secret',
    required: true,
    helpKey: 'help.accessToken',
  },
  {
    key: 'base_path',
    labelKey: 'fields.basePath',
    type: 'string',
    required: false,
    helpKey: 'form.basePathHelp',
    placeholder: '/Backups/2026',
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
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.introTitle')} help="Dropbox">
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
