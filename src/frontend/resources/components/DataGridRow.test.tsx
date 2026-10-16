// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DataGridRow } from '@resources/components/DataGridRow';
import { appTableFeatures, useTable, type ColumnDef } from '@resources/components/table-features';
import { Button } from '@resources/components/ui/Button';

type Item = { id: string; name: string };

const data: Item[] = [{ id: 'c1', name: 'web' }];

function Grid({ onRowClick, onAction }: { onRowClick?: (row: Item) => void; onAction: () => void }) {
  const columns: ColumnDef<Item>[] = [
    { id: 'name', accessorKey: 'name', header: 'Name' },
    {
      id: 'actions',
      header: 'Actions',
      cell: () => (
        <Button
          onClick={(event) => {
            event.stopPropagation();
            onAction();
          }}
        >
          Stop
        </Button>
      ),
    },
  ];
  const table = useTable({ features: appTableFeatures, data, columns });
  return (
    <table>
      <tbody>
        {table.getRowModel().rows.map((row) => (
          <DataGridRow key={row.id} row={row} selectable={false} onRowClick={onRowClick} />
        ))}
      </tbody>
    </table>
  );
}

describe('DataGridRow', () => {
  it('keeps the row role on a clickable row, so its buttons are not nested in a button', () => {
    render(<Grid onRowClick={vi.fn()} onAction={vi.fn()} />);
    const row = screen.getByRole('row');
    expect(row).not.toHaveAttribute('role', 'button');
    expect(row).toHaveAttribute('tabindex', '0');
    expect(screen.getAllByRole('button')).toHaveLength(1);
  });

  it('opens the row on click, Enter and Space', async () => {
    const onRowClick = vi.fn();
    const user = userEvent.setup();
    render(<Grid onRowClick={onRowClick} onAction={vi.fn()} />);
    const row = screen.getByRole('row');
    await user.click(screen.getByText('web'));
    row.focus();
    await user.keyboard('{Enter}');
    await user.keyboard(' ');
    expect(onRowClick).toHaveBeenCalledTimes(3);
    expect(onRowClick).toHaveBeenCalledWith(data[0]);
  });

  it('does not open the row when an inner control is used', async () => {
    const onRowClick = vi.fn();
    const onAction = vi.fn();
    const user = userEvent.setup();
    render(<Grid onRowClick={onRowClick} onAction={onAction} />);
    await user.click(screen.getByRole('button', { name: 'Stop' }));
    screen.getByRole('button', { name: 'Stop' }).focus();
    await user.keyboard('{Enter}');
    expect(onAction).toHaveBeenCalledTimes(2);
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it('is not focusable when it has no click handler', () => {
    render(<Grid onAction={vi.fn()} />);
    expect(screen.getByRole('row')).not.toHaveAttribute('tabindex');
  });
});
