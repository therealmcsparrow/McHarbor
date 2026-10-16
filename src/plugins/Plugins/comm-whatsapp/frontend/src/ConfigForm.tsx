// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-whatsapp — config form for the WhatsApp notification channel.
//
// Renders when the host shows the channel edit dialog and the
// operator picks "WhatsApp" as the channel_type. A method
// selector at the top switches between the four connection
// surfaces:
//
//   cloud    Official Meta Cloud API
//   gateway  Self-hosted gateway (Baileys/wa-gateway)
//   bot      WhatsApp Business API platform (360dialog, etc.)
//   saas     Third-party SaaS provider (Twilio, MessageBird, ...)
//
// The per-method field list is rendered as a separate section;
// required-field validation lives in the host (mirrors
// whatsappConfigSchema in backend/main.go).

import type { ChangeEvent } from 'react';
import { IconBrandWhatsapp, IconInfoCircle, IconLock } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Field, Section } from '@resources/components/ui/Form';
import { ALL_METHOD_FIELDS, RECIPIENT_FIELD, fieldsFor, type ConfigField } from './fields';
import { MethodPicker } from './MethodPicker';
import { Rich } from './Rich';
import type { WhatsappMethod } from './index';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function ConfigForm({ value, onChange, t: hostT }: Props) {
  const t = (key: string) => hostT?.(key) ?? key;
  const method: WhatsappMethod = ((): WhatsappMethod => {
    const m = value['method'];
    if (m === 'cloud' || m === 'gateway' || m === 'bot' || m === 'saas') return m;
    return 'cloud';
  })();

  function set(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }

  function pickMethod(next: WhatsappMethod) {
    // Reset method-specific credentials so the next save is
    // clean for the freshly chosen connection. The host treats
    // the persisted row as opaque so a stale access_token from
    // the previous method won't be silently reused.
    const nextValue: Record<string, string> = { ...value, method: next };
    for (const f of ALL_METHOD_FIELDS) {
      delete nextValue[f.key];
    }
    onChange(nextValue);
  }

  function renderField(f: ConfigField) {
    const id = `comm-whatsapp-${f.key}`;
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
          value={value[f.key] ?? ''}
          placeholder={f.placeholder}
          autoComplete="off"
          spellCheck={false}
          onChange={(e: ChangeEvent<HTMLInputElement>) => set(f.key, e.target.value)}
        />
      </Field>
    );
  }

  return (
    <Stack gap={4}>
      <Section
        icon={<IconBrandWhatsapp className="size-4" />}
        title={t('form.channelTitle')}
        help={t('form.channelHelp')}
      >
        <p className="text-sm text-foreground">{t('form.channelBody')}</p>
      </Section>

      <Section icon={<IconLock className="size-4" />} title={t('form.methodTitle')} help={t('form.methodHelp')}>
        <MethodPicker method={method} onPick={pickMethod} t={t} />
      </Section>

      <Grid columns={2} gap={4}>
        {renderField(RECIPIENT_FIELD)}
        {fieldsFor(method).map((f) => renderField(f))}
      </Grid>

      <Section icon={<IconInfoCircle className="size-4" />} title={t('form.differTitle')} help={t('form.differHelp')}>
        <ul className="ml-5 list-disc space-y-1 text-sm text-foreground">
          <li>
            <Rich text={t('form.differCloud')} />
          </li>
          <li>
            <Rich text={t('form.differGateway')} />
          </li>
          <li>
            <Rich text={t('form.differBot')} />
          </li>
          <li>
            <Rich text={t('form.differSaas')} />
          </li>
        </ul>
      </Section>
    </Stack>
  );
}
