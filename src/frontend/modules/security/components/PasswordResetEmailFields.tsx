// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Textarea } from '@resources/components/ui/Textarea';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PLACEHOLDERS, type EmailForm } from './usePasswordResetEmailSettings';

type ChannelOption = { value: string; label: string };

type PasswordResetEmailFieldsProps = {
  form: EmailForm;
  channelOptions: ChannelOption[];
  channelsLoading: boolean;
  saving: boolean;
  onChange: <K extends keyof EmailForm>(field: K, value: EmailForm[K]) => void;
};

/** The four inputs of the password-reset email form. */
export function PasswordResetEmailFields({
  form,
  channelOptions,
  channelsLoading,
  saving,
  onChange,
}: PasswordResetEmailFieldsProps) {
  const { t } = useTranslation('security');

  return (
    <>
      <div>
        {/* No htmlFor: Select renders a Radix popover trigger, not a
            form control with a stable id. It carries ariaLabel instead. */}
        <Label className="mb-1.5 text-xs">{t('password.resetEmail.channelLabel')}</Label>
        <Select
          value={form.channelId}
          onChange={(value) => onChange('channelId', value)}
          options={channelOptions}
          variant="outline"
          searchable={false}
          disabled={saving || channelsLoading}
          ariaLabel={t('password.resetEmail.channelLabel')}
          placeholder={t('password.resetEmail.channelNone')}
        />
        <TypographyText size="xs" tone="muted" className="mt-1">
          {t('password.resetEmail.channelHint')}
        </TypographyText>
      </div>

      <div>
        <Label htmlFor="password-reset-email-from" className="mb-1.5 text-xs">
          {t('password.resetEmail.fromLabel')}
        </Label>
        <Input
          id="password-reset-email-from"
          type="email"
          value={form.from}
          onChange={(entry) => onChange('from', entry.target.value)}
          placeholder={t('password.resetEmail.fromPlaceholder')}
          disabled={saving}
          variant="outline"
        />
        <TypographyText size="xs" tone="muted" className="mt-1">
          {t('password.resetEmail.fromHint')}
        </TypographyText>
      </div>

      <div>
        <Label htmlFor="password-reset-email-subject" className="mb-1.5 text-xs">
          {t('password.resetEmail.subjectLabel')}
        </Label>
        <Input
          id="password-reset-email-subject"
          value={form.subject}
          onChange={(eventItem) => onChange('subject', eventItem.target.value)}
          disabled={saving}
          variant="outline"
        />
      </div>

      <div>
        <Label htmlFor="password-reset-email-body" className="mb-1.5 text-xs">
          {t('password.resetEmail.bodyLabel')}
        </Label>
        <Textarea
          id="password-reset-email-body"
          rows={12}
          value={form.body}
          onChange={(evt) => onChange('body', evt.target.value)}
          disabled={saving}
          variant="outline"
          className="min-h-[240px] font-mono text-xs"
        />
        <TypographyText size="xs" tone="muted" className="mt-1">
          {t('password.resetEmail.bodyHint', { token: '{{link}}' })}
        </TypographyText>
        <Flex gap={1} wrap className="mt-2">
          {PLACEHOLDERS.map((token) => (
            <code
              key={token}
              className="rounded border border-border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
            >
              {token}
            </code>
          ))}
        </Flex>
      </div>
    </>
  );
}
