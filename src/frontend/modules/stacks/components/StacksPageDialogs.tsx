// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { OperationProgressDialog } from '@resources/components/OperationProgressDialog';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { isProtectedStack } from '@core/utils/protection';
import { useDeleteStack, usePruneStack, useUpdateStack } from '@resources/hooks/useStacks';
import type { StackInfo } from '@resources/hooks/useStacks';
import type { StacksPageDialogState } from '../hooks/useStacksPageDialogs';
import type { useStackBatchOperations } from '../hooks/useStackBatchOperations';
import { StackDownDialog } from './StackDownDialog';
import { CreateStackDialog } from './CreateStackDialog';
import { EditStackDialog } from './EditStackDialog';
import { StackLogsDialog } from './StackLogsDialog';
import { TakeOverDialog } from './TakeOverDialog';

type StacksPageDialogsProps = {
  stacks: StackInfo[];
  dialogs: StacksPageDialogState;
  batch: ReturnType<typeof useStackBatchOperations>;
};

export function StacksPageDialogs({ stacks, dialogs, batch }: StacksPageDialogsProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const deleteStack = useDeleteStack();
  const pruneStack = usePruneStack();
  const updateStack = useUpdateStack();
  const { batchProgress, reinstallTargets, runStackOperation } = batch;
  const {
    createOpen,
    setCreateOpen,
    editTarget,
    setEditTarget,
    logsTarget,
    setLogsTarget,
    removeTarget,
    setRemoveTarget,
    takeOverTarget,
    setTakeOverTarget,
    downTarget,
    setDownTarget,
    reinstallAllConfirmOpen,
    setReinstallAllConfirmOpen,
    pruneConfirmOpen,
    setPruneConfirmOpen,
  } = dialogs;

  async function pruneUnusedServices() {
    const pruneTargets = stacks.filter((stack) => stack.type === 'managed' && !isProtectedStack(stack));
    await Promise.allSettled(pruneTargets.map((stack) => pruneStack.mutateAsync(stack.name)));
  }

  return (
    <>
      <CreateStackDialog open={createOpen} onOpenChange={setCreateOpen} />

      <EditStackDialog
        stack={editTarget}
        onClose={() => setEditTarget(null)}
        onSave={(compose) => {
          if (!editTarget) return;
          updateStack.mutate({ name: editTarget.name, compose }, { onSuccess: () => setEditTarget(null) });
        }}
        saving={updateStack.isPending}
      />

      <StackLogsDialog stack={logsTarget} onClose={() => setLogsTarget(null)} />

      <StackDownDialog stack={downTarget} onOpenChange={(next) => !next && setDownTarget(null)} />

      <TakeOverDialog
        open={takeOverTarget !== null}
        onOpenChange={(next) => !next && setTakeOverTarget(null)}
        stackName={takeOverTarget?.name}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(next) => !next && setRemoveTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription', {
          name: removeTarget?.name ?? '',
        })}
        confirmLabel={t('confirm.removeLabel')}
        onConfirm={() => {
          if (removeTarget) deleteStack.mutate(removeTarget.name);
          setRemoveTarget(null);
        }}
        loading={deleteStack.isPending}
      />

      <ConfirmDialog
        open={reinstallAllConfirmOpen}
        onOpenChange={setReinstallAllConfirmOpen}
        title={t('updates.confirm.reinstallAllTitle')}
        description={t('updates.confirm.reinstallAllDescription', {
          count: reinstallTargets.length,
        })}
        confirmLabel={t('updates.progress.reinstallAction')}
        onConfirm={() => {
          void runStackOperation('reinstall', reinstallTargets);
          setReinstallAllConfirmOpen(false);
        }}
        loading={batchProgress.isRunning}
      />

      <ConfirmDialog
        open={pruneConfirmOpen}
        onOpenChange={setPruneConfirmOpen}
        title={t('prune.button')}
        description={tc('batch.confirmPruneDescription')}
        confirmLabel={t('prune.button')}
        onConfirm={() => {
          void pruneUnusedServices();
          setPruneConfirmOpen(false);
        }}
        loading={pruneStack.isPending}
      />

      <OperationProgressDialog state={batchProgress.dialogState} onClose={batchProgress.closeDialog} />
    </>
  );
}
