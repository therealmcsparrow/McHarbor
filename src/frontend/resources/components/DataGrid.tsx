// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getPaginationRowModel,
  getFilteredRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
  type RowSelectionState,
  type FilterFn,
} from '@tanstack/react-table';
import { useState, useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowsSort, IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Spinner } from '@resources/components/ui/Spinner';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const safeGlobalFilter: FilterFn<any> = (row, _columnId, filterValue) => {
  const search = String(filterValue).toLowerCase();
  if (!search) return true;

  const values = row.getAllCells().map((cell) => {
    const val = cell.getValue();
    if (val == null) return '';
    if (Array.isArray(val)) return val.join(' ');
    if (typeof val === 'object') return JSON.stringify(val);
    return String(val);
  });

  return values.some((v) => v.toLowerCase().includes(search));
};

export type BatchAction = {
  label: string;
  icon?: React.ComponentType<{ className?: string }>;
  variant?: 'default' | 'destructive';
  onClick: (selectedRows: unknown[]) => void;
  confirm?: boolean;
};

type DataGridProps<T> = {
  data: T[];
  columns: ColumnDef<T, unknown>[];
  searchKey?: string;
  searchPlaceholder?: string;
  pageSize?: number;
  onRowClick?: (row: T) => void;
  loading?: boolean;
  emptyMessage?: string;
  tableFixed?: boolean;
  selectable?: boolean;
  onSelectionChange?: (rows: T[]) => void;
  batchActions?: BatchAction[];
  getRowId?: (row: T) => string;
  getRowClassName?: (row: T) => string | undefined;
};

export function DataGrid<T>({
  data,
  columns,
  searchKey,
  searchPlaceholder,
  pageSize = 25,
  onRowClick,
  loading = false,
  emptyMessage,
  tableFixed = false,
  selectable = false,
  onSelectionChange,
  batchActions,
  getRowId,
  getRowClassName,
}: DataGridProps<T>) {
  const { t } = useTranslation('common');
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState('');
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [confirmAction, setConfirmAction] = useState<BatchAction | null>(null);

  const resolvedSearchPlaceholder = searchPlaceholder ?? t('dataGrid.search');
  const resolvedEmptyMessage = emptyMessage ?? t('dataGrid.noData');

  // Keep selection stable across polling refreshes, but drop rows that no longer exist.
  useEffect(() => {
    if (!selectable) return;

    const currentRowIds = new Set(
      data.map((row, index) => (getRowId ? getRowId(row) : String(index)))
    );

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
        return next;
      });
    },
    [],
  );

  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter, ...(selectable ? { rowSelection } : {}) },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: safeGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    ...(selectable
      ? {
          enableRowSelection: true,
          onRowSelectionChange: handleRowSelectionChange,
          ...(getRowId ? { getRowId: (row: T) => getRowId(row) } : {}),
        }
      : {}),
    initialState: { pagination: { pageSize } },
  });

  const selectedRows = selectable
    ? table.getSelectedRowModel().rows.map((r) => r.original)
    : [];
  const selectedCount = selectedRows.length;

  // Notify parent of selection changes
  useEffect(() => {
    if (selectable && onSelectionChange) {
      onSelectionChange(selectedRows);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rowSelection]);

  const totalColSpan = columns.length + (selectable ? 1 : 0);

  return (
    <div className="space-y-4">
      {/* Search + Batch Toolbar */}
      <div className="flex items-center gap-3">
        {searchKey && (
          <input
            type="text"
            value={globalFilter}
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder={resolvedSearchPlaceholder}
            className="py-1s px-2 block w-full max-w-sm bg-card border border-border rounded-lg text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
          />
        )}
        {selectable && selectedCount > 0 && batchActions && batchActions.length > 0 && (
          <div className="ml-auto flex items-center gap-2">
            <span className="text-xs font-medium text-muted-foreground">
              {t('batch.selected', { count: selectedCount })}
            </span>
            {batchActions.map((action) => {
              const Icon = action.icon;
              return (
                <Button
                  key={action.label}
                  variant={action.variant === 'destructive' ? 'destructive' : 'outline'}
                  size="sm"
                  onClick={() => {
                    if (action.confirm) {
                      setConfirmAction(action);
                    } else {
                      action.onClick(selectedRows);
                    }
                  }}
                >
                  {Icon && <Icon className="h-3.5 w-3.5" />}
                  {action.label}
                </Button>
              );
            })}
          </div>
        )}
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-xl border border-border">
        <table className={cn('w-full divide-y divide-border', tableFixed && 'table-fixed')}>
          <thead className="bg-muted">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id}>
                {selectable && (
                  <th className="w-10 px-2 py-3">
                    <Checkbox
                      checked={
                        table.getIsAllPageRowsSelected()
                          ? true
                          : table.getIsSomePageRowsSelected()
                            ? 'indeterminate'
                            : false
                      }
                      onCheckedChange={(checked) =>
                        table.toggleAllPageRowsSelected(!!checked)
                      }
                      aria-label="Select all"
                    />
                  </th>
                )}
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    className={cn(
                      'px-2 py-3 text-start text-xs font-medium text-muted-foreground uppercase tracking-wider',
                      header.column.getCanSort() && 'cursor-pointer select-none hover:text-foreground'
                    )}
                    style={tableFixed && !(header.column.columnDef.meta as Record<string, unknown>)?.flex ? { width: header.getSize() } : undefined}
                    onClick={header.column.getToggleSortingHandler()}
                  >
                    <div className="flex items-center gap-1">
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {header.column.getCanSort() && (
                        <IconArrowsSort className="h-3 w-3" />
                      )}
                    </div>
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody className="divide-y divide-border">
            {loading ? (
              <tr>
                <td colSpan={totalColSpan} className="px-6 py-12 text-center text-sm text-muted-foreground">
                  <div className="flex items-center justify-center gap-2">
                    <Spinner size="sm" />
                    {t('dataGrid.loading')}
                  </div>
                </td>
              </tr>
            ) : table.getRowModel().rows.length === 0 ? (
              <tr>
                <td colSpan={totalColSpan} className="px-6 py-12 text-center text-sm text-muted-foreground">
                  {resolvedEmptyMessage}
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className={cn(
                    'transition-colors hover:bg-muted/50',
                    onRowClick && 'cursor-pointer',
                    selectable && row.getIsSelected() && 'bg-primary/5',
                    getRowClassName?.(row.original)
                  )}
                  onClick={() => onRowClick?.(row.original)}
                >
                  {selectable && (
                    <td className="w-10 px-2 py-1" onClick={(e) => e.stopPropagation()}>
                      <Checkbox
                        checked={row.getIsSelected()}
                        onCheckedChange={(checked) => row.toggleSelected(!!checked)}
                        aria-label="Select row"
                      />
                    </td>
                  )}
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className={cn('px-2 py-1 text-sm text-foreground', tableFixed ? 'overflow-hidden' : 'break-words')}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Batch Confirm Dialog */}
      <ConfirmDialog
        open={confirmAction !== null}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        title={t('batch.confirmTitle')}
        description={t('batch.confirmDescription', {
          action: confirmAction?.label?.toLowerCase() ?? '',
          count: selectedCount,
        })}
        onConfirm={() => {
          if (confirmAction) {
            confirmAction.onClick(selectedRows);
            setConfirmAction(null);
            setRowSelection({});
          }
        }}
      />

      {/* Pagination */}
      {table.getPageCount() > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            {t('dataGrid.showing', {
              from: table.getState().pagination.pageIndex * table.getState().pagination.pageSize + 1,
              to: Math.min(
                (table.getState().pagination.pageIndex + 1) * table.getState().pagination.pageSize,
                data.length
              ),
              total: data.length,
            })}
          </p>
          <div className="flex items-center gap-x-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              <IconChevronLeft className="h-4 w-4" />
              {t('dataGrid.prev')}
            </Button>
            <span className="text-sm text-muted-foreground">
              {t('dataGrid.page', { current: table.getState().pagination.pageIndex + 1, total: table.getPageCount() })}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              {t('dataGrid.next')}
              <IconChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

