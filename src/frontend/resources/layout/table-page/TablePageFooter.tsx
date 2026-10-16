// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// TablePageFooter — one pager, two sources of truth.
//
// A client-paged list gets its numbers from TanStack, which is holding every
// row. A server-paged list gets them from the page's own query state, because
// that is the only place the real total lives. Both feed the same pager, which
// is why audit and notifications no longer each hand-roll one.

import type { RowData, Table } from '@resources/components/table-features';
import { TablePagination, tablePageSizeOptions } from '@resources/components/TablePagination';
import type { TablePagePagination } from './types';

type TablePageFooterProps<T extends RowData> = {
  pagination: TablePagePagination;
  table: Table<T>;
  rowCount: number;
  onPageSizeStored: (pageSize: number) => void;
};

export function TablePageFooter<T extends RowData>({
  pagination,
  table,
  rowCount,
  onPageSizeStored,
}: TablePageFooterProps<T>) {
  if (pagination.mode === 'none') {
    return null;
  }

  if (pagination.mode === 'server') {
    return (
      <TablePagination
        page={pagination.page}
        totalPages={pagination.totalPages}
        pageSize={pagination.pageSize}
        from={pagination.from}
        to={pagination.to}
        total={pagination.total}
        pageSizeOptions={pagination.pageSizeOptions ?? tablePageSizeOptions}
        leading={pagination.leading}
        onPageChange={pagination.onPageChange}
        onPageSizeChange={(next) => {
          onPageSizeStored(next);
          pagination.onPageSizeChange(next);
        }}
      />
    );
  }

  const state = table.state.pagination;

  return (
    <TablePagination
      page={state.pageIndex + 1}
      totalPages={table.getPageCount()}
      pageSize={state.pageSize}
      pageSizeOptions={pagination.pageSizeOptions ?? tablePageSizeOptions}
      from={rowCount === 0 ? 0 : state.pageIndex * state.pageSize + 1}
      to={Math.min((state.pageIndex + 1) * state.pageSize, rowCount)}
      total={rowCount}
      onPageChange={(next) => table.setPageIndex(next - 1)}
      onPageSizeChange={(next) => {
        onPageSizeStored(next);
        table.setPageSize(next);
        table.setPageIndex(0);
      }}
    />
  );
}
