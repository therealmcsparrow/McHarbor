// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { RowSelectionState } from '@resources/components/table-features';

type UseDataGridSelectionArgs<T> = {
  data: T[];
  selectable: boolean;
  getRowId?: (row: T) => string;
  isRowDisabled?: (row: T) => boolean;
  onSelectionChange?: (rows: T[]) => void;
};

// useDataGridSelection owns the row-selection state machine for
// DataGrid: keeps the RowSelectionState in sync with the upstream
// data, honours `isRowDisabled` and `getRowId`, notifies the
// parent via `onSelectionChange` whenever the visible selection
// changes, and exposes `selectedRows` so the orchestrator doesn't
// have to re-derive them from TanStack's row model.
export function useDataGridSelection<T>({
  data,
  selectable,
  getRowId,
  isRowDisabled,
  onSelectionChange,
}: UseDataGridSelectionArgs<T>) {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});

  // Keep selection stable across polling refreshes, but drop rows that
  // no longer exist in `data`.
  useEffect(() => {
    if (!selectable) return;

    const currentRowIds = new Set(data.map((row, index) => (getRowId ? getRowId(row) : String(index))));

    setRowSelection((prev) => {
      let changed = false;
      const next: RowSelectionState = {};
      for (const [rowId, selected] of Object.entries(prev)) {
        if (currentRowIds.has(rowId)) {
          next[rowId] = selected;
        } else {
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [data, getRowId, selectable]);

  const handleRowSelectionChange = useCallback(
    (updaterOrValue: RowSelectionState | ((old: RowSelectionState) => RowSelectionState)) => {
      setRowSelection((prev) => {
        const next = typeof updaterOrValue === 'function' ? updaterOrValue(prev) : updaterOrValue;
        if (isRowDisabled) {
          const allowed = new Set(
            data.filter((row) => !isRowDisabled(row)).map((row, index) => (getRowId ? getRowId(row) : String(index))),
          );
          for (const rowId of Object.keys(next)) {
            if (!allowed.has(rowId)) {
              delete (next as RowSelectionState)[rowId];
            }
          }
        }
        return next;
      });
    },
    [data, getRowId, isRowDisabled],
  );

  // Resolve the current selection against the latest `data` so
  // both the parent callback and the orchestrator see the same T[].
  const selectedRows = useMemo(() => {
    if (!selectable) return [] as T[];
    const selectedIds = new Set(
      Object.entries(rowSelection)
        .filter(([, selected]) => selected)
        .map(([id]) => id),
    );
    return data.filter((row) => {
      const id = getRowId ? getRowId(row) : '';
      return selectedIds.has(id);
    });
  }, [rowSelection, data, selectable, getRowId]);

  // Notify parent when selection changes.
  useEffect(() => {
    if (!selectable || !onSelectionChange) return;
    onSelectionChange(selectedRows);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedRows]);

  return {
    rowSelection,
    setRowSelection,
    handleRowSelectionChange,
    selectedRows,
  };
}
