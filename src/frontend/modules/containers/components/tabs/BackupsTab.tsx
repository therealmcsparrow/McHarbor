// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { BackupRunsPanel } from './BackupRunsPanel';
import { RestoreRunDialog } from '../RestoreRunDialog';
import { UploadRestoreDialog } from './UploadRestoreDialog';
import {
  type ContainerBackupRun,
  useCancelContainerBackupRun,
  useContainerBackupRuns,
  useDeleteContainerBackupRun,
  useRetryDestinationUpload,
} from '@resources/hooks/useContainerBackups';

type BackupsTabProps = {
  containerId: string;
  containerName: string;
};

// The run list destination of the Data group. The settings group and the
// saved schedules are their own tabs on the same rail — see
// BackupSettingsTab and BackupSchedulesTab. The restore_* progress stage
// map lives in RestoreRunDialog.tsx, which owns the full restore dialog so
// the cross-environment Backups page can reuse it without copy-paste.
export function BackupsTab({ containerId }: BackupsTabProps) {
  const { t } = useTranslation('containers');
  const { data: runs = [], isLoading: runsLoading } = useContainerBackupRuns(containerId);
  const deleteRun = useDeleteContainerBackupRun();
  const cancelRun = useCancelContainerBackupRun();
  const retryDestinationUpload = useRetryDestinationUpload();

  const [deleteRunTarget, setDeleteRunTarget] = useState<ContainerBackupRun | null>(null);
  const [cancelRunTarget, setCancelRunTarget] = useState<ContainerBackupRun | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<ContainerBackupRun | null>(null);
  const [uploadRestoreOpen, setUploadRestoreOpen] = useState(false);
  const [restoreKey, setRestoreKey] = useState('');

  if (runsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <BackupRunsPanel
        runs={runs}
        onRestore={(run) => {
          setRestoreKey('');
          setRestoreTarget(run);
        }}
        onDelete={(run) => setDeleteRunTarget(run)}
        onCancel={(run) => setCancelRunTarget(run)}
        onRetryUpload={(run, destination) =>
          retryDestinationUpload.mutate({
            runId: run.id,
            destinationId: destination.id,
          })
        }
        onUploadRestore={() => setUploadRestoreOpen(true)}
        retryPendingFor={retryDestinationUpload.isPending ? retryDestinationUpload.variables?.destinationId : undefined}
        deletePending={deleteRun.isPending}
        cancelPending={cancelRun.isPending}
      />

      {deleteRunTarget && (
        <ConfirmDialog
          open={!!deleteRunTarget}
          onOpenChange={(isOpen) => {
            if (!isOpen) setDeleteRunTarget(null);
          }}
          title={t('backups.deleteRunTitle')}
          description={t('backups.deleteRunDescription')}
          confirmLabel={t('backups.deleteRun')}
          loading={deleteRun.isPending}
          onConfirm={() => {
            deleteRun.mutate(deleteRunTarget.id, {
              onSuccess: () => setDeleteRunTarget(null),
            });
          }}
        />
      )}
      {cancelRunTarget && (
        <ConfirmDialog
          open={!!cancelRunTarget}
          onOpenChange={(isOpen) => {
            if (!isOpen) setCancelRunTarget(null);
          }}
          title={t('backups.cancelRunTitle')}
          description={t('backups.cancelRunDescription')}
          confirmLabel={t('backups.cancelRunConfirm')}
          loading={cancelRun.isPending}
          onConfirm={() => {
            cancelRun.mutate(cancelRunTarget.id, {
              onSuccess: () => setCancelRunTarget(null),
            });
          }}
        />
      )}
      <RestoreRunDialog
        run={restoreTarget}
        containerId={containerId}
        initialSecretKey={restoreKey}
        onClose={() => {
          setRestoreTarget(null);
          setRestoreKey('');
        }}
      />
      <UploadRestoreDialog
        containerId={containerId}
        dialogOpen={uploadRestoreOpen}
        onOpenChange={setUploadRestoreOpen}
        onStaged={(run, secretKey) => {
          setRestoreKey(secretKey);
          setRestoreTarget(run);
        }}
      />
    </div>
  );
}
