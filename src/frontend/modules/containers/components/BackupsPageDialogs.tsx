// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { EditPlanDialog } from './EditPlanDialog';
import { RestoreRunDialog } from './RestoreRunDialog';
import {
  useDeleteContainerBackupPlan,
  useDeleteContainerBackupRun,
  type ContainerBackupPlan,
  type ContainerBackupRun,
} from '@resources/hooks/useContainerBackups';

type BackupsPageDialogsProps = {
  restoreTarget: ContainerBackupRun | null;
  onCloseRestore: () => void;
  deleteRunTarget: ContainerBackupRun | null;
  onCloseDeleteRun: () => void;
  bulkRunTargets: ContainerBackupRun[];
  onCloseBulkRuns: () => void;
  editTarget: ContainerBackupPlan | null;
  onCloseEdit: () => void;
  deletePlanTarget: ContainerBackupPlan | null;
  onCloseDeletePlan: () => void;
  bulkPlanTargets: ContainerBackupPlan[];
  onCloseBulkPlans: () => void;
};

/**
 * Every dialog the Backups page can open, in one place — the page itself is
 * about layout and navigation, and six inline dialogs made that hard to see.
 *
 * The delete mutations live here because nothing else on the page uses them.
 */
export function BackupsPageDialogs({
  restoreTarget,
  onCloseRestore,
  deleteRunTarget,
  onCloseDeleteRun,
  bulkRunTargets,
  onCloseBulkRuns,
  editTarget,
  onCloseEdit,
  deletePlanTarget,
  onCloseDeletePlan,
  bulkPlanTargets,
  onCloseBulkPlans,
}: BackupsPageDialogsProps) {
  const { t } = useTranslation('containers');
  const deleteRun = useDeleteContainerBackupRun();
  const deletePlan = useDeleteContainerBackupPlan();

  // Both bulk deletes run sequentially so a single failure surfaces a toast
  // and leaves the rest deletable. The mutation hooks invalidate their list
  // query on each success, so the table refreshes incrementally.
  function runBulkRunDelete() {
    const ids = bulkRunTargets.map((run) => run.id);
    onCloseBulkRuns();
    const step = (index: number) => {
      if (index >= ids.length) return;
      const id = ids[index];
      if (!id) return;
      deleteRun.mutate(id, { onSuccess: () => step(index + 1) });
    };
    step(0);
  }

  function runBulkPlanDelete() {
    const ids = bulkPlanTargets.map((plan) => plan.id);
    onCloseBulkPlans();
    const step = (index: number) => {
      if (index >= ids.length) return;
      const id = ids[index];
      if (!id) return;
      deletePlan.mutate(id, { onSuccess: () => step(index + 1) });
    };
    step(0);
  }

  return (
    <>
      <RestoreRunDialog run={restoreTarget} containerId={restoreTarget?.containerId ?? ''} onClose={onCloseRestore} />
      {deleteRunTarget && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen2) => {
            if (!isOpen2) onCloseDeleteRun();
          }}
          title={t('backups.deleteRunTitle')}
          description={t('backups.deleteRunDescription')}
          confirmLabel={t('backups.deleteRun')}
          loading={deleteRun.isPending}
          onConfirm={() => {
            deleteRun.mutate(deleteRunTarget.id, { onSuccess: onCloseDeleteRun });
          }}
        />
      )}
      {bulkRunTargets.length > 0 && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen2) => {
            if (!isOpen2) onCloseBulkRuns();
          }}
          title={t('backups.bulkRunsDeleteTitle')}
          description={t('backups.bulkRunsDeleteDescription', {
            count: bulkRunTargets.length,
          })}
          confirmLabel={t('backups.bulkRunsDeleteConfirm', {
            count: bulkRunTargets.length,
          })}
          loading={deleteRun.isPending}
          onConfirm={runBulkRunDelete}
        />
      )}

      <EditPlanDialog plan={editTarget} onClose={onCloseEdit} />
      {deletePlanTarget && (
        <ConfirmDialog
          open
          onOpenChange={(dialogOpen) => {
            if (!dialogOpen) onCloseDeletePlan();
          }}
          title={t('backups.deleteScheduleTitle')}
          description={t('backups.deleteScheduleDescription')}
          confirmLabel={t('backups.deleteSchedule')}
          loading={deletePlan.isPending}
          onConfirm={() => {
            deletePlan.mutate(deletePlanTarget.id, { onSuccess: onCloseDeletePlan });
          }}
        />
      )}
      {bulkPlanTargets.length > 0 && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => {
            if (!isOpen) onCloseBulkPlans();
          }}
          title={t('backups.bulkDeleteTitle')}
          description={t('backups.bulkDeleteDescription', {
            count: bulkPlanTargets.length,
          })}
          confirmLabel={t('backups.bulkDeleteConfirm', {
            count: bulkPlanTargets.length,
          })}
          loading={deletePlan.isPending}
          onConfirm={runBulkPlanDelete}
        />
      )}
    </>
  );
}
