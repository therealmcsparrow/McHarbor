// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { RowData, ColumnDef, Table } from '@resources/components/table-features';
import { Spinner } from '@resources/components/ui/Spinner';
import { TableSkeleton } from '@resources/components/ui/Skeleton';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Flex } from '@resources/components/ui/Layout';
import { Text as UiText } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';
import { DataGridHeader } from './DataGridHeader';
import { DataGridRow } from './DataGridRow';
import { DataGridGroupHeader } from './DataGridGroupHeader';
import { bucketRows } from './data-grid-groups';
import type { TableDensity, TablePageGrouping, TableResizeProps } from '@resources/layout/table-page/types';

// Rebased onto the audit table's padding: px-4 with py-2.5 as the default rung,
// rather than the tighter px-3/py-1.5 the grid used before.
const densityRowClasses: Record<TableDensity, string> = {
  compact: '[&>tbody>tr>td]:py-1.5 [&>tbody>tr>td]:px-4',
  default: '[&>tbody>tr>td]:py-2.5 [&>tbody>tr>td]:px-4',
  comfortable: '[&>tbody>tr>td]:py-4 [&>tbody>tr>td]:px-4',
};

type DataGridTableProps<T extends RowData> = {
  table: Table<T>;
  columns: ColumnDef<T, unknown>[];
  selectable: boolean;
  loading: boolean;
  loadingSkeleton?: boolean;
  errorState?: unknown;
  onRetry?: () => void;
  totalRows: number;
  density: TableDensity;
  /** Dims the table while a background refetch is in flight. */
  fetching?: boolean;
  tableFixed?: boolean;
  fillHeight?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  emptyIcon?: React.ReactNode;
  emptyMessage?: string;
  onRowClick?: (row: T) => void;
  isRowDisabled?: (row: T) => boolean;
  getRowClassName?: (row: T) => string | undefined;
  getRowId?: (row: T) => string;
  getRowTestId?: (row: T) => string;
  grouping?: TablePageGrouping<T>;
} & TableResizeProps;

// DataGridTable is the actual `<table>` rendered inside the
// bordered card. Extracted so the orchestrator can compose it with
// the toolbar and pagination without bloating past the 200-line
// guideline. The `<thead>` lives in DataGridHeader and single
// rows live in DataGridRow.
export function DataGridTable<T extends RowData>({
  table,
  columns,
  selectable,
  loading,
  loadingSkeleton,
  errorState,
  onRetry,
  totalRows: _totalRows,
  density,
  fetching = false,
  tableFixed,
  fillHeight,
  emptyTitle,
  emptyDescription,
  emptyIcon,
  emptyMessage,
  onRowClick,
  isRowDisabled,
  getRowClassName,
  getRowId,
  getRowTestId,
  grouping,
  columnSizes,
  columnUserSized,
  onColumnResize,
  resizable,
  columnAlign,
}: DataGridTableProps<T>) {
  const { t } = useTranslation('common');
  const totalColSpan = columns.length + (selectable ? 1 : 0);
  const rows = table.getRowModel().rows;
  const isError = errorState !== undefined && errorState !== null;

  const buckets = grouping ? bucketRows(rows, grouping.key) : [];

  return (
    <div
      data-scroll-exempt
      aria-busy={fetching}
      className={cn(
        'w-full min-w-0 rounded-lg border border-border transition-opacity',
        fetching && 'opacity-60',
        fillHeight ? 'min-h-0 flex-1 overflow-auto' : 'overflow-x-auto',
      )}
    >
      <table className={cn('w-full text-sm', tableFixed && 'table-fixed', densityRowClasses[density])}>
        <DataGridHeader
          table={table}
          columns={columns}
          selectable={selectable}
          tableFixed={tableFixed}
          {...{ columnSizes, columnUserSized, onColumnResize, resizable, columnAlign }}
        />
        <tbody>
          {isError ? (
            <tr>
              <td colSpan={totalColSpan} className="p-0">
                <ErrorState
                  variant="banner"
                  error={errorState}
                  onRetry={onRetry}
                  className="rounded-none border-0 bg-transparent"
                />
              </td>
            </tr>
          ) : loading ? (
            loadingSkeleton ? (
              <tr>
                <td colSpan={totalColSpan} className="p-0">
                  <TableSkeleton
                    rows={3}
                    columns={Math.max(columns.length, 4)}
                    density={density}
                    className="rounded-none border-0"
                  />
                </td>
              </tr>
            ) : (
              <tr>
                <td colSpan={totalColSpan} className="px-6 py-12 text-center">
                  <Flex align="center" justify="center" gap={2}>
                    <Spinner size="sm" />
                    <UiText size="sm" tone="muted">
                      {t('dataGrid.loading')}
                    </UiText>
                  </Flex>
                </td>
              </tr>
            )
          ) : rows.length === 0 ? (
            <tr>
              <td colSpan={totalColSpan} className="p-0">
                <EmptyState
                  variant="compact"
                  className="rounded-none border-0 bg-transparent"
                  icon={emptyIcon}
                  title={emptyTitle ?? t('dataGrid.noData')}
                  description={emptyDescription ?? emptyMessage ?? t('dataGrid.noData')}
                />
              </td>
            </tr>
          ) : (
            (grouping ? buckets : [{ key: '', rows }]).flatMap((bucket) => {
              const collapsed = grouping ? grouping.collapsed.includes(bucket.key) : false;
              const body = collapsed
                ? []
                : bucket.rows.map((row) => (
                    <DataGridRow
                      key={row.id}
                      row={row}
                      selectable={selectable}
                      tableFixed={tableFixed}
                      columnSizes={columnSizes}
                      columnUserSized={columnUserSized}
                      onRowClick={onRowClick}
                      isRowDisabled={isRowDisabled}
                      getRowClassName={getRowClassName}
                      getRowId={getRowId}
                      getRowTestId={getRowTestId}
                      columnAlign={columnAlign}
                    />
                  ));
              if (!grouping) return body;
              return [
                <DataGridGroupHeader
                  key={`group:${bucket.key}`}
                  label={grouping.label(bucket.key)}
                  count={bucket.rows.length}
                  collapsed={collapsed}
                  colSpan={totalColSpan}
                  onToggle={() => grouping.onToggle(bucket.key)}
                />,
                ...body,
              ];
            })
          )}
        </tbody>
      </table>
    </div>
  );
}
