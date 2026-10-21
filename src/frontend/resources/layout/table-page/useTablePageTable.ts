// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The TanStack table instance behind TablePage: sorting, column visibility and
// order bound to the user's saved table view, row selection, and the one
// branch that matters — a server-paged list must not paginate again, or the
// grid would slice the page the API already sliced.

import { useEffect, useMemo, useState } from 'react';
import {
  appTableFeatures,
  useTable,
  type ColumnDef,
  type RowData,
  type SortingState,
} from '@resources/components/table-features';
import { useDataGridSelection } from '@resources/components/useDataGridSelection';
import type { TableView } from './useTableView';
import type { TableColumnMeta } from './types';

// A column that only renders cells has nothing to sort by; give it its meta.sortValue, or the row's field of the same
// name, so the sortable setting means something on every column.
function withSortValue<T extends RowData>(column: ColumnDef<T, unknown>, id: string): ColumnDef<T, unknown> {
  const def = column as { accessorKey?: unknown; accessorFn?: unknown };
  if (typeof def.accessorKey === 'string' || typeof def.accessorFn === 'function') return column;
  const sortValue = (column.meta as TableColumnMeta | undefined)?.sortValue as ((row: T) => unknown) | undefined;
  return {
    ...column,
    accessorFn: (row: T) => (sortValue ? sortValue(row) : (row as Record<string, unknown>)?.[id]),
    sortUndefined: 'last',
  } as ColumnDef<T, unknown>;
}

function shallowEqual(a: object | undefined, b: object | undefined): boolean {
  if (a === b) return true;
  if (!a || !b) return false;
  const aKeys = Object.keys(a);
  if (aKeys.length !== Object.keys(b).length) return false;
  return aKeys.every((key) => (a as Record<string, unknown>)[key] === (b as Record<string, unknown>)[key]);
}

// A caller whose columns are not memoised hands over a freshly resolved layout on every render. When the
// sortable map and the table state both change identity each render, TanStack's store re-renders the
// hook from inside render and never settles, so the layout pieces are held stable by value.
function useShallowStable<V extends object | undefined>(value: V): V {
  const [stable, setStable] = useState(value);
  const same = shallowEqual(stable, value);
  if (!same) setStable(value);
  return same ? stable : value;
}

type UseTablePageTableArgs<T extends RowData> = {
  data: T[];
  columns: ColumnDef<T, unknown>[];
  view: TableView;
  serverPaged: boolean;
  selectable: boolean;
  getRowId?: (row: T) => string;
  isRowDisabled?: (row: T) => boolean;
};

export function useTablePageTable<T extends RowData>({
  data,
  columns,
  view,
  serverPaged,
  selectable,
  getRowId,
  isRowDisabled,
}: UseTablePageTableArgs<T>) {
  const [sorting, setSorting] = useState<SortingState>([]);

  const sortableById = useShallowStable(view.resolved.sortable);
  // The saved layout decides which columns can be sorted; a column with no saved choice keeps its definition's.
  const sortableColumns = useMemo(
    () =>
      columns.map((column) => {
        const id = column.id ?? (column as { accessorKey?: unknown }).accessorKey;
        if (typeof id !== 'string' || !sortableById || !(id in sortableById)) return column;
        return { ...withSortValue(column, id), enableSorting: sortableById[id] };
      }),
    [columns, sortableById],
  );
  useEffect(() => {
    setSorting((current) => {
      const kept = current.filter((entry) => sortableById?.[entry.id] !== false);
      return kept.length === current.length ? current : kept;
    });
  }, [sortableById]);

  const columnVisibility = useShallowStable(view.resolved.visibility);
  const columnOrder = useShallowStable(view.resolved.order);

  const selection = useDataGridSelection<T>({ data, selectable, getRowId, isRowDisabled });

  const table = useTable({
    features: appTableFeatures,
    data,
    columns: sortableColumns,
    state: {
      sorting,
      columnVisibility,
      columnOrder,
      ...(selectable ? { rowSelection: selection.rowSelection } : {}),
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: (updater) => {
      const next = typeof updater === 'function' ? updater(columnVisibility) : updater;
      view.saveLayout({ resolved: { ...view.resolved, visibility: { ...columnVisibility, ...next } } });
    },
    // The whole point of the branch: a server-paged list is handed one page of
    // rows that the API already sliced, so the table must not slice them again.
    // v8 achieved that by withholding the pagination row model; v9 registers row
    // models statically, so it is `manualPagination` that turns the slicing off
    // while leaving the pagination APIs the footer reads intact.
    manualPagination: serverPaged,
    ...(selectable
      ? {
          enableRowSelection: true,
          onRowSelectionChange: selection.handleRowSelectionChange,
          ...(getRowId ? { getRowId: (row: T) => getRowId(row) } : {}),
        }
      : {}),
    // v9 types `initialState.pagination` as a whole `PaginationState`, so the
    // starting page index has to be spelled out rather than defaulted.
    ...(serverPaged ? {} : { initialState: { pagination: { pageIndex: 0, pageSize: view.pageSize } } }),
  });

  const savedPageSize = view.pageSize;
  const currentPageSize = table.state.pagination.pageSize;
  useEffect(() => {
    if (!serverPaged && currentPageSize !== savedPageSize) {
      table.setPageSize(savedPageSize);
    }
    // oxlint-disable-next-line react/exhaustive-deps -- syncs the saved page size when it or serverPaged changes; table and currentPageSize are deliberately not triggers, a user page change must not be reverted
  }, [savedPageSize, serverPaged]);

  return { table, selection };
}
