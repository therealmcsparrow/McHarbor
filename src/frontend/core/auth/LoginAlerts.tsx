// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconClockHour4, IconLockSquare } from '@tabler/icons-react';
import { Notice } from './LoginForms';

type LoginAlertsProps = {
  locked: boolean;
  disabled: boolean;
  error: string;
  countdown: string;
};

export function LoginAlerts({ locked, disabled, error, countdown }: LoginAlertsProps) {
  const { t } = useTranslation('auth');
  return (
    <>
      {locked && (
        <Notice icon={<IconClockHour4 className="mt-0.5 size-5 shrink-0" />} title={t('login.locked')}>
          {t('login.lockedRetryIn', { time: countdown })}
        </Notice>
      )}

      {disabled && (
        <Notice icon={<IconLockSquare className="mt-0.5 size-5 shrink-0" />} title={t('login.disabled')}>
          {t('login.disabledHint')}
        </Notice>
      )}

      {error && !locked && !disabled && (
        <div
          className="mb-4 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
          role="alert"
        >
          {error}
        </div>
      )}
    </>
  );
}
