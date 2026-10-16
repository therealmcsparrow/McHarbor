// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ColumnDef } from '@resources/components/table-features';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import type { LayoutPayload } from './table-layout';
import { useTableView } from './useTableView';
import { useCreateFilter, useDeleteFilter, type SavedFilter } from './useTableViewApi';
import { createTestQueryClient } from '../../../test/render';
import { server } from '../../../test/server';

type Row = { name: string; image: string; status: string };

const columns: ColumnDef<Row, unknown>[] = [
  { id: 'name' },
  { id: 'image', meta: { defaultVisible: false } },
  { id: 'status' },
];

const savedLayout: LayoutPayload = {
  v: 1,
  columns: [
    { id: 'status', visible: true, size: 'auto' },
    { id: 'name', visible: true, size: 'auto' },
    { id: 'image', visible: true, size: 'auto' },
  ],
  density: 'compact',
  pageSize: 50,
  autoRefreshMs: 5000,
};

function wrapper() {
  const client = createTestQueryClient();
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

describe('table view persistence', () => {
  let stored: { layout: { id: string; payload: LayoutPayload } | null; filters: SavedFilter[] };
  const puts = vi.fn();
  const deletes = vi.fn();

  beforeEach(() => {
    stored = { layout: null, filters: [] };
    puts.mockReset();
    deletes.mockReset();
    server.use(
      http.get('*/api/table-views/containers', () => HttpResponse.json({ success: true, data: stored })),
      http.put('*/api/table-views/containers/layout', async ({ request }) => {
        const payload = (await request.json()) as LayoutPayload;
        puts(payload);
        stored = { ...stored, layout: { id: 'l1', payload } };
        return HttpResponse.json({ success: true, data: stored.layout });
      }),
      http.delete('*/api/table-views/containers/layout', () => {
        deletes();
        stored = { ...stored, layout: null };
        return HttpResponse.json({ success: true });
      }),
    );
  });

  it('falls back to the declared defaults when nothing is saved', async () => {
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.resolved.order).toEqual(['name', 'image', 'status']);
    expect(result.current.resolved.visibility).toEqual({ name: true, image: false, status: true });
    expect(result.current.density).toBe('default');
    expect(result.current.pageSize).toBe(tableDefaultPageSize);
    expect(result.current.filters).toEqual([]);
  });

  it('loads the saved order, visibility, density, page size and refresh', async () => {
    stored.layout = { id: 'l1', payload: savedLayout };
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.resolved.order).toEqual(['status', 'name', 'image']);
    expect(result.current.resolved.visibility.image).toBe(true);
    expect(result.current.density).toBe('compact');
    expect(result.current.pageSize).toBe(50);
    expect(result.current.autoRefreshMs).toBe(5000);
  });

  it('saves the whole layout when one setting changes', async () => {
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setDensity('compact'));
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith({
      v: 1,
      columns: [
        { id: 'name', visible: true, size: 'auto' },
        { id: 'image', visible: false, size: 'auto' },
        { id: 'status', visible: true, size: 'auto' },
      ],
      density: 'compact',
      pageSize: tableDefaultPageSize,
      autoRefreshMs: 0,
    });
    await waitFor(() => expect(result.current.density).toBe('compact'));
  });

  it('rolls the optimistic change back when the save fails', async () => {
    stored.layout = { id: 'l1', payload: savedLayout };
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.put('*/api/table-views/containers/layout', async () => {
        await held;
        return HttpResponse.json({ success: false, error: 'table view store is read-only' }, { status: 500 });
      }),
    );
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.density).toBe('compact'));
    act(() => result.current.setDensity('comfortable'));
    await waitFor(() => expect(result.current.density).toBe('comfortable'));
    release();
    await waitFor(() => expect(result.current.density).toBe('compact'));
  });

  it('resets to the defaults by deleting the saved layout', async () => {
    stored.layout = { id: 'l1', payload: savedLayout };
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.density).toBe('compact'));
    act(() => result.current.resetLayout());
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.density).toBe('default'));
    expect(result.current.resolved.order).toEqual(['name', 'image', 'status']);
    expect(result.current.resolved.visibility.image).toBe(false);
  });

  it('does not try to save when the table view store is unavailable', async () => {
    server.use(
      http.get('*/api/table-views/containers', () =>
        HttpResponse.json({ success: false, error: 'down' }, { status: 500 }),
      ),
    );
    const { result } = renderHook(() => useTableView('containers', columns), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.available).toBe(false), { timeout: 5000 });
    act(() => result.current.setDensity('compact'));
    expect(puts).not.toHaveBeenCalled();
    expect(result.current.resolved.order).toEqual(['name', 'image', 'status']);
  });

  it('creates and deletes saved filters, refreshing the list each time', async () => {
    const filter: SavedFilter = {
      id: 'f1',
      tableId: 'containers',
      name: 'Running',
      state: { status: 'running' },
      isDefault: false,
    };
    server.use(
      http.post('*/api/table-views/containers/filters', async ({ request }) => {
        const body = (await request.json()) as Omit<SavedFilter, 'id' | 'tableId'>;
        stored = { ...stored, filters: [{ ...filter, ...body }] };
        return HttpResponse.json({ success: true, data: stored.filters[0] });
      }),
      http.delete('*/api/table-views/containers/filters/f1', () => {
        stored = { ...stored, filters: [] };
        return HttpResponse.json({ success: true });
      }),
    );
    const { result } = renderHook(
      () => ({
        view: useTableView('containers', columns),
        create: useCreateFilter('containers'),
        remove: useDeleteFilter('containers'),
      }),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.view.loading).toBe(false));
    act(() => result.current.create.mutate({ name: 'Running', state: { status: 'running' }, isDefault: false }));
    await waitFor(() => expect(result.current.view.filters.map((f) => f.name)).toEqual(['Running']));
    act(() => result.current.remove.mutate('f1'));
    await waitFor(() => expect(result.current.view.filters).toEqual([]));
  });
});
