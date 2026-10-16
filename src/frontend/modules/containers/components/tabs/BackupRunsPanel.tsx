// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconInfoCircle, IconUpload } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { BackupRunDestinations } from './BackupRunDestinations';
import { BackupRunRowActions } from './BackupRunRowActions';
import { UploadedBadge } from '../UploadedBadge';
import { BackupRunErrorDialog, BackupRunItemsDialog } from '../BackupRunDetailDialogs';
import { runHasError } from '../../pages/backups-page-helpers';
import type { ContainerBackupRun, ContainerBackupRunDestination } from '@resources/hooks/useContainerBackups';

type BackupRunsPanelProps = {
  runs: ContainerBackupRun[];
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onCancel: (run: ContainerBackupRun) => void;
  onRetryUpload: (run: ContainerBackupRun, destination: ContainerBackupRunDestination) => void;
  onUploadRestore: () => void;
  retryPendingFor?: string;
  deletePending: boolean;
  cancelPending: boolean;
};

function backupProgressDescription(
  run: ContainerBackupRun,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (run.status !== 'running' || !run.progressStage) {
    return '';
  }
  if (run.progressMessage) {
    return run.progressMessage;
  }
  return t(`backups.progress.${run.progressStage}`);
}

export function BackupRunsPanel({
  runs,
  onRestore,
  onDelete,
  onCancel,
  onRetryUpload,
  onUploadRestore,
  retryPendingFor,
  deletePending,
  cancelPending,
}: BackupRunsPanelProps) {
  const { t } = useTranslation('containers');
  const formatPrefs = useLocaleFormat();
  // The two info dialogs are local to this panel: nothing outside it opens
  // them, so threading a target through BackupsTab would only add props.
  const [itemsTarget, setItemsTarget] = useState<ContainerBackupRun | null>(null);
  const [errorTarget, setErrorTarget] = useState<ContainerBackupRun | null>(null);

  return (
    <section className="mc-glass rounded-lg p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-foreground">{t('detail.backups')}</h2>
        <Button variant="outline" size="sm" permission={PERM.RESTORE_RUN} onClick={onUploadRestore}>
          <IconUpload className="size-4" />
          {t('backups.restoreFromFile')}
        </Button>
      </div>
      {runs.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">{t('backups.noRuns')}</p>
      ) : (
        <div className="space-y-2">
          {runs.map((run) => (
            <div key={run.id} className="rounded-lg border border-border bg-background p-3">
              <div className="flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">{t(`backups.operation.${run.operation ?? 'backup'}`)}</Badge>
                    {run.origin === 'upload' && <UploadedBadge runId={run.id} />}
                    {/* Same pair of info buttons as the Backups overview
                        table: what a partial run actually contained, and
                        why a run (or one of its uploads) failed. */}
                    {run.scope === 'partial' && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('backups.itemsDialog.open')}
                        onClick={() => setItemsTarget(run)}
                        data-testid={`run-items-info-${run.id}`}
                      >
                        <IconInfoCircle className="size-4" aria-hidden="true" />
                      </Button>
                    )}
                    <Badge
                      variant={
                        run.status === 'success' ? 'success' : run.status === 'failure' ? 'destructive' : 'secondary'
                      }
                    >
                      {run.status === 'running' && <Spinner size="sm" />}
                      {t(`backups.status.${run.status}`)}
                    </Badge>
                    {runHasError(run) && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('backups.errorDialog.open')}
                        onClick={() => setErrorTarget(run)}
                        data-testid={`run-error-info-${run.id}`}
                      >
                        <IconAlertTriangle className="size-4 text-destructive" aria-hidden="true" />
                      </Button>
                    )}
                    <span className="text-xs text-muted-foreground">{formatDate(run.startedAt, formatPrefs)}</span>
                  </div>
                  {backupProgressDescription(run, t) && (
                    <p className="mt-1 text-xs text-muted-foreground">{backupProgressDescription(run, t)}</p>
                  )}
                  <BackupRunDestinations
                    run={run}
                    onRestore={onRestore}
                    onDelete={onDelete}
                    onRetryUpload={(destination) => onRetryUpload(run, destination)}
                    retryPendingFor={retryPendingFor}
                  />
                </div>
                <BackupRunRowActions
                  run={run}
                  onRestore={onRestore}
                  onDelete={onDelete}
                  onCancel={onCancel}
                  deletePending={deletePending}
                  cancelPending={cancelPending}
                />
              </div>
            </div>
          ))}
        </div>
      )}
      <BackupRunItemsDialog run={itemsTarget} onClose={() => setItemsTarget(null)} />
      <BackupRunErrorDialog run={errorTarget} onClose={() => setErrorTarget(null)} />
    </section>
  );
}
