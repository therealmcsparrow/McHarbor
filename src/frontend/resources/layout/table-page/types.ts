// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType, ReactNode } from 'react';
import type { RowData, ColumnDef } from '@resources/components/table-features';
import type { SearchMode } from '@resources/utils/search-filter';
import type { FilterFacet } from '@resources/components/FilterChips';

/**
 * A bulk action offered in the selection bar. Lived in `DataGrid` until that
 * shim was deleted; it is part of the table layout's contract, not the grid's.
 */
export type BatchAction = {
  label: string;
  icon?: ComponentType<{ className?: string }>;
  iconClassName?: string;
  variant?: 'default' | 'destructive';
  onClick: (selectedRows: unknown[]) => void;
  confirm?: boolean;
  permission?: string | string[];
};

export type TableDensity = 'compact' | 'default' | 'comfortable';

export const TABLE_DENSITIES: readonly TableDensity[] = ['compact', 'default', 'comfortable'];

/**
 * Column widths come from a fixed token set rather than a runtime pixel number,
 * because Tailwind arbitrary values resolve at build time and an inline `style`
 * is banned by the React standards.
 */
export type TableColumnWidth = 'auto' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl';

export const TABLE_COLUMN_WIDTHS: readonly TableColumnWidth[] = ['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl'];

export type TableColumnAlign = 'auto' | 'left' | 'center' | 'right';

export type TableColumnMeta = {
  width?: TableColumnWidth;
  /** Listed in the columns menu but not toggleable — actions, selection. */
  locked?: boolean;
  /** Excluded from CSV/JSON export — an actions column renders only buttons. */
  noExport?: boolean;
  defaultVisible?: boolean;
  group?: string;
  label?: string;
  wrap?: boolean;
  /**
   * The value a column that only renders cells is sorted (and exported) by. Without it the row's own field of the
   * same name is used, which is wrong when the cell shows something derived — uptime from a start time, say.
   */
  sortValue?: (row: never) => string | number | boolean | null | undefined;
};

export type TableColumnAlignMap = {
  header: Record<string, TableColumnAlign>;
  cell: Record<string, TableColumnAlign>;
};

export type TableResizeProps = {
  columnSizes?: Record<string, TableColumnWidth>;
  columnUserSized?: ReadonlySet<string>;
  onColumnResize?: (id: string, size: TableColumnWidth) => void;
  resizable?: boolean;
  columnAlign?: TableColumnAlignMap;
};

export type ActiveFilterChip = {
  id: string;
  /** Already translated. */
  label: string;
  onRemove: () => void;
};

/** Auto-refresh intervals, in milliseconds. 0 means off. */
export const AUTO_REFRESH_OPTIONS: readonly number[] = [0, 5_000, 15_000, 30_000, 60_000];

export type TablePagePagination =
  | { mode: 'client'; pageSizeOptions?: readonly number[] }
  | {
      mode: 'server';
      page: number;
      totalPages: number;
      pageSize: number;
      total: number;
      from: number;
      to: number;
      onPageChange: (page: number) => void;
      onPageSizeChange: (pageSize: number) => void;
      pageSizeOptions?: readonly number[];
      /** Extra content above the "showing" line — e.g. an unread count. */
      leading?: ReactNode;
    }
  | { mode: 'none' };

/** What the filter bar itself renders — no row type involved. */
export type TableSearchControl = {
  value: string;
  onChange: (value: string) => void;
  mode: SearchMode;
  onModeChange: (mode: SearchMode) => void;
  placeholder: string;
  /** True when the regex in `value` failed to compile. */
  error?: boolean;
};

export type TablePageSearch<T extends RowData> = TableSearchControl & {
  /**
   * The fields this list searches. When provided, TablePage filters `data`
   * itself through `matchesSearchFields`, so all three match modes work.
   *
   * Omit it when the page needs filtering the layout cannot express — a
   * structured query parser, a facet that composes with the text, a filter that
   * has to run before a server call — and narrow `data` before passing it.
   */
  fields?: (row: T) => (string | null | undefined)[];
};

export type TablePageSavedFilters = {
  state: Record<string, unknown>;
  onRestore: (state: Record<string, unknown>) => void;
  pristine?: boolean;
  defaults?: Record<string, unknown>;
};

export type TablePageToolbar = {
  columns?: boolean;
  density?: boolean;
  refresh?: { onRefresh: () => void; isFetching: boolean };
  export?: {
    /** Basename without extension. */
    filename: string;
    /** When set, the button calls this instead of exporting client-side. */
    server?: () => void;
  };
  chips?: { items: ActiveFilterChip[]; onClear: () => void };
  actions?: ReactNode;
};

export type TablePageGrouping<T extends RowData> = {
  key: (row: T) => string;
  label: (key: string) => string;
  collapsed: readonly string[];
  onToggle: (key: string) => void;
};

export type TablePageProps<T extends RowData> = {
  /** Namespaces preferences and saved filters in localStorage. Stable per page. */
  id: string;
  /** ReactNode, not string: some pages put a back button or icon in the title. */
  title: ReactNode;
  description?: string;
  actions?: ReactNode;
  /**
   * Set false inside a tab or panel that already has its own heading, so the
   * layout does not draw an empty header band above the filters.
   */
  header?: boolean;

  /**
   * Rendered between the header and the filter band — a DegradedStateBanner or
   * similar page-level notice that must sit above the filters, not below the
   * table.
   */
  banner?: ReactNode;

  search?: TablePageSearch<T>;
  savedFilters?: TablePageSavedFilters;
  filters?: ReactNode;
  facets?: {
    items: readonly FilterFacet<string>[];
    value: string;
    onChange: (next: string) => void;
  };
  toolbar?: TablePageToolbar;

  columns: ColumnDef<T, unknown>[];
  data: T[];
  loading?: boolean;
  error?: unknown;
  onRetry?: () => void;
  empty?: { icon?: ReactNode; title: string; description?: string };

  grouping?: TablePageGrouping<T>;

  selectable?: boolean;
  batchActions?: BatchAction[];
  getRowId?: (row: T) => string;
  /** Emitted as `data-testid` on each row, for existing test selectors. */
  getRowTestId?: (row: T) => string;
  getRowClassName?: (row: T) => string | undefined;
  isRowDisabled?: (row: T) => boolean;
  onRowClick?: (row: T) => void;

  /**
   * Replaces the table region. For a page that offers an alternate view of the
   * same rows — a card grid, a map — so the header, filter band, toolbar and
   * pager stay identical across views instead of being rebuilt per view.
   *
   * Take the function form. The pager is rendered outside this region and drives
   * the table's page state either way, so a plain node renders every row and the
   * pager then appears to do nothing; the function is handed the current page's
   * rows, already filtered and sliced.
   */
  body?: ReactNode | ((rows: T[]) => ReactNode);

  pagination?: TablePagePagination;
  /**
   * Rendered below the table, under the pager. For a list that pages by
   * scrolling rather than by page number — an infinite-scroll sentinel and its
   * "loading more" line.
   */
  footer?: ReactNode;
};
