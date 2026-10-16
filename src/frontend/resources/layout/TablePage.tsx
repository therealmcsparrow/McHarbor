// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// TablePage — the layout every tabled page uses.
//
//   PageHeader        title · description · actions
//   TableFilterBar    row 1: search · mode · saved filters
//                     row 2: filters + facets + chips  |  TableToolbar
//   DataGridTable     the only scroller on the page
//   TablePagination   client- or server-fed
//
// Before this, a list page picked between two table systems — the TanStack
// `DataGrid` or the audit page's hand-rolled table — and re-implemented
// whichever chrome the other one had. See
// docs/superpowers/specs/2026-08-27-table-page-layout-design.md.
//
// This renders the search field but does NOT filter: the app's search supports
// contains / exact / regex via `matchesSearchFields` while TanStack's global
// filter only does contains. Pages filter their own `data` before passing it.

import { useMemo } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import type { RowData } from '@resources/components/table-features';
import { matchesSearchFields } from '@resources/utils/search-filter';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGridTable } from '@resources/components/DataGridTable';
import { FilterChips } from '@resources/components/FilterChips';
import { TableFilterBar } from './table-page/TableFilterBar';
import { TableToolbar } from './table-page/TableToolbar';
import { TableFilterChips } from './table-page/TableFilterChips';
import { TableBulkActions } from './table-page/TableBulkActions';
import { TablePageFooter } from './table-page/TablePageFooter';
import { useTableView } from './table-page/useTableView';
import { useTablePageTable } from './table-page/useTablePageTable';
import { useTableSavedFilters } from './table-page/useTableSavedFilters';
import { useSearchSavedFilters } from './table-page/useSearchSavedFilters';
import type { TablePageProps } from './table-page/types';
import { usePageActionsSlot } from './PageActionsSlot';

export function TablePage<T extends RowData>({
  id,
  title,
  description,
  actions,
  header = true,
  banner,
  search,
  savedFilters,
  filters,
  facets,
  toolbar: toolbarNode,
  columns,
  data,
  loading = false,
  error,
  onRetry,
  empty,
  grouping,
  selectable = false,
  batchActions,
  getRowId,
  getRowTestId,
  getRowClassName,
  isRowDisabled,
  onRowClick,
  body,
  pagination = { mode: 'client' },
  footer,
}: TablePageProps<T>) {
  const { t } = useTranslation('common');
  const view = useTableView(id, columns);
  const serverPaged = pagination.mode === 'server';

  // A page that declares its searchable fields gets filtering here, with all
  // three match modes. One that does not filters its own `data` first — see
  // the note in TablePageSearch.
  const searchFields = search?.fields;
  const searchValue = search?.value ?? '';
  const searchMode = search?.mode;
  const rows = useMemo(() => {
    if (!searchFields || !searchMode || !searchValue.trim()) return data;
    return data.filter((row) => matchesSearchFields(searchValue, searchMode, searchFields(row)).matched);
  }, [data, searchFields, searchMode, searchValue]);

  const { table, selection } = useTablePageTable({
    data: rows,
    columns,
    view,
    serverPaged,
    selectable,
    getRowId,
    isRowDisabled,
  });

  const searchSaved = useSearchSavedFilters(search);
  const filtersConfig = savedFilters ?? searchSaved;
  const saved = useTableSavedFilters(id, filtersConfig);
  const filteredRows = table.getFilteredRowModel().rows;
  const rowCount = filteredRows.length;
  const selectableRows = isRowDisabled ? filteredRows.filter((row) => !isRowDisabled(row.original)) : filteredRows;

  const secondRow =
    filters || facets || toolbarNode?.chips ? (
      <>
        {filters}
        {facets && <FilterChips facets={facets.items} value={facets.value} onChange={facets.onChange} />}
        {toolbarNode?.chips && <TableFilterChips items={toolbarNode.chips.items} matchCount={rowCount} />}
      </>
    ) : undefined;

  // Page actions belong beside the title, not in the table toolbar.
  //
  // The toolbar is about the table — which columns, how dense, how fresh, how
  // to export it — so a "Create network" button sitting in it reads as an
  // operation on the rows rather than on the page. They are hoisted here and
  // placed BEFORE `actions`, which is where the view-mode toggle lives, so the
  // order is [create] [import] [prune] | [table/card toggle].
  //
  // A headerless table (inside a tab) has nowhere to hoist them to, so there
  // they stay in the toolbar exactly as before.
  const headerActions = header ? (
    <>
      {toolbarNode?.actions}
      {toolbarNode?.actions && actions && <div aria-hidden="true" className="mx-0.5 h-5 w-px bg-border" />}
      {actions}
    </>
  ) : (
    actions
  );
  const actionsSlot = usePageActionsSlot();
  const hoistToSlot = !header && actionsSlot !== null && Boolean(toolbarNode?.actions);
  const toolbarConfig = (header || hoistToSlot) && toolbarNode ? { ...toolbarNode, actions: undefined } : toolbarNode;

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      {header && <PageHeader title={title} description={description} actions={headerActions} headingLeaf />}
      {hoistToSlot && actionsSlot && createPortal(toolbarNode?.actions, actionsSlot)}
      {banner}

      <TableFilterBar
        search={search}
        savedFilters={filtersConfig ? saved : undefined}
        onClearFilters={
          filtersConfig
            ? () => {
                saved.clear();
                toolbarNode?.chips?.onClear?.();
              }
            : undefined
        }
        filters={secondRow}
        toolbar={
          toolbarConfig && <TableToolbar config={toolbarConfig} table={table} view={view} serverPaged={serverPaged} />
        }
        selection={
          selectable && batchActions
            ? {
                count: selection.selectedRows.length,
                total: rowCount,
                selectable: selectableRows.length,
                onSelectAll: serverPaged
                  ? undefined
                  : () => selection.setRowSelection(Object.fromEntries(selectableRows.map((row) => [row.id, true]))),
                onClear: () => selection.setRowSelection({}),
                actions: (
                  <TableBulkActions
                    actions={batchActions}
                    selectedRows={selection.selectedRows}
                    onComplete={() => selection.setRowSelection({})}
                  />
                ),
              }
            : undefined
        }
      />

      {(typeof body === 'function' ? body(table.getRowModel().rows.map((row) => row.original)) : body) ?? (
        <DataGridTable
          table={table}
          columns={columns}
          selectable={selectable}
          loading={loading}
          loadingSkeleton
          fetching={toolbarNode?.refresh?.isFetching ?? false}
          errorState={error}
          onRetry={onRetry}
          totalRows={rowCount}
          density={view.density}
          grouping={grouping}
          fillHeight
          emptyIcon={empty?.icon}
          emptyTitle={empty?.title ?? t('dataGrid.noData')}
          emptyDescription={empty?.description}
          onRowClick={onRowClick}
          isRowDisabled={isRowDisabled}
          getRowClassName={getRowClassName}
          getRowId={getRowId}
          getRowTestId={getRowTestId}
          columnSizes={view.resolved.sizes}
          columnUserSized={view.resolved.userSized}
          onColumnResize={view.resizeColumn}
          resizable={view.available}
          columnAlign={{ header: view.resolved.headerAlign, cell: view.resolved.cellAlign }}
        />
      )}

      <TablePageFooter pagination={pagination} table={table} rowCount={rowCount} onPageSizeStored={view.setPageSize} />

      {footer}
    </div>
  );
}
