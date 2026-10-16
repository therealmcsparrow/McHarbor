// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen, within } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { PageActionsSlotProvider } from '@resources/layout/PageActionsSlot';
import { renderWithProviders } from '../../test/render';
import { server } from '../../test/server';

type Row = { id: string; name: string };
const columns: ColumnDef<Row, unknown>[] = [{ id: 'name', accessorKey: 'name', header: 'Name' }];
const rows: Row[] = [{ id: 'r1', name: 'row-1' }];

function Table() {
  return (
    <TablePage
      id="slot-test"
      header={false}
      title="Rows"
      columns={columns}
      data={rows}
      getRowId={(row) => row.id}
      toolbar={{ columns: true, actions: <button type="button">Create row</button> }}
    />
  );
}

function WithSlot() {
  const [slot, setSlot] = useState<HTMLDivElement | null>(null);
  return (
    <>
      <div data-testid="page-actions" ref={setSlot} />
      <div data-testid="tab-body">
        <PageActionsSlotProvider element={slot}>
          <Table />
        </PageActionsSlotProvider>
      </div>
    </>
  );
}

describe('page actions slot', () => {
  beforeEach(() => {
    server.use(
      http.get('*/api/table-views/slot-test', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
    );
  });

  it('moves a headerless table’s actions into the page header slot, once', async () => {
    renderWithProviders(<WithSlot />);
    const header = screen.getByTestId('page-actions');
    expect(await within(header).findByRole('button', { name: 'Create row' })).toBeInTheDocument();
    expect(
      within(screen.getByTestId('tab-body')).queryByRole('button', { name: 'Create row' }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Create row' })).toHaveLength(1);
  });

  it('leaves the actions in the toolbar when no page provides a slot', async () => {
    renderWithProviders(<Table />);
    expect(await screen.findByRole('button', { name: 'Create row' })).toBeInTheDocument();
  });
});
