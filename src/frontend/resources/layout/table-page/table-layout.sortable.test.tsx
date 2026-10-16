// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ColumnDef } from '@resources/components/table-features';
import { renderWithProviders } from '../../../test/render';
import { declaredColumns, resolveLayout, toLayoutColumns, type LayoutPayload } from './table-layout';
import { TableEditColumnSettings } from './TableEditColumnSettings';

type Row = { name: string; image: string };

const columns: ColumnDef<Row, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'image', header: 'Image', enableSorting: false },
  { id: 'status', header: 'Status', cell: () => null },
  { id: 'uptime', header: 'Uptime', cell: () => null, meta: { sortValue: (row: Row) => row.name.length } },
  { id: 'actions', accessorFn: (row) => row.name, header: '' },
];

function payload(cols: LayoutPayload['columns']): LayoutPayload {
  return { v: 1, columns: cols, density: 'default', pageSize: 25, autoRefreshMs: 0 };
}

describe('column sortable setting', () => {
  it('offers the choice on every column but actions; a display-only column starts unsorted', () => {
    const byId = Object.fromEntries(declaredColumns(columns).map((c) => [c.id, c]));
    expect(byId.name).toMatchObject({ canSort: true, defaultSortable: true });
    expect(byId.image).toMatchObject({ canSort: true, defaultSortable: false });
    expect(byId.status).toMatchObject({ canSort: true, defaultSortable: false });
    expect(byId.uptime).toMatchObject({ canSort: true, defaultSortable: true });
    expect(byId.actions).toMatchObject({ canSort: false, defaultSortable: false });
  });

  it('applies a saved choice and ignores one stored for the actions column', () => {
    const declared = declaredColumns(columns);
    const resolved = resolveLayout(
      declared,
      payload([
        { id: 'name', visible: true, size: 'auto', sortable: false },
        { id: 'image', visible: true, size: 'auto', sortable: true },
        { id: 'actions', visible: true, size: 'auto', sortable: true },
      ]),
    );
    expect(resolved.sortable).toMatchObject({ name: false, image: true, actions: false });
  });

  it('stores only a choice that differs from the column default', () => {
    const declared = declaredColumns(columns);
    const resolved = resolveLayout(declared, null);
    resolved.sortable = { ...resolved.sortable, name: false, image: false };
    const saved = Object.fromEntries(toLayoutColumns(declared, resolved).map((c) => [c.id, c.sortable]));
    expect(saved).toMatchObject({
      name: false,
      image: undefined,
      status: undefined,
      uptime: undefined,
      actions: undefined,
    });
  });

  it('shows the switch only when the column can sort, and reports a change', () => {
    const onSortable = vi.fn();
    const props = {
      id: 'name',
      label: 'Name',
      size: 'auto' as const,
      headerAlign: 'auto' as const,
      cellAlign: 'auto' as const,
      onResize: vi.fn(),
      onAlign: vi.fn(),
      onSortable,
    };
    const { unmount } = renderWithProviders(<TableEditColumnSettings {...props} sortable />);
    fireEvent.click(screen.getByRole('button', { name: /Name/ }));
    fireEvent.click(screen.getByRole('switch', { name: 'Sortable' }));
    expect(onSortable).toHaveBeenCalledWith('name', false);
    unmount();

    renderWithProviders(<TableEditColumnSettings {...props} id="actions" label="Actions" sortable={undefined} />);
    fireEvent.click(screen.getByRole('button', { name: /Actions/ }));
    expect(screen.queryByRole('switch', { name: 'Sortable' })).not.toBeInTheDocument();
  });
});
