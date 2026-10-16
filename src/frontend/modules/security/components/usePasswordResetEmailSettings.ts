// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * Settings plumbing for the password-reset email card.
 *
 * Four settings rows, seeded by migration 087, read at send time by
 * `users.ResetEmailSettings`.
 */

type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

export const KEYS = {
  channelId: 'security.password_reset_email_channel_id',
  from: 'security.password_reset_email_from',
  subject: 'security.password_reset_email_subject',
  body: 'security.password_reset_email_body',
} as const;

export type EmailForm = {
  channelId: string;
  from: string;
  subject: string;
  body: string;
};

// Mirrors the seeds in 087_password_reset_email.sql. Used only when a
// row is missing (a database that predates the migration).
export const DEFAULTS: EmailForm = {
  channelId: '',
  from: '',
  subject: 'Reset your {{appName}} password',
  body: [
    'Hello {{displayName}},',
    '',
    'We received a request to reset the password for your {{appName}} account ({{username}}).',
    '',
    'Choose a new password here:',
    '{{link}}',
    '',
    'This link can be used once and expires in {{expiresMinutes}} minutes, at {{expiresAt}}.',
    '',
    'If you did not ask for this, you can ignore this message — your password stays unchanged and no one has been given access to your account.',
  ].join('\n'),
};

// The tokens renderResetTemplate substitutes, in the order they are
// most useful to an author.
export const PLACEHOLDERS = [
  '{{displayName}}',
  '{{username}}',
  '{{email}}',
  '{{link}}',
  '{{expiresMinutes}}',
  '{{expiresAt}}',
  '{{appName}}',
] as const;

export function useResetEmailSettings() {
  return useQuery({
    queryKey: ['settings', 'password-reset-email'],
    queryFn: async (): Promise<EmailForm> => {
      const entries = await Promise.all(
        (Object.entries(KEYS) as Array<[keyof EmailForm, string]>).map(async ([field, key]) => {
          const res = await api.get<SettingRow>(`/settings/${key}`);
          // A 404 is the ordinary state of a database that predates
          // migration 087, not an error worth failing the card over.
          return [field, res.success && res.data ? res.data.value : null] as const;
        }),
      );

      const out: EmailForm = { ...DEFAULTS };
      for (const [field, value] of entries) {
        if (value !== null) out[field] = value;
      }
      return out;
    },
  });
}

export function useSaveResetEmailSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');

  return useMutation({
    mutationFn: (form: EmailForm) =>
      api
        .put('/settings', {
          settings: (Object.entries(KEYS) as Array<[keyof EmailForm, string]>).map(([field, key]) => ({
            key,
            value: form[field],
            category: 'security',
          })),
        })
        .then(assertSuccess),
    meta: { success: t('toast.settingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'password-reset-email'] });
    },
  });
}
