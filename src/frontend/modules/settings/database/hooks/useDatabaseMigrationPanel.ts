// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import {
  useCancelMigration,
  useDatabaseConfig,
  useDatabaseTest,
  useMigrationStatus,
  useStartMigration,
} from './useDatabase';
import { ACTIVE_MIGRATION_PHASES, type DatabaseDriver } from '../types';
import { emptyConnection } from '../components/DatabaseConnectionForm';

/**
 * Everything Settings → System → Database needs to hold: the queries, the
 * draft target connection, and the two actions that act on it.
 *
 * Kept out of the panel so that component is only the layout of the three
 * cards it renders.
 */
export function useDatabaseMigrationPanel() {
  const { t } = useTranslation('database');
  const queryClient = useQueryClient();

  const config = useDatabaseConfig();
  const test = useDatabaseTest();
  const startMigration = useStartMigration();
  const cancelMigration = useCancelMigration();

  const activeDriver = config.data?.driver;
  const [chosenDriver, setDriver] = useState<DatabaseDriver | null>(null);
  const driver: DatabaseDriver = chosenDriver ?? activeDriver ?? 'sqlite';
  const isCurrent = driver === activeDriver;
  const [moveSameKind, setMoveSameKind] = useState(false);
  const blocked = isCurrent && !moveSameKind;
  const [connection, setConnection] = useState(() => emptyConnection(driver));
  const [rawMode, setRawMode] = useState(false);
  const [restartAfter, setRestartAfter] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);

  const migration = useMigrationStatus(dialogOpen);
  const jobActive = Boolean(migration.data && ACTIVE_MIGRATION_PHASES.includes(migration.data.phase));

  useEffect(() => {
    setConnection(emptyConnection(driver));
    setMoveSameKind(false);
    test.reset();
    // `test` is a stable mutation object from TanStack Query; including it
    // would re-run this on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [driver]);

  const target = useMemo(() => ({ ...connection, driver }), [connection, driver]);
  const readOnly = config.data?.envPinned ?? false;
  const canMigrate = Boolean(test.data?.ok) && !readOnly && !jobActive && !blocked;

  const runTest = () => {
    if (blocked) return;
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

  const runMigration = () => {
    if (!canMigrate) return;
    setDialogOpen(true);
    startMigration.mutate(
      { target, restartAfter },
      {
        onSettled: (_result, failure) => {
          if (failure) setDialogOpen(false);
        },
      },
    );
  };

  const closeDialog = () => {
    setDialogOpen(false);
    void queryClient.invalidateQueries({ queryKey: ['database', 'config'] });
  };

  return {
    config,
    test,
    startMigration,
    cancelMigration,
    migration,
    driver,
    activeDriver,
    isCurrent,
    blocked,
    moveSameKind,
    setMoveSameKind,
    setDriver,
    setConnection,
    rawMode,
    setRawMode,
    restartAfter,
    setRestartAfter,
    dialogOpen,
    closeDialog,
    target,
    readOnly,
    jobActive,
    canMigrate,
    runTest,
    runMigration,
  };
}
