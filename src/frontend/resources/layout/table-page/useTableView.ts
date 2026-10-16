// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import type { ColumnDef, RowData } from '@resources/components/table-features';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import {
  declaredColumns,
  resolveLayout,
  toLayoutColumns,
  type DeclaredColumn,
  type LayoutPayload,
  type ResolvedLayout,
} from './table-layout';
import { useResetLayout, useSaveLayout, useTableViewQuery, type SavedFilter } from './useTableViewApi';
import { useTableViewImport } from './useTableViewImport';
import type { TableColumnWidth, TableDensity } from './types';

export type TableView = {
  available: boolean;
  loading: boolean;
  declared: DeclaredColumn[];
  resolved: ResolvedLayout;
  density: TableDensity;
  pageSize: number;
  autoRefreshMs: number;
  filters: SavedFilter[];
  saveLayout: (
    patch: Partial<Pick<LayoutPayload, 'density' | 'pageSize' | 'autoRefreshMs'>> & { resolved?: ResolvedLayout },
  ) => void;
  resetLayout: () => void;
  setDensity: (next: TableDensity) => void;
  setAutoRefreshMs: (next: number) => void;
  setPageSize: (next: number) => void;
  resizeColumn: (id: string, size: TableColumnWidth) => void;
};

export function useTableView<T extends RowData>(tableId: string, columns: ColumnDef<T, unknown>[]): TableView {
  const query = useTableViewQuery(tableId);
  const save = useSaveLayout(tableId);
  const reset = useResetLayout(tableId);
  const declared = useMemo(() => declaredColumns(columns), [columns]);
  const saved = query.data?.layout?.payload ?? null;
  const resolved = useMemo(() => resolveLayout(declared, saved), [declared, saved]);

  useTableViewImport(tableId, declared, query);

  const density = saved?.density ?? 'default';
  const pageSize = saved?.pageSize ?? tableDefaultPageSize;
  const autoRefreshMs = saved?.autoRefreshMs ?? 0;
  const available = !query.isError;

  const saveLayout = useCallback<TableView['saveLayout']>(
    (patch) => {
      if (!available) return;
      const nextResolved = patch.resolved ?? resolved;
      save.mutate({
        v: 1,
        columns: toLayoutColumns(declared, nextResolved),
        density: patch.density ?? density,
        pageSize: patch.pageSize ?? pageSize,
        autoRefreshMs: patch.autoRefreshMs ?? autoRefreshMs,
      });
    },
    [autoRefreshMs, available, declared, density, pageSize, resolved, save],
  );

  const resizeColumn = useCallback(
    (id: string, size: TableColumnWidth) =>
      saveLayout({ resolved: { ...resolved, sizes: { ...resolved.sizes, [id]: size } } }),
    [resolved, saveLayout],
  );

  return {
    available,
    loading: query.isPending,
    declared,
    resolved,
    density,
    pageSize,
    autoRefreshMs,
    filters: query.data?.filters ?? [],
    saveLayout,
    resetLayout: () => reset.mutate(),
    setDensity: (next) => saveLayout({ density: next }),
    setAutoRefreshMs: (next) => saveLayout({ autoRefreshMs: next }),
    setPageSize: (next) => saveLayout({ pageSize: next }),
    resizeColumn,
  };
}
