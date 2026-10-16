// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-telegram — config form for the Telegram notification channel.
//
// Renders when the host shows the channel create/edit dialog and the
// operator picks "Telegram" as the channel_type. Mirrors the backend's
// telegramConfigSchema() plus the host-owned `name` column that every
// communication channel needs before the host lets it be saved.
//
// Field list:
//   name                   required  string  host channel name
//   bot_token              required  secret  from @BotFather
//   chat_id                required  string  numeric ID or @channelname
//   parse_mode             optional  select  "" | "Markdown" | "HTML"
//   disable_notification   optional  select  "false" | "true"

import type { ChangeEvent } from 'react';
import { useEffect } from 'react';
import { IconInfoCircle, IconBellOff, IconCode } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
  /** 'edit' when the host is editing an existing channel: secrets are redacted and optional. */
  mode?: 'create' | 'edit';
};

const requiredFields = ['name', 'chat_id'];
const secretFields = ['bot_token'];
const echoKey = (key: string) => key;
const BOT_TOKEN_PLACEHOLDER = '0000000000:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx';
const CHAT_ID_PLACEHOLDER = '123456789 / @mychannel';
const MD_BOLD = '*bold*';
const MD_ITALIC = '_italic_';
const MD_LINK = '[text](url)';
const HTML_BOLD = '<b>';
const HTML_ITALIC = '<i>';
const HTML_LINK = '<a href="...">';

type TextFieldProps = {
  fieldKey: string;
  label: string;
  placeholder: string;
  help: string;
  type?: string;
  value: Record<string, string>;
  isEdit: boolean;
  keepHint: string;
  onSet: (key: string, next: string) => void;
};

function TextField({
  fieldKey,
  label,
  placeholder,
  help,
  type = 'text',
  value,
  isEdit,
  keepHint,
  onSet,
}: TextFieldProps) {
  const id = `comm-telegram-${fieldKey}`;
  const effectivePlaceholder = isEdit && secretFields.includes(fieldKey) ? keepHint : placeholder;
  return (
    <div>
      <Label htmlFor={id} className="mb-1">
        {label}
      </Label>
      <Input
        id={id}
        type={type}
        value={value[fieldKey] ?? ''}
        placeholder={effectivePlaceholder}
        autoComplete="off"
        spellCheck={false}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onSet(fieldKey, e.target.value)}
      />
      <p className="mt-1 text-xs text-muted-foreground">{help}</p>
    </div>
  );
}

export function ConfigForm({ value, onChange, onError, t: hostT, mode }: Props) {
  const isEdit = mode === 'edit';
  const t = hostT ?? echoKey;
  const requiredMessage = t('error.required');
  useEffect(() => {
    for (const field of requiredFields) {
      onError?.(field, (value[field] ?? '').trim() ? null : requiredMessage);
    }
    for (const field of secretFields) {
      onError?.(field, isEdit || (value[field] ?? '').trim() ? null : requiredMessage);
    }
  }, [value, isEdit, onError, requiredMessage]);

  function set(key: string, nextValue: string) {
    onChange({ ...value, [key]: nextValue });
  }

  const parseModeOptions = [
    { value: '', label: t('parseMode.plain') },
    { value: 'Markdown', label: t('parseMode.markdown') },
    { value: 'HTML', label: t('parseMode.html') },
  ];

  const silentOptions = [
    { value: 'false', label: t('silent.off') },
    { value: 'true', label: t('silent.on') },
  ];

  const shared = { value, isEdit, keepHint: t('secretKeepHint'), onSet: set };

  return (
    <Stack gap={4}>
      <Section icon={<IconInfoCircle className="size-4" />} title={t('title')} help={t('subtitle')}>
        <p className="text-sm text-foreground">{t('description')}</p>
      </Section>

      <TextField
        {...shared}
        fieldKey="name"
        label={t('fields.name')}
        placeholder={t('placeholder.name')}
        help={t('help.name')}
      />

      <Grid columns={2} gap={4}>
        <TextField
          {...shared}
          fieldKey="bot_token"
          label={t('fields.botToken')}
          placeholder={BOT_TOKEN_PLACEHOLDER}
          help={t('help.botToken')}
          type="password"
        />
        <TextField
          {...shared}
          fieldKey="chat_id"
          label={t('fields.chatId')}
          placeholder={CHAT_ID_PLACEHOLDER}
          help={t('help.chatId')}
        />
      </Grid>

      <Grid columns={2} gap={4}>
        <div>
          <Label htmlFor="comm-telegram-parse_mode" className="mb-1">
            {t('fields.parseMode')}
          </Label>
          <Select
            value={value.parse_mode ?? ''}
            onChange={(next) => set('parse_mode', next)}
            options={parseModeOptions}
            searchable={false}
            portalled={false}
            ariaLabel={t('fields.parseMode')}
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('help.parseMode')}</p>
        </div>
        <div>
          <Label htmlFor="comm-telegram-disable_notification" className="mb-1">
            {t('fields.disableNotification')}
          </Label>
          <Select
            value={value.disable_notification || 'false'}
            onChange={(next) => set('disable_notification', next)}
            options={silentOptions}
            searchable={false}
            portalled={false}
            ariaLabel={t('fields.disableNotification')}
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('help.disableNotification')}</p>
        </div>
      </Grid>

      <Section icon={<IconCode className="size-4" />} title={t('tips.title')} help={t('tips.help')}>
        <ul className="ml-5 list-disc space-y-1 text-sm text-foreground">
          <li>
            <strong>{t('parseMode.markdown')}</strong> — <code>{MD_BOLD}</code>, <code>{MD_ITALIC}</code>,{' '}
            <code>{MD_LINK}</code>. {t('tips.markdown')} (<code>_</code> <code>*</code> <code>[</code> <code>]</code>{' '}
            <code>`</code>) {t('tips.markdownSuffix')}
          </li>
          <li>
            <strong>{t('parseMode.html')}</strong> — <code>{HTML_BOLD}</code> <code>{HTML_ITALIC}</code>{' '}
            <code>{HTML_LINK}</code>. {t('tips.html')} <code>&lt;</code> <code>&gt;</code> <code>&amp;</code>.
          </li>
          <li>
            <strong>{t('parseMode.plain')}</strong> — {t('tips.plain')}
          </li>
        </ul>
      </Section>

      <Section icon={<IconBellOff className="size-4" />} title={t('silent.title')} help={t('silent.help')}>
        <p className="text-sm text-foreground">{t('silent.description')}</p>
      </Section>
    </Stack>
  );
}
