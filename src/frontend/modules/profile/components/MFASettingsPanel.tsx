// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconShieldCheck, IconShieldLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useMFAStatus, useMFABeginEnrollment, type MFAEnrollment } from '@resources/hooks/useMFA';
import { formatDate } from '@resources/utils/format';
import { DisableMFADialog } from './DisableMFADialog';
import { MFAEnrollmentSteps } from './MFAEnrollmentSteps';

type Props = {
  className?: string;
};

export function MFASettingsPanel({ className }: Props) {
  const { t } = useTranslation('common');
  const statusValue = useMFAStatus();
  const begin = useMFABeginEnrollment();
  const [disableOpen, setDisableOpen] = useState(false);

  // The secret and recovery codes are readable exactly once, in the
  // response to enroll/begin — hold them here for the duration of the
  // wizard and drop them the moment it closes.
  const [enrollment, setEnrollment] = useState<MFAEnrollment | null>(null);

  function handleBegin() {
    setEnrollment(null);
    begin.mutate(undefined, {
      onSuccess: (data) => {
        if (data) setEnrollment(data);
      },
    });
  }

  return (
    <section className={className}>
      <h2 className="mb-1 text-lg font-semibold text-foreground">
        <IconShieldLock className="mr-2 inline size-5" />
        {t('mfa.title')}
      </h2>
      <p className="mb-4 text-xs text-muted-foreground">{t('mfa.description')}</p>

      {statusValue.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner size="sm" /> {t('mfa.loading')}
        </div>
      ) : statusValue.data?.enabled ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2 rounded-md border border-success/40 bg-success/10 p-3">
            <IconShieldCheck className="size-5 text-success" />
            <div className="flex-1">
              <div className="text-sm font-medium text-success">{t('mfa.enabled')}</div>
              {statusValue.data.enrolledAt && (
                <div className="text-xs text-success/70">
                  {t('mfa.enrolledAt', { date: formatDate(statusValue.data.enrolledAt) })}
                </div>
              )}
              {typeof statusValue.data.recoveryRemaining === 'number' && (
                <div className="text-xs text-success/70">
                  {t('mfa.recoveryRemaining', { count: statusValue.data.recoveryRemaining })}
                </div>
              )}
            </div>
            <Button variant="outline" size="sm" onClick={() => setDisableOpen(true)}>
              {t('mfa.disable')}
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {!enrollment ? (
            <div className="space-y-2">
              <div className="mc-glass flex items-center gap-2 rounded-md p-3">
                <IconShieldLock className="size-5 text-muted-foreground" />
                <div className="flex-1 text-sm">
                  <div className="font-medium">{t('mfa.disabled')}</div>
                  <div className="text-xs text-muted-foreground">{t('mfa.disabledHelp')}</div>
                </div>
                <Button onClick={handleBegin} disabled={begin.isPending}>
                  {begin.isPending ? <Spinner size="sm" /> : t('mfa.enable')}
                </Button>
              </div>
              {begin.isError && (
                <div role="alert" className="text-xs text-danger">
                  {t('mfa.beginFailed')}
                </div>
              )}
            </div>
          ) : (
            <MFAEnrollmentSteps
              otpauthUri={enrollment.otpauthUri}
              secret={enrollment.secret}
              recoveryCodes={enrollment.recoveryCodes}
              onCancel={() => setEnrollment(null)}
              onConfirmed={() => {
                setEnrollment(null);
                void statusValue.refetch();
              }}
            />
          )}
        </div>
      )}
      <DisableMFADialog open={disableOpen} onOpenChange={setDisableOpen} onSuccess={() => setEnrollment(null)} />
    </section>
  );
}
