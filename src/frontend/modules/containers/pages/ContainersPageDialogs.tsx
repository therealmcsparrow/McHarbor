// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { BulkRemoveContainerDialog } from '../components/BulkRemoveContainerDialog';
import { CreateContainerDialog } from '../components/CreateContainerDialog';
import { toContainerTarget } from '../hooks/useContainerOperationRunner';
import type { ContainerOperationMode, ContainerOperationTarget } from '../hooks/useContainerUpdates';
import type { useContainersPageDialogState } from '../hooks/useContainersPageState';

interface ContainersPageDialogsProps {
  ui: ReturnType<typeof useContainersPageDialogState>;
  allTargets: ContainerOperationTarget[];
  batchRunning: boolean;
  pruning: boolean;
  onRun: (mode: ContainerOperationMode, targets: ContainerOperationTarget[]) => unknown;
  onPrune: () => void;
}

export function ContainersPageDialogs({
  ui,
  allTargets,
  batchRunning,
  pruning,
  onRun,
  onPrune,
}: ContainersPageDialogsProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { recreateTarget, setRecreateTarget, bulkRemoveTargets } = ui;
  return (
    <>
      <CreateContainerDialog
        open={ui.createOpen}
        onOpenChange={ui.setCreateOpen}
        onCreated={(id) => navigate(`/containers/${id}`)}
      />

      <BulkRemoveContainerDialog
        containers={bulkRemoveTargets}
        open={bulkRemoveTargets.length > 0}
        onOpenChange={(isOpen) => !isOpen && ui.setBulkRemoveTargets([])}
      />

      <ConfirmDialog
        open={ui.recreateAllConfirmOpen}
        onOpenChange={ui.setRecreateAllConfirmOpen}
        title={t('updates.confirm.recreateAllTitle')}
        description={t('updates.confirm.recreateAllDescription', {
          count: allTargets.length,
        })}
        confirmLabel={t('updates.progress.recreateAction')}
        onConfirm={() => {
          onRun('recreate', allTargets);
          ui.setRecreateAllConfirmOpen(false);
        }}
        loading={batchRunning}
      />

      <ConfirmDialog
        open={recreateTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setRecreateTarget(null)}
        title={t('confirm.recreateTitle')}
        description={t('confirm.recreateNowDescription')}
        confirmLabel={t('confirm.recreateConfirm')}
        onConfirm={() => {
          if (recreateTarget) {
            void onRun('recreate', [toContainerTarget(recreateTarget)]);
          }
          setRecreateTarget(null);
        }}
        loading={batchRunning}
      />

      <ConfirmDialog
        open={ui.pruneConfirmOpen}
        onOpenChange={ui.setPruneConfirmOpen}
        title={t('pruneUnused')}
        description={t('pruneDescription')}
        confirmLabel={t('pruneUnused')}
        onConfirm={() => {
          onPrune();
          ui.setPruneConfirmOpen(false);
        }}
        loading={pruning}
      />
    </>
  );
}
