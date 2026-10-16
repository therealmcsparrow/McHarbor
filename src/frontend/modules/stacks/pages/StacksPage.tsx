// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TablePage } from '@resources/layout/TablePage';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { isProtectedStack } from '@core/utils/protection';
import { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
import { useStacks, useStackAction, useDeleteStack, usePruneStack } from '@resources/hooks/useStacks';
import { useCheckStackUpdates } from '../hooks/useStackUpdates';
import { useStackBatchOperations } from '../hooks/useStackBatchOperations';
import { useStacksViewStore } from '../stores/stacks-view';
import { useStackTableColumns } from '../components/StackTableColumns';
import { getStackBatchActions } from '../components/StackBatchActions';
import { StackCardView } from '../components/StackCardView';
import { StacksPageDialogs } from '../components/StacksPageDialogs';
import { useStacksPageDialogs } from '../hooks/useStacksPageDialogs';
import { StacksPageHeaderActions } from '../components/StacksPageHeaderActions';

export default function StacksPage() {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');
  const { data: stacks = [], isLoading, isFetching, refetch } = useStacks();
  const { data: statsMap } = useContainersBulkStats();
  const action = useStackAction();
  const deleteStack = useDeleteStack();
  const pruneStack = usePruneStack();
  const checkUpdates = useCheckStackUpdates();
  const { viewMode, setViewMode } = useStacksViewStore();
  const batch = useStackBatchOperations(stacks, t);
  const {
    batchProgress,
    updateResults,
    updateAvailableTargets,
    reinstallTargets,
    getManagedTargets,
    runStackOperation,
  } = batch;

  const dialogs = useStacksPageDialogs();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const handleAction = (stackName: string, act: string) => {
    const stack = stacks.find((item) => item.name === stackName);
    if (stack && isProtectedStack(stack)) return;
    if (stack && act === 'down') dialogs.setDownTarget(stack);
    else if (stack) action.mutate({ stack, action: act });
  };
  const handleDelete = (stackName: string) => {
    const stack = stacks.find((item) => item.name === stackName);
    if (stack && isProtectedStack(stack)) return;
    deleteStack.mutate(stackName);
  };
  const updatesAvailable = updateAvailableTargets.length;

  const columns = useStackTableColumns({
    onEdit: dialogs.setEditTarget,
    onLogs: dialogs.setLogsTarget,
    onRemove: dialogs.setRemoveTarget,
    onTakeOver: dialogs.setTakeOverTarget,
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

  // The card view shares the table's query, modes and fields.
  const matcher = useMemo(() => createSearchMatcher(query, searchMode), [query, searchMode]);
  const cardStacks = useMemo(
    () =>
      query.trim() === ''
        ? stacks
        : stacks.filter(
            (stack) =>
              matchesSearchFields(query, searchMode, [stack.name, ...stack.services.map((svc) => svc.name)]).matched,
          ),
    [stacks, query, searchMode],
  );

  return (
    <>
      <TablePage
        id="stacks"
        title={t('title')}
        description={t('description', { count: stacks.length })}
        actions={<StacksPageHeaderActions scope="header" t={t} viewMode={viewMode} setViewMode={setViewMode} />}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('searchPlaceholder'),
          error: matcher.error !== null,
          fields: (stack) => [stack.name, ...stack.services.map((svc) => svc.name)],
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-stacks' },
          // One action set for both views. The card view used to render its own
          // copy alongside a second search box.
          actions: (
            <StacksPageHeaderActions
              scope="toolbar"
              t={t}
              viewMode={viewMode}
              setViewMode={setViewMode}
              setCreateOpen={dialogs.setCreateOpen}
              checkUpdatesPending={checkUpdates.isPending}
              batchRunning={batchProgress.isRunning}
              updatesAvailable={updatesAvailable}
              reinstallCount={reinstallTargets.length}
              pruneRunning={pruneStack.isPending}
              onCheckUpdates={() => checkUpdates.mutate(undefined)}
              onUpdateAll={() => runStackOperation('update', updateAvailableTargets)}
              onReinstallAll={() => dialogs.setReinstallAllConfirmOpen(true)}
              onPruneUnused={() => dialogs.setPruneConfirmOpen(true)}
            />
          ),
        }}
        columns={columns}
        data={stacks}
        loading={isLoading}
        empty={{ title: t('emptyMessage') }}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.id}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <StackCardView
                stacks={cardStacks}
                isLoading={isLoading}
                statsMap={statsMap}
                onAction={handleAction}
                onEdit={dialogs.setEditTarget}
                onLogs={dialogs.setLogsTarget}
                onRemove={dialogs.setRemoveTarget}
                onTakeOver={dialogs.setTakeOverTarget}
              />
            </div>
          )
        }
      />

      <StacksPageDialogs stacks={stacks} dialogs={dialogs} batch={batch} />
    </>
  );
}
