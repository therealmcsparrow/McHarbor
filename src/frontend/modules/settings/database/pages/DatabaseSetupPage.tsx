// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowRight, IconPlugConnected, IconRefresh } from '@tabler/icons-react';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import { Button } from '@resources/components/ui/Button';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useApplyDatabaseSetup, useDatabaseSetupStatus, useDatabaseSetupTest } from '../hooks/useDatabase';
import type { DatabaseDriver } from '../types';
import { DatabaseConnectionForm, emptyConnection } from '../components/DatabaseConnectionForm';
import { DatabaseDriverPicker } from '../components/DatabaseDriverPicker';
import { DatabaseTestSteps } from '../components/DatabaseTestSteps';
import { SetupTokenField } from '@core/auth/SetupTokenField';

/**
 * The first-run database screen.
 *
 * It comes before the admin account on purpose. Creating the first user is
 * the first row McHarbor writes; asking the database question afterwards
 * would mean either throwing that row away or copying it, and both are worse
 * than asking first. Choosing SQLite is a real choice here, not a skip — it
 * writes the same secret file the other two do, which is what records that
 * the question was answered.
 */
const BRAND_NAME = 'McHarbor';

export default function DatabaseSetupPage() {
  const { t } = useTranslation('database');
  const navigate = useNavigate();

  const currentStatus = useDatabaseSetupStatus();
  const test = useDatabaseSetupTest();
  const { reset: resetTest } = test;
  const apply = useApplyDatabaseSetup();

  const [driver, setDriver] = useState<DatabaseDriver>('sqlite');
  const [connection, setConnection] = useState(() => emptyConnection('sqlite'));
  const [rawMode, setRawMode] = useState(false);
  const [restarting, setRestarting] = useState(false);
  // The reload timer is held so unmounting cancels it. Without that, leaving
  // this page during the wait still navigated the browser eight seconds later.
  const reloadTimer = useRef<number | null>(null);
  useEffect(
    () => () => {
      if (reloadTimer.current !== null) window.clearTimeout(reloadTimer.current);
    },
    [],
  );

  // Nothing to ask when the environment already decided, or when the
  // question has been answered — go straight to the admin account.
  useEffect(() => {
    if (!currentStatus.data) return;
    if (!currentStatus.data.needsSetup) {
      void navigate('/login', { replace: true });
      return;
    }
    if (currentStatus.data.envPinned || currentStatus.data.configured) {
      void navigate('/setup', { replace: true });
    }
  }, [currentStatus.data, navigate]);

  useEffect(() => {
    // eslint-disable-next-line react/set-state-in-effect -- a driver change discards the connection typed for the previous driver
    setConnection(emptyConnection(driver));
    resetTest();
  }, [driver, resetTest]);

  const target = useMemo(() => ({ ...connection, driver }), [connection, driver]);
  const needsTest = driver !== 'sqlite';
  const canContinue = !needsTest || Boolean(test.data?.ok);

  const runTest = () => {
    test.mutate(target, {
      onSuccess: (result) => {
        if (result.ok) toast.success(t('test.passed'));
        else {
          const failed = result.steps.find((step) => step.status === 'fail');
          toastError(failed?.detail ?? t('test.failed'), t('test.failed'));
        }
      },
    });
  };

  const continueToAdmin = () => {
    apply.mutate(target, {
      onSuccess: (result) => {
        if (result.restartScheduled) {
          // The container is coming back on the new database. Reloading
          // sooner than it can answer would only show a connection error,
          // so the page waits and then reloads into the admin step.
          setRestarting(true);
          reloadTimer.current = window.setTimeout(() => window.location.assign('/setup'), 8000);
          return;
        }
        if (result.restartRequired) {
          setRestarting(true);
          return;
        }
        void navigate('/setup', { replace: true });
      },
    });
  };

  if (currentStatus.isLoading) {
    return (
      <Flex align="center" justify="center" className="min-h-screen bg-background">
        <Spinner className="size-6" />
      </Flex>
    );
  }

  if (restarting) {
    return (
      <Flex align="center" justify="center" className="min-h-screen bg-background p-6">
        <Box className="w-full max-w-lg rounded-xl border border-border bg-card p-8 text-center">
          <Stack gap={3}>
            <Flex justify="center">
              <IconRefresh className="size-8 text-primary" aria-hidden />
            </Flex>
            <TypographyText weight="semibold" size="lg">
              {t('setup.restartingTitle')}
            </TypographyText>
            <TypographyText size="sm" tone="muted">
              {apply.data?.restartScheduled ? t('setup.restartingBody') : t('setup.restartManualBody')}
            </TypographyText>
          </Stack>
        </Box>
      </Flex>
    );
  }

  return (
    <Flex align="center" justify="center" className="min-h-screen bg-background p-6">
      <Box className="w-full max-w-3xl rounded-xl border border-border bg-card p-8 shadow-lg">
        <Stack gap={6}>
          <Stack gap={2} className="items-center text-center">
            <img src="/logo_McSparrow.svg" alt={BRAND_NAME} className="h-12" />
            <TypographyText weight="semibold" size="lg">
              {t('setup.title')}
            </TypographyText>
            <TypographyText size="sm" tone="muted">
              {t('setup.description')}
            </TypographyText>
          </Stack>

          <SetupTokenField />

          <DatabaseDriverPicker
            value={driver}
            onChange={setDriver}
            drivers={currentStatus.data?.availableDrivers ?? ['sqlite', 'postgres', 'mysql']}
            current={currentStatus.data?.driver}
          />

          <DatabaseConnectionForm
            value={target}
            onChange={setConnection}
            rawMode={rawMode}
            onRawModeChange={setRawMode}
          />

          {needsTest && (
            <Stack gap={3}>
              <Flex align="center" gap={2} wrap>
                <Button variant="outline" disabled={test.isPending} onClick={runTest}>
                  <IconPlugConnected className="size-4" aria-hidden />
                  {test.isPending ? t('test.running') : t('test.run')}
                </Button>
                <TypographyText size="sm" tone="muted">
                  {t('setup.testRequired')}
                </TypographyText>
              </Flex>
              {(test.data || test.isPending) && <DatabaseTestSteps result={test.data} running={test.isPending} />}
            </Stack>
          )}

          <Flex justify="end" gap={2}>
            <Button disabled={!canContinue || apply.isPending} onClick={continueToAdmin}>
              {t('setup.continue')}
              <IconArrowRight className="size-4" aria-hidden />
            </Button>
          </Flex>
        </Stack>
      </Box>
    </Flex>
  );
}
