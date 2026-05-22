// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  IconPlus,
  IconLayoutGrid,
  IconLayoutList,
  IconRefresh,
  IconArrowUp,
  IconRotate,
} from '@tabler/icons-react';
import { OperationProgressDialog } from '@resources/components/OperationProgressDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import {
  useStacks,
  useStackAction,
  useDeleteStack,
  useUpdateStack,
} from '../hooks/useStacks';
import {
  useCheckStackUpdates,
  useStackOperationActions,
  useStackUpdateResults,
  type StackOperationMode,
  type StackOperationTarget,
} from '../hooks/useStackUpdates';
import type { StackInfo } from '../hooks/useStacks';
import { useStacksViewStore } from '../stores/stacks-view';
import { useStackTableColumns } from '../components/StackTableColumns';
import { getStackBatchActions } from '../components/StackBatchActions';
import { StackCardView } from '../components/StackCardView';
import { CreateStackDialog } from '../components/CreateStackDialog';
import { EditStackDialog } from '../components/EditStackDialog';
import { StackLogsDialog } from '../components/StackLogsDialog';
import { TakeOverDialog } from '../components/TakeOverDialog';

export default function StacksPage() {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const { data: stacks = [], isLoading } = useStacks();
  const { data: statsMap } = useContainersBulkStats();
  const action = useStackAction();
  const deleteStack = useDeleteStack();
  const updateStack = useUpdateStack();
  const checkUpdates = useCheckStackUpdates();
  const stackOperations = useStackOperationActions();
  const batchProgress = useBatchProgressOperation();
  const updateResults = useStackUpdateResults();
  const { viewMode, setViewMode } = useStacksViewStore();

  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<StackInfo | null>(null);
  const [logsTarget, setLogsTarget] = useState<StackInfo | null>(null);
  const [removeTarget, setRemoveTarget] = useState<StackInfo | null>(null);
  const [takeOverTarget, setTakeOverTarget] = useState<StackInfo | null>(null);

  const handleAction = (name: string, act: string) => action.mutate({ name, action: act });
  const handleDelete = (name: string) => deleteStack.mutate(name);
  const managedStacks = stacks.filter((stack) => stack.type === 'managed');
  const updateAvailableTargets = managedStacks
    .filter((stack) => updateResults?.get(stack.name)?.updateAvailable)
    .map(toStackTarget);
  const reinstallTargets = managedStacks.map(toStackTarget);
  const updatesAvailable = updateAvailableTargets.length;

  function toStackTarget(stack: StackInfo): StackOperationTarget {
    return {
      name: stack.name,
      images: [...new Set(stack.services.map((service) => service.image).filter(Boolean))],
    };
  }

  function getManagedTargets(rows: StackInfo[]) {
    const managedRows = rows.filter((row) => row.type === 'managed');

    if (managedRows.length !== rows.length) {
      toast.warning(t('updates.managedOnly'));
    }

    return managedRows.map(toStackTarget);
  }

  async function runStackOperation(mode: StackOperationMode, targets: StackOperationTarget[]) {
    if (targets.length === 0) {
      return;
    }

    const batchScanner = stackOperations.createBatchScanner();

    await batchProgress.runBatchOperation({
      title: t(`updates.progress.${mode}Title`),
      description: t(`updates.progress.${mode}Description`),
      actionLabel: t(`updates.progress.${mode}Action`),
      items: targets,
      getKey: (target) => target.name,
      getLabel: (target) => target.name,
      execute: async (target, progress) => {
        const result = await stackOperations.runOperation(target, mode, {
          log: progress.log,
          scanner: batchScanner,
        });
        return { detail: result.detail };
      },
      onComplete: async (result) => {
        await stackOperations.finalizeOperation(mode, result.successfulItems);
      },
      getSuccessToast: (result) =>
        result.successCount === 1
          ? mode === 'update'
            ? t('toast.updated')
            : t('toast.reinstalled')
          : mode === 'update'
            ? t('updates.bulkUpdated', { count: result.successCount })
            : t('updates.bulkReinstalled', { count: result.successCount }),
      getErrorToast: (result) =>
        t('updates.partialComplete', {
          success: result.successCount,
          total: result.total,
        }),
    });
  }

  const columns = useStackTableColumns({
    onEdit: setEditTarget,
    onLogs: setLogsTarget,
    onRemove: setRemoveTarget,
    onTakeOver: setTakeOverTarget,
    onAction: handleAction,
    updateResults,
  });

  const batchActions = getStackBatchActions({
    tc,
    onAction: handleAction,
    onDelete: handleDelete,
    onUpdateSelected: (rows) => runStackOperation('update', getManagedTargets(rows)),
    onReinstallSelected: (rows) => runStackOperation('reinstall', getManagedTargets(rows)),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description', { count: stacks.length })}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => checkUpdates.mutate(undefined)}
              disabled={checkUpdates.isPending || batchProgress.isRunning}
            >
              {checkUpdates.isPending ? <Spinner size="sm" /> : <IconRefresh className="h-4 w-4" />}
              {t('updates.searchForUpdates')}
            </Button>
            {updatesAvailable > 0 && (
              <Button
                variant="outline"
                onClick={() => runStackOperation('update', updateAvailableTargets)}
                disabled={batchProgress.isRunning}
              >
                {batchProgress.isRunning ? <Spinner size="sm" /> : <IconArrowUp className="h-4 w-4" />}
                {t('updates.updateAll', { count: updatesAvailable })}
              </Button>
            )}
            {reinstallTargets.length > 0 && (
              <Button
                variant="outline"
                onClick={() => runStackOperation('reinstall', reinstallTargets)}
                disabled={batchProgress.isRunning}
              >
                {batchProgress.isRunning ? <Spinner size="sm" /> : <IconRotate className="h-4 w-4" />}
                {t('updates.reinstallAll', { count: reinstallTargets.length })}
              </Button>
            )}
            <Button onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('deployStack')}
            </Button>
            <div className="h-6 w-px bg-border" />
            <div className="flex items-center rounded-lg border border-border">
              <Button
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('table')}
                aria-label={t('tableView')}
              >
                <IconLayoutList className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === 'card' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('card')}
                aria-label={t('cardView')}
              >
                <IconLayoutGrid className="h-4 w-4" />
              </Button>
            </div>
          </>
        }
      />

      {viewMode === 'table' ? (
        <DataGrid
          data={stacks}
          columns={columns}
          searchKey="name"
          searchPlaceholder={t('searchPlaceholder')}
          loading={isLoading}
          emptyMessage={t('emptyMessage')}
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.id}
        />
      ) : (
        <StackCardView
          stacks={stacks}
          isLoading={isLoading}
          statsMap={statsMap}
          onAction={handleAction}
          onEdit={setEditTarget}
          onLogs={setLogsTarget}
          onRemove={setRemoveTarget}
          onTakeOver={setTakeOverTarget}
        />
      )}

      <CreateStackDialog open={createOpen} onOpenChange={setCreateOpen} />

      <EditStackDialog
        stack={editTarget}
        onClose={() => setEditTarget(null)}
        onSave={(compose) => {
          if (!editTarget) return;
          updateStack.mutate(
            { name: editTarget.name, compose },
            { onSuccess: () => setEditTarget(null) },
          );
        }}
        saving={updateStack.isPending}
      />

      <StackLogsDialog
        stack={logsTarget}
        onClose={() => setLogsTarget(null)}
      />

      <TakeOverDialog
        open={takeOverTarget !== null}
        onOpenChange={(open) => !open && setTakeOverTarget(null)}
        stackName={takeOverTarget?.name}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => !open && setRemoveTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription', { name: removeTarget?.name ?? '' })}
        confirmLabel={t('confirm.removeLabel')}
        onConfirm={() => {
          if (removeTarget) deleteStack.mutate(removeTarget.name);
          setRemoveTarget(null);
        }}
        loading={deleteStack.isPending}
      />

      <OperationProgressDialog
        state={batchProgress.dialogState}
        onClose={batchProgress.closeDialog}
      />
    </div>
  );
}
