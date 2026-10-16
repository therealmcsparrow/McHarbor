// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconShieldLock } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText, Caption } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { useMFABeginEnrollment, type MFAEnrollment } from '@resources/hooks/useMFA';
import { useAuth } from '@core/auth/useAuth';
import { MFAEnrollmentSteps } from '../components/MFAEnrollmentSteps';

export default function MFAEnrollmentRequiredPage() {
  const { t } = useTranslation('auth');
  const navigate = useNavigate();
  const user = useAuth((s) => s.user);
  const isLoading = useAuth((s) => s.isLoading);
  const enrollmentRequired = useAuth((s) => s.mfaEnrollmentRequired);
  const clearEnrollmentRequired = useAuth((s) => s.clearMFAEnrollmentRequired);
  const logout = useAuth((s) => s.logout);
  const begin = useMFABeginEnrollment();
  const [enrollment, setEnrollment] = useState<MFAEnrollment | null>(null);

  useEffect(() => {
    if (isLoading) return;
    if (!user) {
      void navigate('/login', { replace: true });
      return;
    }
    if (!enrollmentRequired) {
      void navigate('/dashboard', { replace: true });
    }
  }, [isLoading, user, enrollmentRequired, navigate]);

  function handleBegin() {
    setEnrollment(null);
    begin.mutate(undefined, {
      onSuccess: (data) => {
        if (data) setEnrollment(data);
      },
    });
  }

  function handleConfirmed() {
    setEnrollment(null);
    clearEnrollmentRequired();
    toast.success(t('mfaEnrollment.success'));
    void navigate('/dashboard', { replace: true });
  }

  if (isLoading || !user) {
    return (
      <Flex align="center" justify="center" className="h-screen bg-background">
        <Spinner size="xl" />
      </Flex>
    );
  }

  return (
    <Flex align="center" justify="center" className="min-h-screen bg-background p-4">
      <Box className="w-full max-w-2xl rounded-lg border border-border bg-card p-8 shadow-lg">
        <Flex direction="col" align="center" gap={3} className="mb-6">
          <Flex align="center" justify="center" aria-hidden="true" className="size-12 bg-warning/10 text-warning">
            <IconShieldLock className="size-5" />
          </Flex>
          <Caption>{t('mfaEnrollment.title')}</Caption>
        </Flex>
        <TypographyText size="sm" tone="muted" className="mb-6">
          {t('mfaEnrollment.description')}
        </TypographyText>

        {enrollment ? (
          <MFAEnrollmentSteps
            otpauthUri={enrollment.otpauthUri}
            secret={enrollment.secret}
            recoveryCodes={enrollment.recoveryCodes}
            onCancel={() => setEnrollment(null)}
            onConfirmed={handleConfirmed}
          />
        ) : (
          <Flex direction="col" gap={2}>
            <Button onClick={handleBegin} disabled={begin.isPending} className="w-full">
              {begin.isPending ? <Spinner size="sm" /> : t('mfaEnrollment.begin')}
            </Button>
            {begin.isError && (
              <TypographyText size="sm" tone="destructive" role="alert">
                {t('mfa.beginFailed', { ns: 'common' })}
              </TypographyText>
            )}
          </Flex>
        )}

        <Button type="button" variant="ghost" size="sm" className="mt-4 w-full" onClick={logout}>
          {t('forceChange.signOutInstead')}
        </Button>
      </Box>
    </Flex>
  );
}
