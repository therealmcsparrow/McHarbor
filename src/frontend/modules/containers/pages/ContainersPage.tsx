// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';
import { ContainerUtilityDialogs } from '../components/ContainerUtilityDialogs';
import { ContainersPageActions } from '../components/ContainersPageActions';
import { ContainerStateFilterChips } from '../components/ContainerStateFilterChips';
import { useContainerChangeHighlights } from '../hooks/useContainerChangeHighlights';
import { useContainers, useContainerAction, usePruneContainers } from '../hooks/useContainers';
import { useContainersBulkStats } from '../hooks/useContainersBulkStats';
import { useCheckContainerUpdates, useContainerUpdateResults } from '../hooks/useContainerUpdates';
import { useContainerOperationRunner } from '../hooks/useContainerOperationRunner';
import { useContainersPageTable } from '../hooks/useContainersPageTable';
import {
  useContainersPageDialogState,
  useContainersPageFilters,
  useUpdateAvailableIds,
} from '../hooks/useContainersPageState';
import { useContainersViewStore } from '../stores/containers-view';
import { ContainersCardBody } from './ContainersCardBody';
import { ContainersLoadBanners, ContainersLoadFailed } from './ContainersLoadBanners';
import { ContainersPageDialogs } from './ContainersPageDialogs';

export default function ContainersPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const envId = useEnvironmentStore((state) => state.currentId);
  const uploadActive = useEnvironmentUploadActive(envId);
  const {
    data: containers = [],
    isLoading,
    error: containersError,
    failureCount,
    errorUpdatedAt,
    refetch: refetchContainers,
  } = useContainers();
  const { data: statsMap } = useContainersBulkStats();
  const { highlightContainerChangesEnabled } = useCurrentEnvironmentActivitySettings();
  const action = useContainerAction();
  const pruneContainers = usePruneContainers();
  const checkUpdates = useCheckContainerUpdates();
  const batchProgress = useBatchProgressOperation();
  const updateResults = useContainerUpdateResults();
  const { viewMode, setViewMode } = useContainersViewStore();
  const ui = useContainersPageDialogState();

  const updateAvailableIDs = useUpdateAvailableIds(updateResults);
  const filters = useContainersPageFilters(containers, updateAvailableIDs);
  const highlightedIDs = useContainerChangeHighlights({
    containers,
    updateResults,
    enabled: highlightContainerChangesEnabled,
  });

  const { allTargets, updateTargets, runContainerOperation } = useContainerOperationRunner(
    containers,
    updateAvailableIDs,
    batchProgress,
  );

  const { columns, batchActions } = useContainersPageTable(ui, action, updateResults, runContainerOperation);
  const retry = () => void refetchContainers();

  if (containersError && containers.length === 0) {
    return <ContainersLoadFailed error={containersError} onRetry={retry} />;
  }

  const actionProps = {
    checkingUpdates: checkUpdates.isPending,
    batchRunning: batchProgress.isRunning,
    uploadActive,
    updatesAvailable: updateTargets.length,
    totalContainers: allTargets.length,
    onCheckUpdates: () => checkUpdates.mutate(undefined),
    onUpdateAll: () => runContainerOperation('update', updateTargets),
    onRecreateAll: () => ui.setRecreateAllConfirmOpen(true),
    onPruneUnused: () => ui.setPruneConfirmOpen(true),
    t,
  };

  return (
    <>
      <TablePage
        id="containers"
        title={t('title')}
        description={t('description', { count: containers.length })}
        actions={
          <ContainersPageActions {...actionProps} scope="header" viewMode={viewMode} onViewModeChange={setViewMode} />
        }
        banner={
          <ContainersLoadBanners
            error={containersError}
            failureCount={failureCount}
            updatedAt={errorUpdatedAt}
            onRetry={retry}
          />
        }
        search={filters.searchProps}
        filters={
          <ContainerStateFilterChips
            containers={containers}
            updateAvailableIDs={updateAvailableIDs}
            value={filters.stateFacet}
            onChange={filters.setStateFacet}
          />
        }
        savedFilters={filters.savedFilters}
        toolbar={{
          // Column and density controls act on the table; in card view they
          // would be levers attached to nothing.
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: retry, isFetching: isLoading },
          export: { filename: 'mcharbor-containers' },
          // One action set, rendered once, shared by both views.
          actions: <ContainersPageActions {...actionProps} scope="toolbar" onCreate={() => ui.setCreateOpen(true)} />,
        }}
        columns={columns}
        data={filters.visibleContainers}
        loading={isLoading}
        empty={{ title: t('noContainersFound') }}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.Id}
        getRowClassName={(row) =>
          highlightedIDs.has(row.Id) ? 'bg-warning/10 shadow-[inset_4px_0_0_var(--status-warning)]' : undefined
        }
        onRowClick={(row) => navigate(`/containers/${row.Id}`)}
        body={
          viewMode === 'table' ? undefined : (
            <ContainersCardBody
              containers={containers}
              visibleContainers={filters.visibleContainers}
              statsMap={statsMap}
              highlightedIds={highlightedIDs}
              isLoading={isLoading}
              ui={ui}
              onAction={action.mutate}
            />
          )
        }
      />

      <ContainerUtilityDialogs
        removeTarget={ui.removeTarget}
        moveTarget={ui.moveTarget}
        renameTarget={ui.renameTarget}
        terminalTarget={ui.terminalTarget}
        logsTarget={ui.logsTarget}
        takeOverTarget={ui.takeOverTarget}
        progressState={batchProgress.dialogState}
        closeProgress={batchProgress.closeDialog}
        setRemoveTarget={ui.setRemoveTarget}
        setMoveTarget={ui.setMoveTarget}
        setRenameTarget={ui.setRenameTarget}
        setTerminalTarget={ui.setTerminalTarget}
        setLogsTarget={ui.setLogsTarget}
        setTakeOverTarget={ui.setTakeOverTarget}
        t={t}
      />

      <ContainersPageDialogs
        ui={ui}
        allTargets={allTargets}
        batchRunning={batchProgress.isRunning}
        pruning={pruneContainers.isPending}
        onRun={runContainerOperation}
        onPrune={() => pruneContainers.mutate()}
      />
    </>
  );
}
