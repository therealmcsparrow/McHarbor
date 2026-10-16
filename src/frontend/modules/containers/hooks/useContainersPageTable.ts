// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import type { ContainerInfo } from '@core/types/docker';
import { useContainerBatchActions } from './useContainerBatchActions';
import { useContainerColumns } from './useContainerColumns';
import type { useContainerAction } from './useContainers';
import type { ContainerOperationMode, ContainerOperationTarget } from './useContainerUpdates';
import { toContainerTarget } from './useContainerOperationRunner';
import type { useContainersPageDialogState } from './useContainersPageState';

type ColumnsArgs = Parameters<typeof useContainerColumns>[0];

/** Builds the containers table columns and bulk actions, wired to the page's dialog state. */
export function useContainersPageTable(
  ui: ReturnType<typeof useContainersPageDialogState>,
  action: ReturnType<typeof useContainerAction>,
  updateResults: ColumnsArgs['updateResults'],
  run: (mode: ContainerOperationMode, targets: ContainerOperationTarget[]) => unknown,
) {
  const runOnRows = (mode: ContainerOperationMode, rows: ContainerInfo[]) => run(mode, rows.map(toContainerTarget));
  const columns = useContainerColumns({
    action,
    onTerminal: ui.setTerminalTarget,
    onLogs: ui.setLogsTarget,
    onRename: ui.setRenameTarget,
    onMove: ui.setMoveTarget,
    onRecreate: ui.setRecreateTarget,
    onRemove: ui.setRemoveTarget,
    onTakeOver: ui.setTakeOverTarget,
    updateResults,
  });
  const batchActions = useContainerBatchActions({
    action,
    onUpdateSelected: (rows) => runOnRows('update', rows),
    onRecreateSelected: (rows) => runOnRows('recreate', rows),
    onRemoveSelected: (rows) => ui.setBulkRemoveTargets(rows),
  });
  return { columns, batchActions };
}
