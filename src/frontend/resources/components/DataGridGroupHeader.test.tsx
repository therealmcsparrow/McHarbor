// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DataGridGroupHeader } from './DataGridGroupHeader';

function mount(collapsed: boolean, onToggle = vi.fn()) {
  render(
    <table>
      <tbody>
        <DataGridGroupHeader label="production" count={7} collapsed={collapsed} colSpan={4} onToggle={onToggle} />
      </tbody>
    </table>,
  );
  return onToggle;
}

describe('DataGridGroupHeader', () => {
  it('offers to expand a collapsed group and reports it as collapsed', () => {
    mount(true);
    const toggle = screen.getByRole('button', { name: 'Expand production' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('offers to collapse an expanded group and reports it as expanded', () => {
    mount(false);
    expect(screen.getByRole('button', { name: 'Collapse production' })).toHaveAttribute('aria-expanded', 'true');
  });

  it('toggles once per click and never submits an enclosing form', async () => {
    const onSubmit = vi.fn((event: { preventDefault: () => void }) => event.preventDefault());
    const onToggle = vi.fn();
    render(
      <form onSubmit={onSubmit}>
        <table>
          <tbody>
            <DataGridGroupHeader label="production" count={7} collapsed={false} colSpan={4} onToggle={onToggle} />
          </tbody>
        </table>
      </form>,
    );
    await userEvent.setup().click(screen.getByRole('button', { name: 'Collapse production' }));
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
