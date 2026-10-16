// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconMail, IconAlertTriangle } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Flex } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { useEmailChannels } from '../hooks/useEmailChannels';
import { PasswordResetEmailFields } from './PasswordResetEmailFields';
import {
  DEFAULTS,
  useResetEmailSettings,
  useSaveResetEmailSettings,
  type EmailForm,
} from './usePasswordResetEmailSettings';

/**
 * Email delivery for password-reset links.
 *
 * The card deliberately does not offer a "recipient" field: the recipient
 * is always the account that asked for the reset, and letting an operator
 * pin one would turn the feature into a way to have every reset link
 * mailed somewhere else.
 *
 * From is an override, not a requirement — blank means the chosen
 * channel keeps its own sender, which is normally the address whose
 * SPF/DKIM records actually match.
 */
export function PasswordResetEmailCard() {
  // 'security' owns the card's own copy; 'common' supplies the two
  // button labels every settings form in the app already shares.
  const { t } = useTranslation(['security', 'common']);
  const { data: saved, isLoading } = useResetEmailSettings();
  const { data: channels, isLoading: channelsLoading } = useEmailChannels();
  const save = useSaveResetEmailSettings();

  const [form, setForm] = useState<EmailForm>(DEFAULTS);
  const [error, setError] = useState('');

  const [syncedSaved, setSyncedSaved] = useState<typeof saved>(undefined);
  if (saved !== syncedSaved) {
    setSyncedSaved(saved);
    if (saved) setForm(saved);
  }

  function update<K extends keyof EmailForm>(field: K, value: EmailForm[K]) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function handleSubmit(evt: FormEvent<HTMLFormElement>) {
    evt.preventDefault();

    // A From override that is not an address would be rejected by the
    // relay at send time, where nobody is watching. Catch it here.
    const from = form.from.trim();
    if (from !== '' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(from)) {
      setError(t('password.resetEmail.invalidFrom'));
      return;
    }
    if (form.channelId !== '' && form.subject.trim() === '') {
      setError(t('password.resetEmail.subjectRequired'));
      return;
    }
    if (form.channelId !== '' && form.body.trim() === '') {
      setError(t('password.resetEmail.bodyRequired'));
      return;
    }
    if (form.channelId !== '' && !form.body.includes('{{link}}')) {
      // The token is passed in rather than written into the locale
      // string: i18next would read a literal {{link}} in the copy as an
      // interpolation of a variable named `link` and blank it out.
      setError(t('password.resetEmail.linkRequired', { token: '{{link}}' }));
      return;
    }

    setError('');
    save.mutate({ ...form, from });
  }

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="py-12">
        <Spinner size="lg" />
      </Flex>
    );
  }

  const channelOptions = [
    { value: '', label: t('password.resetEmail.channelNone') },
    ...(channels ?? []).map((channel) => ({
      value: channel.id,
      label: channel.isDefault
        ? t('password.resetEmail.channelOptionDefault', {
            name: channel.name,
            type: channel.typeLabel,
          })
        : t('password.resetEmail.channelOption', {
            name: channel.name,
            type: channel.typeLabel,
          }),
    })),
  ];

  const noChannels = !channelsLoading && (channels?.length ?? 0) === 0;

  return (
    <div className="space-y-6">
      <div>
        <Flex align="center" gap={2}>
          <IconMail className="size-4 text-muted-foreground" />
          <h2 className="text-base font-semibold text-foreground">{t('password.resetEmail.title')}</h2>
        </Flex>
        <p className="mt-1 text-sm text-muted-foreground">{t('password.resetEmail.description')}</p>
      </div>

      {noChannels && (
        <Flex
          align="start"
          gap={2}
          className="rounded-md border border-warning/40 bg-warning/10 p-3 text-xs text-warning"
        >
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
          <span>{t('password.resetEmail.noChannels')}</span>
        </Flex>
      )}

      <form className="max-w-2xl space-y-4" onSubmit={handleSubmit}>
        <PasswordResetEmailFields
          form={form}
          channelOptions={channelOptions}
          channelsLoading={channelsLoading}
          saving={save.isPending}
          onChange={update}
        />

        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
            {error}
          </div>
        )}

        <Button type="submit" disabled={save.isPending} permission={PERM.SETTINGS_MANAGE}>
          {save.isPending ? t('common:actions.saving') : t('common:actions.save')}
        </Button>
      </form>
    </div>
  );
}
