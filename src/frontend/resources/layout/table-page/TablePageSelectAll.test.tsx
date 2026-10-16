// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { grantPermissions } from '../../../test/permissions';

type Row = { id: string; name: string; locked: boolean };

const rows: Row[] = Array.from({ length: 60 }, (_, i) => ({
  id: `r${i}`,
  name: i < 10 ? `web-${i}` : `db-${i}`,
  locked: i === 59,
}));

const columns: ColumnDef<Row, unknown>[] = [{ id: 'name', accessorKey: 'name', header: 'Name' }];

function Page({
  search,
  onRemove,
  permission,
}: {
  search: string;
  onRemove: (rows: unknown[]) => void;
  permission?: string;
}) {
  return (
    <TablePage
      id="select-all-test"
      title="Rows"
      columns={columns}
      data={rows}
      selectable
      getRowId={(row) => row.id}
      isRowDisabled={(row) => row.locked}
      search={{
        value: search,
        onChange: () => {},
        mode: 'contains',
        onModeChange: () => {},
        placeholder: 'Search',
        fields: (row) => [row.name],
      }}
      batchActions={[
        { label: 'Remove', icon: IconTrash, variant: 'destructive', confirm: true, onClick: onRemove, permission },
      ]}
    />
  );
}

async function selectPage(user: ReturnType<typeof userEvent.setup>) {
  const header = await screen.findByRole('checkbox', { name: 'Select all' });
  await user.click(header);
}

describe('bulk selection across pages', () => {
  beforeEach(() => {
    server.use(
      http.get('*/api/table-views/select-all-test', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
    );
  });

  it('offers every matching row once the page is selected, and says how many', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Page search="" onRemove={vi.fn()} />);
    await selectPage(user);
    expect(await screen.findByText('25 of 60 selected')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Select all 59 matching' }));
    expect(await screen.findByText('59 of 60 selected')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Select all \d+ matching/ })).not.toBeInTheDocument();
  });

  it('never selects a disabled row through select-all', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    renderWithProviders(<Page search="" onRemove={onRemove} />);
    await selectPage(user);
    await user.click(await screen.findByRole('button', { name: 'Select all 59 matching' }));
    await user.click(screen.getByRole('button', { name: 'Remove' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }));
    const removed = onRemove.mock.calls[0]?.[0] as Row[];
    expect(removed.some((row) => row.locked)).toBe(false);
  });

  it('means every row matching the filter, not the rendered page, and confirms the count first', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    renderWithProviders(<Page search="db-" onRemove={onRemove} />);
    await selectPage(user);
    await user.click(await screen.findByRole('button', { name: 'Select all 49 matching' }));
    expect(await screen.findByText('49 of 50 selected')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Remove' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Are you sure you want to remove 49 items?')).toBeInTheDocument();
    expect(onRemove).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(onRemove).toHaveBeenCalledTimes(1));
    const removed = onRemove.mock.calls[0]?.[0] as Row[];
    expect(removed).toHaveLength(49);
    expect(removed.every((row) => row.name.startsWith('db-'))).toBe(true);
  });

  it('keeps the header checkbox to the page, so selecting it alone never reaches other pages', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    renderWithProviders(<Page search="" onRemove={onRemove} />);
    await selectPage(user);
    await user.click(await screen.findByRole('button', { name: 'Remove' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }));
    expect(onRemove.mock.calls[0]?.[0]).toHaveLength(25);
  });

  it('renders a batch action the caller lacks the permission for disabled, and never runs it', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    const onRemove = vi.fn();
    renderWithProviders(<Page search="" onRemove={onRemove} permission="users.manage" />);
    await selectPage(user);
    const remove = await screen.findByRole('button', { name: 'Remove' });
    expect(remove).toBeDisabled();
    await user.click(remove);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(onRemove).not.toHaveBeenCalled();
  });
});
