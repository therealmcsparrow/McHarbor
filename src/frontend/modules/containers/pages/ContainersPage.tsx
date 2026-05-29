// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { ContainerInfo } from '@core/types/docker';
import { DataGrid } from '@resources/components/DataGrid';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import { isProtectedContainer } from '@core/utils/protection';
import { ContainerCardGrid } from '../components/ContainerCardGrid';
import { ContainerUtilityDialogs } from '../components/ContainerUtilityDialogs';
import { ContainersPageActions } from '../components/ContainersPageActions';
import { useContainerBatchActions } from '../hooks/useContainerBatchActions';
import { useContainerChangeHighlights } from '../hooks/useContainerChangeHighlights';
import { useContainerColumns } from '../hooks/useContainerColumns';
import { useContainers, useContainerAction, usePruneContainers } from '../hooks/useContainers';
import { useContainersBulkStats } from '../hooks/useContainersBulkStats';
import {
  useCheckContainerUpdates,
  useContainerOperationActions,
  useContainerUpdateResults,
  type ContainerOperationMode,
  type ContainerOperationTarget,
} from '../hooks/useContainerUpdates';
import { useContainersViewStore } from '../stores/containers-view';

export default function ContainersPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { data: containers = [], isLoading } = useContainers();
  const { data: statsMap } = useContainersBulkStats();
  const { highlightContainerChangesEnabled } = useCurrentEnvironmentActivitySettings();
  const action = useContainerAction();
  const pruneContainers = usePruneContainers();
  const checkUpdates = useCheckContainerUpdates();
  const containerOperations = useContainerOperationActions();
  const batchProgress = useBatchProgressOperation();
  const updateResults = useContainerUpdateResults();
  const { viewMode, setViewMode } = useContainersViewStore();
  const [removeTarget, setRemoveTarget] = useState<ContainerInfo | null>(null);
  const [terminalTarget, setTerminalTarget] = useState<ContainerInfo | null>(null);
  const [logsTarget, setLogsTarget] = useState<ContainerInfo | null>(null);
  const [takeOverTarget, setTakeOverTarget] = useState<ContainerInfo | null>(null);
  const [reinstallAllConfirmOpen, setReinstallAllConfirmOpen] = useState(false);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);

  const updateAvailableIDs = new Set(
    updateResults ? [...updateResults.values()].filter((result) => result.updateAvailable).map((result) => result.containerId) : [],
  );
  const highlightedIDs = useContainerChangeHighlights({
    containers,
    updateResults,
    enabled: highlightContainerChangesEnabled,
  });

  function toTarget(container: ContainerInfo): ContainerOperationTarget {
    return {
      id: container.Id,
      name: container.Names?.[0]?.replace(/^\//, '') ?? container.Id,
      image: container.Image,
    };
  }

  async function runContainerOperation(mode: ContainerOperationMode, targets: ContainerOperationTarget[]) {
    if (targets.length === 0) {
      return;
    }

    const batchScanner = containerOperations.createBatchScanner();
    await batchProgress.runBatchOperation({
      title: t(`updates.progress.${mode}Title`),
      description: t(`updates.progress.${mode}Description`),
      actionLabel: t(`updates.progress.${mode}Action`),
      items: targets,
      getKey: (target) => target.id,
      getLabel: (target) => target.name,
      execute: async (target, progress) => {
        const result = await containerOperations.runOperation(target, mode, { log: progress.log, scanner: batchScanner });
        return { detail: result.detail };
      },
      onComplete: async (result) => {
        await containerOperations.finalizeOperation(mode, result.successfulItems);
      },
      getSuccessToast: (result) =>
        result.successCount === 1
          ? mode === 'update'
            ? t('toast.updated')
            : t('toast.reinstalled')
          : mode === 'update'
            ? t('updates.bulkUpdated', { count: result.successCount })
            : t('updates.bulkReinstalled', { count: result.successCount }),
      getErrorToast: (result) => t('updates.partialComplete', { success: result.successCount, total: result.total }),
    });
  }

  const mutableContainers = containers.filter((container) => !isProtectedContainer(container));
  const allTargets = mutableContainers.map(toTarget);
  const updateTargets = mutableContainers.filter((container) => updateAvailableIDs.has(container.Id)).map(toTarget);
  const columns = useContainerColumns({ action, onTerminal: setTerminalTarget, onLogs: setLogsTarget, onRemove: setRemoveTarget, onTakeOver: setTakeOverTarget, updateResults });
  const batchActions = useContainerBatchActions({
    action,
    onUpdateSelected: (rows) => runContainerOperation('update', rows.map(toTarget)),
    onReinstallSelected: (rows) => runContainerOperation('reinstall', rows.map(toTarget)),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description', { count: containers.length })}
        actions={
          <ContainersPageActions
            viewMode={viewMode}
            checkingUpdates={checkUpdates.isPending}
            batchRunning={batchProgress.isRunning}
            updatesAvailable={updateTargets.length}
            totalContainers={allTargets.length}
            onCheckUpdates={() => checkUpdates.mutate(undefined)}
            onUpdateAll={() => runContainerOperation('update', updateTargets)}
            onReinstallAll={() => setReinstallAllConfirmOpen(true)}
            onPruneUnused={() => setPruneConfirmOpen(true)}
            onCreate={() => navigate('/containers/create')}
            onViewModeChange={setViewMode}
            t={t}
          />
        }
      />

      {viewMode === 'table' ? (
        <DataGrid
          data={containers}
          columns={columns}
          searchKey="Image"
          searchPlaceholder={t('searchPlaceholder')}
          loading={isLoading}
          emptyMessage={t('noContainersFound')}
          onRowClick={(row) => navigate(`/containers/${row.Id}`)}
          tableFixed
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.Id}
          getRowClassName={(row) =>
            highlightedIDs.has(row.Id)
              ? 'bg-amber-500/10 shadow-[inset_4px_0_0_rgba(251,191,36,0.95),0_0_28px_rgba(251,191,36,0.12)]'
              : undefined
          }
        />
      ) : (
        <ContainerCardGrid
          containers={containers}
          statsMap={statsMap}
          highlightedIds={highlightedIDs}
          isLoading={isLoading}
          onAction={(id, nextAction) => {
            const target = containers.find((container) => container.Id === id);
            if (target && isProtectedContainer(target)) return;
            action.mutate({ id, action: nextAction });
          }}
          onTerminal={setTerminalTarget}
          onLogs={setLogsTarget}
          onRemove={setRemoveTarget}
          onClick={(container) => navigate(`/containers/${container.Id}`)}
        />
      )}

      <ContainerUtilityDialogs
        removeTarget={removeTarget}
        terminalTarget={terminalTarget}
        logsTarget={logsTarget}
        takeOverTarget={takeOverTarget}
        progressState={batchProgress.dialogState}
        closeProgress={batchProgress.closeDialog}
        setRemoveTarget={setRemoveTarget}
        setTerminalTarget={setTerminalTarget}
        setLogsTarget={setLogsTarget}
        setTakeOverTarget={setTakeOverTarget}
        t={t}
      />

      <ConfirmDialog
        open={reinstallAllConfirmOpen}
        onOpenChange={setReinstallAllConfirmOpen}
        title={t('updates.confirm.reinstallAllTitle')}
        description={t('updates.confirm.reinstallAllDescription', {
          count: allTargets.length,
        })}
        confirmLabel={t('updates.progress.reinstallAction')}
        onConfirm={() => {
          runContainerOperation('reinstall', allTargets);
          setReinstallAllConfirmOpen(false);
        }}
        loading={batchProgress.isRunning}
      />

      <ConfirmDialog
        open={pruneConfirmOpen}
        onOpenChange={setPruneConfirmOpen}
        title={t('pruneUnused')}
        description={t('pruneDescription')}
        confirmLabel={t('pruneUnused')}
        onConfirm={() => {
          pruneContainers.mutate();
          setPruneConfirmOpen(false);
        }}
        loading={pruneContainers.isPending}
      />
    </div>
  );
}
