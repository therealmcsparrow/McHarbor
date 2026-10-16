// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { RestoreOptionsPanel } from './RestoreOptionsPanel';
import { RestoreProgressView } from './RestoreProgressView';
import { useRestoreRunState } from './useRestoreRunState';

export { RestoreProgressView } from './RestoreProgressView';

type RestoreRunDialogProps = {
  // The original backup run we want to restore. While the restore is
  // running, this stays open and a separate progress view is shown
  // inside the same dialog so the user can keep watching the
  // backup → restore pipeline from one place.
  run: ContainerBackupRun | null;
  // Required because the restore API takes containerId in the path
  // for the existing per-container tab. The cross-environment
  // Backups page passes the run's own containerId here.
  containerId: string;
  onClose: () => void;
  // The key an uploaded archive was staged with, so the operator is not asked twice.
  initialSecretKey?: string;
};

// RestoreRunDialog is the full restore flow: fetch restore options,
// ask for a secret key when the archive is encrypted, fire the
// restore mutation, and then poll the resulting restore run until
// it reaches a terminal state. Used by both the per-container
// Backups tab and the cross-environment Backups overview page.
export function RestoreRunDialog({ run, containerId, onClose, initialSecretKey }: RestoreRunDialogProps) {
  const { t } = useTranslation('containers');
  const [openedAt] = useState(() => new Date().toISOString());
  const state = useRestoreRunState(run, containerId, onClose, initialSecretKey);
  const {
    restoreBackup,
    cancelRun,
    restoreSecretKey,
    setRestoreSecretKey,
    restoreSelectedItems,
    restoreRunId,
    restoreOptionsLoading,
    restoreRun,
    restoreRunLoading,
    restoreIsActive,
    allFull,
    closeRestoreDialog,
    startRestore,
  } = state;

  return (
    <Dialog
      open={!!run}
      onOpenChange={(isOpen) => {
        // While a restore is running, ignore outside clicks so the
        // user keeps the progress view. They can still press the
        // Close button once the run reaches a terminal state.
        if (!isOpen && (restoreIsActive || restoreRunId === null)) {
          closeRestoreDialog();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          {restoreRunId ? (
            <DialogTitle>
              {restoreRun?.status === 'failure'
                ? t('backups.restoreFailedTitle')
                : restoreIsActive
                  ? t('backups.restoringTitle')
                  : t('backups.restoreCompleteTitle')}
            </DialogTitle>
          ) : (
            <DialogTitle>{t('backups.restoreTitle')}</DialogTitle>
          )}
          {restoreRunId ? null : <DialogDescription>{t('backups.restoreDescription')}</DialogDescription>}
        </DialogHeader>
        <DialogBody className="space-y-4">
          {restoreRunId ? (
            <RestoreProgressView run={restoreRun} loading={restoreRunLoading} startedAt={run?.startedAt ?? openedAt} />
          ) : (
            <>
              {run?.requiresSecretKey && (
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    {t('backups.restoreUploadKey')}
                  </label>
                  <Input
                    type="password"
                    value={restoreSecretKey}
                    onChange={(evt) => setRestoreSecretKey(evt.target.value)}
                    placeholder={t('backups.restoreSecretPlaceholder')}
                    autoComplete="off"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">{t('backups.restoreSecretDescription')}</p>
                </div>
              )}
              <RestoreOptionsPanel run={run} state={state} />
              <p className="text-xs text-muted-foreground">{t('backups.restoreWarning')}</p>
            </>
          )}
        </DialogBody>
        <DialogFooter>
          {restoreRunId ? (
            <>
              {restoreIsActive && (
                <Button
                  variant="destructive"
                  onClick={() => {
                    if (restoreRun) {
                      cancelRun.mutate(restoreRun.id);
                    }
                  }}
                  disabled={cancelRun.isPending}
                  data-testid="restore-cancel"
                >
                  {t('backups.cancelRunConfirm')}
                </Button>
              )}
              <Button onClick={closeRestoreDialog} disabled={restoreIsActive} data-testid="restore-close">
                {restoreIsActive ? t('backups.restoring') : t('backups.closeRestore')}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" onClick={closeRestoreDialog} disabled={restoreBackup.isPending}>
                {t('backups.cancelRestore')}
              </Button>
              <Button
                variant="destructive"
                onClick={startRestore}
                disabled={
                  restoreBackup.isPending ||
                  restoreOptionsLoading ||
                  (!allFull && restoreSelectedItems.length === 0) ||
                  (run?.requiresSecretKey && restoreSecretKey.trim() === '')
                }
              >
                {restoreBackup.isPending ? t('backups.restoring') : t('backups.restore')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
