// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { StackInfo } from '@resources/hooks/useStacks';
import { StackRowActions } from './StackTableCells';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';

function stack(targetsSelf: boolean): StackInfo {
  return { id: 's1', name: 'mcharbor', status: 'running', services: [], type: 'managed', targetsSelf };
}

function mount(s: StackInfo) {
  return renderWithProviders(
    <StackRowActions
      stack={s}
      onEdit={vi.fn()}
      onLogs={vi.fn()}
      onRemove={vi.fn()}
      onTakeOver={vi.fn()}
      onAction={vi.fn()}
    />,
  );
}

describe('StackRowActions on the stack that runs McHarbor', () => {
  beforeEach(() => grantPermissions(['stacks.view', 'stacks.manage', 'stacks.delete']));

  it('disables edit, restart, stop, down and remove and names host_self.manage', () => {
    mount(stack(true));
    const gated = screen.getAllByRole('button', { name: /host_self\.manage/ });
    const names = gated.map((b) => b.getAttribute('aria-label') ?? '');
    for (const action of ['Edit', 'Restart', 'Stop', 'Down', 'Remove']) {
      expect(names.some((n) => n.startsWith(action))).toBe(true);
    }
    for (const button of gated) expect(button).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Logs' })).toBeEnabled();
  });

  it('leaves another stack ungated', () => {
    mount(stack(false));
    expect(screen.queryAllByRole('button', { name: /host_self\.manage/ })).toHaveLength(0);
  });

  // The adopt route answers 403 err.host_self.stack_required for such a stack, so the
  // disabled Take over button must name the same key rather than leave it to the toast.
  it('disables Take over on a discovered stack that runs McHarbor', () => {
    mount({ ...stack(true), type: 'discovered' });
    const takeOver = screen
      .getAllByRole('button', { name: /host_self\.manage/ })
      .find((b) => (b.getAttribute('aria-label') ?? '').startsWith('Take Over'));
    expect(takeOver).toBeDefined();
    expect(takeOver).toBeDisabled();
  });

  it('lets a host_self.manage holder act on it', () => {
    grantPermissions(['stacks.view', 'stacks.manage', 'stacks.delete', 'host_self.manage']);
    mount(stack(true));
    expect(screen.queryAllByRole('button', { name: /host_self\.manage/ })).toHaveLength(0);
    expect(screen.getByRole('button', { name: 'Stop' })).toBeEnabled();
  });
});
