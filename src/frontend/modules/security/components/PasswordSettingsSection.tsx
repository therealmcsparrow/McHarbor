// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { IconClockHour4 } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { Label } from '@resources/components/ui/Label';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Spinner } from '@resources/components/ui/Spinner';
import { PasswordPolicyCard } from '@resources/components/settings/PasswordPolicyCard';
import { PasswordResetEmailCard } from './PasswordResetEmailCard';
import { AutoLogoutCard } from './AutoLogoutCard';
import { MFARequirementCard } from './MFARequirementCard';

type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

const TTL_KEY = 'security.password_reset_ttl_minutes';
const DEFAULT_TTL_MINUTES = 60;
const MIN_TTL_MINUTES = 5;
const MAX_TTL_MINUTES = 60 * 24 * 7; // 7 days

function usePasswordResetTTL() {
  return useQuery({
    queryKey: ['settings', TTL_KEY],
    queryFn: async () => {
      const res = await api.get<SettingRow>(`/settings/${TTL_KEY}`);
      if (!res.success || !res.data) return null;
      const minutes = parseInt(res.data.value, 10);
      return Number.isFinite(minutes) ? minutes : null;
    },
  });
}

function useSavePasswordResetTTL() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (minutes: number) =>
      api
        .put(`/settings/${TTL_KEY}`, {
          key: TTL_KEY,
          value: String(minutes),
          category: 'security',
        })
        .then(assertSuccess),
    meta: { success: t('toast.settingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', TTL_KEY] });
    },
  });
}

export function PasswordSettingsSection() {
  const { t } = useTranslation('security');
  const { data: currentMinutes, isLoading } = usePasswordResetTTL();
  const save = useSavePasswordResetTTL();

  const [minutes, setMinutes] = useState<string>('');
  const [error, setError] = useState('');

  const [syncedMinutes, setSyncedMinutes] = useState<typeof currentMinutes>(undefined);
  if (currentMinutes !== syncedMinutes) {
    setSyncedMinutes(currentMinutes);
    if (currentMinutes !== null && currentMinutes !== undefined) {
      setMinutes(String(currentMinutes));
    } else if (currentMinutes === null && minutes === '') {
      setMinutes(String(DEFAULT_TTL_MINUTES));
    }
  }

  function handleSubmit(evt: FormEvent<HTMLFormElement>) {
    evt.preventDefault();
    const parsed = parseInt(minutes, 10);
    if (!Number.isFinite(parsed) || parsed < MIN_TTL_MINUTES || parsed > MAX_TTL_MINUTES) {
      setError(
        t('password.resetTtl.invalid', {
          min: MIN_TTL_MINUTES,
          max: MAX_TTL_MINUTES,
        }),
      );
      return;
    }
    setError('');
    save.mutate(parsed);
  }

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="py-12">
        <Spinner size="lg" />
      </Flex>
    );
  }

  return (
    <div className="mc-glass space-y-6 rounded-lg p-6">
      <div>
        <h2 className="text-base font-semibold text-foreground">{t('password.resetTtl.title')}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{t('password.resetTtl.description')}</p>
      </div>

      <form className="max-w-md space-y-4" onSubmit={handleSubmit}>
        <Flex align="center" gap={2}>
          <IconClockHour4 className="size-4 text-muted-foreground" />
          <TypographyText size="sm" weight="medium">
            {t('password.resetTtl.label')}
          </TypographyText>
        </Flex>

        <div>
          <Label htmlFor="password-reset-ttl" className="mb-1.5 text-xs">
            {t('password.resetTtl.fieldLabel')}
          </Label>
          <NumberInput
            id="password-reset-ttl"
            nullable
            allowOutOfRange
            min={MIN_TTL_MINUTES}
            max={MAX_TTL_MINUTES}
            step={1}
            value={toNumberOrNull(minutes)}
            onChange={(next) => setMinutes(toNumberText(next))}
            disabled={save.isPending}
          />
          <TypographyText size="xs" tone="muted" className="mt-1">
            {t('password.resetTtl.hint', {
              min: MIN_TTL_MINUTES,
              max: MAX_TTL_MINUTES,
            })}
          </TypographyText>
        </div>

        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
            {error}
          </div>
        )}

        <Button type="submit" permission={PERM.SETTINGS_MANAGE} disabled={save.isPending}>
          {save.isPending ? t('common:actions.saving') : t('common:actions.save')}
        </Button>
      </form>

      <hr className="border-border" />

      {/* Delivery sits next to the lifetime it advertises: the body
          template quotes {{expiresMinutes}}, which is the field above. */}
      <PasswordResetEmailCard />

      <hr className="border-border" />

      <PasswordPolicyCard />

      <hr className="border-border" />

      {/* Not a password rule, but the same question in a different
          register: the policy above decides how hard the secret is to
          guess, this decides how long an unattended screen keeps
          answering to it. */}
      <AutoLogoutCard />

      <hr className="border-border" />

      <MFARequirementCard />
    </div>
  );
}
