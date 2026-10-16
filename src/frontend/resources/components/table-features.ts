// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The single TanStack Table v9 feature registry for the whole app.
//
// v9 made features opt-in: an API is missing because its feature was never
// registered, not because the library dropped it. Registering once, here, keeps
// that decision in one file instead of scattered across two table hooks.
//
// It has to be one shared registry rather than one per hook, because `TFeatures`
// is part of every table type. Column arrays are written once and handed to both
// DataGrid and TablePage, so if the two tables registered different features
// their `ColumnDef` types would stop being interchangeable and every column file
// would have to pick a side.
//
// The re-exported type aliases below take the same parameters the v8 types did
// (`ColumnDef<Row, unknown>`, `Table<Row>`), so call sites read exactly as
// before and only their import path changed.

import {
  columnFilteringFeature,
  columnOrderingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  globalFilteringFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_datetime,
  sortFn_text,
  tableFeatures,
  type ColumnDef as TanStackColumnDef,
  type FilterFn as TanStackFilterFn,
  type ReactTable,
  type Row as TanStackRow,
  type RowData,
} from '@tanstack/react-table';

// Feature order matters to `tableFeatures()`: a row-model slot has to come after
// the feature it builds on. `globalFilteringFeature` likewise requires
// `columnFilteringFeature`, and both share the one `filteredRowModel`.
//
// `sortFns` is not decoration. Auto sorting resolves the names 'datetime',
// 'alphanumeric' and 'text' out of this registry and warns, then silently falls
// back to a basic comparison, when one is absent — so these three are exactly
// what v8's built-in auto sorting used. Registering the whole `sortFns` object
// instead would pull every built-in comparator into the bundle.
// `columnSizingFeature` is registered for its column-def options only: ~90
// `size`/`minSize`/`maxSize` declarations across the column files are part of
// the `ColumnDef` type it contributes. Nothing calls `getSize()` — widths come
// from the fixed token set in DataGridHeader — so this buys type validity, not
// behaviour. Interactive resizing is token-based instead, dragged through
// `TableColumnResizeHandle` onto the nearest `TableColumnWidth`, so
// `columnResizingFeature` stays out: v9 split interactive resizing into its
// own feature and nothing here needs pixel-level resize state.
export const appTableFeatures = tableFeatures({
  columnFilteringFeature,
  columnOrderingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  globalFilteringFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  filteredRowModel: createFilteredRowModel(),
  paginatedRowModel: createPaginatedRowModel(),
  sortedRowModel: createSortedRowModel(),
  filterFns: { includesString: filterFn_includesString },
  sortFns: {
    alphanumeric: sortFn_alphanumeric,
    datetime: sortFn_datetime,
    text: sortFn_text,
  },
});

export type AppTableFeatures = typeof appTableFeatures;

export type ColumnDef<TData extends RowData, TValue = unknown> = TanStackColumnDef<AppTableFeatures, TData, TValue>;

// Deliberately the React `ReactTable`, not table-core's `Table`. Every table in
// the app is the return value of `useTable`, and v9 puts the render-time reads
// (`table.state`, `table.Subscribe`) on the React type only — a core `Table`
// would type-check at the prop boundary and then fail on `table.state`.
export type Table<TData extends RowData> = ReactTable<AppTableFeatures, TData>;

export type Row<TData extends RowData> = TanStackRow<AppTableFeatures, TData>;

export type FilterFn<TData extends RowData> = TanStackFilterFn<AppTableFeatures, TData>;

export { flexRender, useTable } from '@tanstack/react-table';

export type { ColumnVisibilityState, RowData, RowSelectionState, SortingState } from '@tanstack/react-table';
