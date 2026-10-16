// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-ntfy — config form for the ntfy notification channel.
//
// Renders when the host shows the channel edit dialog and the
// operator picks "ntfy" as the channel_type. Mirrors the
// backend's ntfyConfigSchema() so the host can render the form
// without the bundle (the bundle is for advanced UI; the host
// owns the default renderer).
//
// Field list:
//   server_url     required  string  ntfy base URL (default ntfy.sh)
//   topic          required  string  subscriber topic name
//   access_token   optional  secret  topic-scoped Bearer token
//   username       optional  string  Basic auth username
//   password       optional  secret  Basic auth password
//   priority       optional  string  min/low/default/high/urgent
//   tags           optional  string  comma-separated emoji shortcodes

import type { ChangeEvent } from 'react';
import { IconInfoCircle, IconKey, IconTag, IconAlertTriangle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Field, Section } from '@resources/components/ui/Form';
import { Rich } from './Rich';

export type ConfigFieldType = 'string' | 'secret' | 'bool';

export type ConfigField = {
  key: string;
  labelKey: string;
  type: ConfigFieldType;
  required: boolean;
  helpKey?: string;
  placeholder?: string;
  defaultValue?: string;
};

// Source of truth for the form. Mirrors ntfyConfigSchema()
// in backend/main.go.
export const FIELDS: ConfigField[] = [
  {
    key: 'server_url',
    labelKey: 'form.fields.server_url.label',
    type: 'string',
    required: true,
    defaultValue: 'https://ntfy.sh',
    placeholder: 'https://ntfy.sh',
    helpKey: 'form.fields.server_url.help',
  },
  {
    key: 'topic',
    labelKey: 'form.fields.topic.label',
    type: 'string',
    required: true,
    placeholder: 'mcharbor-alerts-prod',
    helpKey: 'form.fields.topic.help',
  },
  {
    key: 'access_token',
    labelKey: 'form.fields.access_token.label',
    type: 'secret',
    required: false,
    placeholder: 'tk_xxxxxxxxxxxxxxxxxxxx',
    helpKey: 'form.fields.access_token.help',
  },
  {
    key: 'username',
    labelKey: 'form.fields.username.label',
    type: 'string',
    required: false,
    placeholder: 'ntfy-user',
    helpKey: 'form.fields.username.help',
  },
  {
    key: 'password',
    labelKey: 'form.fields.password.label',
    type: 'secret',
    required: false,
    placeholder: '••••••••',
    helpKey: 'form.fields.password.help',
  },
  {
    key: 'priority',
    labelKey: 'form.fields.priority.label',
    type: 'string',
    required: false,
    defaultValue: '',
    placeholder: 'min | low | default | high | urgent',
    helpKey: 'form.fields.priority.help',
  },
  {
    key: 'tags',
    labelKey: 'form.fields.tags.label',
    type: 'string',
    required: false,
    defaultValue: '',
    placeholder: 'warning,rocket',
    helpKey: 'form.fields.tags.help',
  },
];

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function ConfigForm({ value, onChange, t: hostT }: Props) {
  const t = (key: string) => hostT?.(key) ?? key;

  function set(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }

  return (
    <Stack gap={4}>
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.publishTitle')} help={t('form.publishHelp')}>
        <p className="text-sm text-foreground">
          <Rich text={t('form.publishBody')} />
        </p>
      </Section>

      <Grid columns={2} gap={4}>
        {FIELDS.map((f) => {
          const v = value[f.key] ?? '';
          const id = `comm-ntfy-${f.key}`;
          return (
            <Field
              key={f.key}
              id={id}
              label={t(f.labelKey)}
              required={f.required}
              description={f.helpKey ? t(f.helpKey) : undefined}
            >
              <Input
                id={id}
                type={f.type === 'secret' ? 'password' : 'text'}
                value={v}
                placeholder={f.placeholder}
                autoComplete="off"
                spellCheck={false}
                onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
              />
            </Field>
          );
        })}
      </Grid>

      <Section icon={<IconKey className="size-4" />} title={t('form.authTitle')} help={t('form.authHelp')}>
        <ul className="ml-5 list-disc space-y-1 text-sm text-foreground">
          <li>
            <Rich text={t('form.authToken')} />
          </li>
          <li>
            <Rich text={t('form.authBasic')} />
          </li>
          <li>
            <Rich text={t('form.authNone')} />
          </li>
        </ul>
      </Section>

      <Section
        icon={<IconAlertTriangle className="size-4" />}
        title={t('form.priorityTitle')}
        help={t('form.priorityHelp')}
      >
        <p className="text-sm text-foreground">
          <Rich text={t('form.priorityBody')} />
        </p>
      </Section>

      <Section icon={<IconTag className="size-4" />} title={t('form.testTitle')} help={t('form.testHelp')}>
        <p className="text-sm text-foreground">
          <Rich text={t('form.testBody')} />
        </p>
      </Section>
    </Stack>
  );
}
