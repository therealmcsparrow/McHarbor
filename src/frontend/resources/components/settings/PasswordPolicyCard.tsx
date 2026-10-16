// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconShieldCheck } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Label } from '@resources/components/ui/Label';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as UiText } from '@resources/components/ui/Typography';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  usePasswordPolicy,
  useSavePasswordPolicy,
  POLICY_DEFAULTS,
  MIN_LENGTH_BOUND,
  MAX_AGE_BOUND,
  HISTORY_BOUND,
  type PolicyForm,
} from '@resources/hooks/usePasswordPolicySettings';
import { PasswordPolicyClassToggles } from './PasswordPolicyClassToggles';

export function PasswordPolicyCard() {
  const { t } = useTranslation('settings');
  const { data, isLoading } = usePasswordPolicy();
  const save = useSavePasswordPolicy();

  const [form, setForm] = useState<PolicyForm>(POLICY_DEFAULTS);
  const [error, setError] = useState('');

  const [syncedData, setSyncedData] = useState<typeof data>(undefined);
  if (data && data !== syncedData) {
    setSyncedData(data);
    setForm(data);
  }

  function handleSubmit(evt: FormEvent<HTMLFormElement>) {
    evt.preventDefault();
    if (form.password_min_length < MIN_LENGTH_BOUND) {
      setError(
        t('security.passwordPolicy.invalidMinLength', {
          min: MIN_LENGTH_BOUND,
        }),
      );
      return;
    }
    if (form.password_max_age_days > MAX_AGE_BOUND) {
      setError(
        t('security.passwordPolicy.invalidMaxAge', {
          max: MAX_AGE_BOUND,
        }),
      );
      return;
    }
    if (form.password_history_size > HISTORY_BOUND) {
      setError(
        t('security.passwordPolicy.invalidHistorySize', {
          max: HISTORY_BOUND,
        }),
      );
      return;
    }
    setError('');
    save.mutate(form);
  }

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="py-12">
        <Spinner size="lg" />
      </Flex>
    );
  }

  return (
    <form className="space-y-6" onSubmit={handleSubmit}>
      <Flex align="center" gap={2}>
        <IconShieldCheck className="size-4 text-muted-foreground" />
        <UiText size="sm" weight="medium">
          {t('security.passwordPolicy.title')}
        </UiText>
      </Flex>

      <p className="text-xs text-muted-foreground">{t('security.passwordPolicy.description')}</p>

      <Stack gap={4}>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="password-min-length" className="text-xs">
              {t('security.passwordPolicy.minLength')}
            </Label>
            <NumberInput
              id="password-min-length"
              allowOutOfRange
              min={MIN_LENGTH_BOUND}
              max={128}
              step={1}
              value={form.password_min_length}
              onChange={(next) => setForm({ ...form, password_min_length: Math.trunc(next) })}
              disabled={save.isPending}
            />
            <UiText size="xs" tone="muted">
              {t('security.passwordPolicy.minLengthHint')}
            </UiText>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="password-max-age" className="text-xs">
              {t('security.passwordPolicy.maxAge')}
            </Label>
            <NumberInput
              id="password-max-age"
              allowOutOfRange
              min={0}
              max={MAX_AGE_BOUND}
              step={1}
              value={form.password_max_age_days}
              onChange={(next) => setForm({ ...form, password_max_age_days: Math.trunc(next) })}
              disabled={save.isPending}
            />
            <UiText size="xs" tone="muted">
              {t('security.passwordPolicy.maxAgeHint')}
            </UiText>
          </div>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="password-history-size" className="text-xs">
            {t('security.passwordPolicy.historySize')}
          </Label>
          <NumberInput
            id="password-history-size"
            allowOutOfRange
            min={0}
            max={HISTORY_BOUND}
            step={1}
            value={form.password_history_size}
            onChange={(next) => setForm({ ...form, password_history_size: Math.trunc(next) })}
            disabled={save.isPending}
          />
          <UiText size="xs" tone="muted">
            {t('security.passwordPolicy.historySizeHint')}
          </UiText>
        </div>

        <PasswordPolicyClassToggles
          form={form}
          disabled={save.isPending}
          onChange={(key, checked) => setForm({ ...form, [key]: checked })}
        />
      </Stack>

      {error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
          {error}
        </div>
      )}

      <Button type="submit" permission={PERM.SETTINGS_MANAGE} disabled={save.isPending}>
        {save.isPending ? t('common:actions.saving') : t('common:actions.save')}
      </Button>
    </form>
  );
}
