// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-teams — config form for the Microsoft Teams notification
// channel.
//
// Renders when the host shows the channel edit dialog and the
// operator picks "teams" as the channel_type. Mirrors the
// backend's teamsConfigSchema() so the host can render the form
// without the bundle (the bundle is for advanced UI; the host
// owns the default renderer).
//
// Field list:
//   webhook_url   required  string  Teams Incoming Webhook connector URL
//   theme_color   optional  string  6-char hex (no #) for the Adaptive Card's
//                                   left accent stripe; defaults to Teams
//                                   brand purple (5059C9)

import type { ChangeEvent } from 'react';
import { IconInfoCircle, IconPalette, IconAlertTriangle, IconCheck } from '@tabler/icons-react';
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

// Source of truth for the form. Mirrors teamsConfigSchema() in
// backend/main.go.
export const FIELDS: ConfigField[] = [
  {
    key: 'webhook_url',
    labelKey: 'form.fields.webhook_url.label',
    type: 'string',
    required: true,
    placeholder: 'https://outlook.office.com/webhook/…@…/IncomingWebhook/…/…',
    helpKey: 'form.fields.webhook_url.help',
  },
  {
    key: 'theme_color',
    labelKey: 'form.fields.theme_color.label',
    type: 'string',
    required: false,
    defaultValue: '5059C9',
    placeholder: '5059C9',
    helpKey: 'form.fields.theme_color.help',
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
      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.webhookTitle')} help={t('form.webhookHelp')}>
        <p className="text-sm text-foreground">
          <Rich text={t('form.webhookBody')} />
        </p>
      </Section>

      <Grid columns={2} gap={4}>
        {FIELDS.map((f) => {
          const v = value[f.key] ?? '';
          const id = `comm-teams-${f.key}`;
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

      <Section icon={<IconPalette className="size-4" />} title={t('form.themeTitle')} help={t('form.themeHelp')}>
        <p className="text-sm text-foreground">
          <Rich text={t('form.themeIntro')} />
        </p>
        <ul className="mt-2 ml-5 list-disc space-y-1 text-sm text-foreground">
          <li>
            <Rich text={t('form.themePurple')} />
          </li>
          <li>
            <Rich text={t('form.themeRed')} />
          </li>
          <li>
            <Rich text={t('form.themeGreen')} />
          </li>
          <li>
            <Rich text={t('form.themeAmber')} />
          </li>
        </ul>
        <p className="mt-2 text-sm text-foreground">
          <Rich text={t('form.themeOutro')} />
        </p>
      </Section>

      <Section
        icon={<IconAlertTriangle className="size-4" />}
        title={t('form.securityTitle')}
        help={t('form.securityHelp')}
      >
        <p className="text-sm text-foreground">
          <Rich text={t('form.securityIntro')} />
        </p>
        <ul className="mt-2 ml-5 list-disc space-y-1 text-sm text-foreground">
          <li>
            <Rich text={t('form.securityGit')} />
          </li>
          <li>
            <Rich text={t('form.securityShare')} />
          </li>
          <li>
            <Rich text={t('form.securityRotate')} />
          </li>
        </ul>
      </Section>

      <Section icon={<IconCheck className="size-4" />} title={t('form.testTitle')} help={t('form.testHelp')}>
        <p className="text-sm text-foreground">
          <Rich text={t('form.testBody')} />
        </p>
      </Section>
    </Stack>
  );
}
