// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import { delay, http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

type Row = { id: string; name: string };

const rows: Row[] = Array.from({ length: 120 }, (_, i) => ({ id: `r${i}`, name: `row-${i}` }));
const columns: ColumnDef<Row, unknown>[] = [{ id: 'name', accessorKey: 'name', header: 'Name' }];

function serveLayout(pageSize: number) {
  server.use(
    http.get('*/api/table-views/page-size-test', async () => {
      await delay(30);
      return HttpResponse.json({
        success: true,
        data: {
          layout: {
            id: 'l1',
            payload: {
              v: 1,
              columns: [{ id: 'name', visible: true, size: 'auto' }],
              density: 'default',
              pageSize,
              autoRefreshMs: 0,
            },
          },
          filters: [],
        },
      });
    }),
  );
}

function bodyRows(container: HTMLElement) {
  return container.querySelectorAll('tbody tr').length;
}

describe('saved page size', () => {
  it('applies a page size that arrives after the table mounted', async () => {
    serveLayout(100);
    const { container } = renderWithProviders(
      <TablePage id="page-size-test" title="Rows" columns={columns} data={rows} getRowId={(row) => row.id} />,
    );
    await waitFor(() => expect(bodyRows(container)).toBe(100));
    expect(screen.getByText('row-99')).toBeInTheDocument();
  });

  it('keeps the default when nothing is saved', async () => {
    server.use(
      http.get('*/api/table-views/page-size-test', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
    );
    const { container } = renderWithProviders(
      <TablePage id="page-size-test" title="Rows" columns={columns} data={rows} getRowId={(row) => row.id} />,
    );
    await waitFor(() => expect(bodyRows(container)).toBe(25));
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(bodyRows(container)).toBe(25);
  });
});
