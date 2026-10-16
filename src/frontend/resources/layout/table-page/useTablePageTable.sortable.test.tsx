// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ColumnDef } from '@resources/components/table-features';
import { declaredColumns, resolveLayout } from './table-layout';
import { useTablePageTable } from './useTablePageTable';
import type { TableView } from './useTableView';

type Row = { name: string; image: string };

const columns: ColumnDef<Row, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'image', header: 'Image' },
  { id: 'size', header: 'Size', cell: () => null, meta: { sortValue: (row: Row) => row.image.length } },
];

function viewWith(sortable: Record<string, boolean>): TableView {
  const declared = declaredColumns(columns);
  const resolved = resolveLayout(declared, null);
  // unavoidable cast: the hook reads only declared, resolved, pageSize and saveLayout from the view
  return {
    declared,
    resolved: { ...resolved, sortable: { ...resolved.sortable, ...sortable } },
    pageSize: 25,
    saveLayout: vi.fn(),
  } as unknown as TableView;
}

describe('table sorting follows the column setting', () => {
  it('stops a column from sorting when its setting is off and leaves the others sortable', () => {
    const { result } = renderHook(() =>
      useTablePageTable({
        data: [{ name: 'b', image: 'x' }],
        columns,
        view: viewWith({ name: false }),
        serverPaged: false,
        selectable: false,
      }),
    );
    expect(result.current.table.getColumn('name')?.getCanSort()).toBe(false);
    expect(result.current.table.getColumn('image')?.getCanSort()).toBe(true);
  });

  it('settles when the caller re-resolves its layout on every render', () => {
    const data = [
      { name: 'a', image: 'xxx' },
      { name: 'b', image: 'x' },
    ];
    let renders = 0;
    const { result } = renderHook(() => {
      renders += 1;
      // A loop here used to exhaust the worker's memory instead of failing; cap it so it fails as a test.
      if (renders > 50) throw new Error(`useTablePageTable did not settle after ${renders} renders`);
      return useTablePageTable({
        data,
        columns,
        view: viewWith({ size: true }),
        serverPaged: false,
        selectable: false,
      });
    });
    act(() => result.current.table.getColumn('size')?.toggleSorting(false));
    expect(renders).toBeLessThan(10);
  });

  it('sorts a display-only column by its sortValue once it is switched on', () => {
    const data = [
      { name: 'a', image: 'xxx' },
      { name: 'b', image: 'x' },
      { name: 'c', image: 'xx' },
    ];
    const { result } = renderHook(() =>
      useTablePageTable({ data, columns, view: viewWith({ size: true }), serverPaged: false, selectable: false }),
    );
    const size = result.current.table.getColumn('size');
    expect(size?.getCanSort()).toBe(true);
    act(() => size?.toggleSorting(false));
    expect(result.current.table.getRowModel().rows.map((row) => row.original.name)).toEqual(['b', 'c', 'a']);
  });
});
