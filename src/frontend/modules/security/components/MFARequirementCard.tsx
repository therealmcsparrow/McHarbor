// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconShieldLock } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { useAuth } from '@core/auth/useAuth';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  MFA_REQUIREMENTS,
  parseMFARequirement,
  useMFARequirement,
  useSaveMFARequirement,
  type MFARequirement,
} from '../hooks/useMFARequirement';

export function MFARequirementCard() {
  const { t } = useTranslation('security');
  const { data: current, isLoading } = useMFARequirement();
  const save = useSaveMFARequirement();
  const [value, setValue] = useState<MFARequirement>('off');

  const [syncedCurrent, setSyncedCurrent] = useState<typeof current>(undefined);
  if (current !== syncedCurrent) {
    setSyncedCurrent(current);
    if (current) setValue(current);
  }

  const options = MFA_REQUIREMENTS.map((requirement) => ({
    value: requirement,
    label: t(`password.mfaRequirement.options.${requirement}.label`),
    description: t(`password.mfaRequirement.options.${requirement}.description`),
  }));

  function handleSubmit(evt: FormEvent<HTMLFormElement>) {
    evt.preventDefault();
    save.mutate(value, {
      onSuccess: () => {
        toast.success(t('toast.settingsSaved'));
        void useAuth.getState().checkSession();
      },
    });
  }

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="py-8">
        <Spinner size="lg" />
      </Flex>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <Flex align="center" gap={2}>
          <IconShieldLock className="size-4 text-muted-foreground" />
          <h2 className="text-base font-semibold text-foreground">{t('password.mfaRequirement.title')}</h2>
        </Flex>
        <p className="mt-1 text-sm text-muted-foreground">{t('password.mfaRequirement.description')}</p>
      </div>

      <form className="max-w-md space-y-4" onSubmit={handleSubmit}>
        <div>
          <Label className="mb-1.5 text-xs">{t('password.mfaRequirement.fieldLabel')}</Label>
          <Select
            value={value}
            onChange={(next) => setValue(parseMFARequirement(next))}
            options={options}
            searchable={false}
            disabled={save.isPending}
            ariaLabel={t('password.mfaRequirement.fieldLabel')}
          />
          <TypographyText size="xs" tone="muted" className="mt-1">
            {t('password.mfaRequirement.hint')}
          </TypographyText>
        </div>

        <Button type="submit" permission={PERM.SETTINGS_MANAGE} disabled={save.isPending}>
          {save.isPending ? t('common:actions.saving') : t('common:actions.save')}
        </Button>
      </form>
    </div>
  );
}
