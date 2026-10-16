// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Button } from '@resources/components/ui/Button';
import { TooltipProvider } from '@resources/components/ui/Tooltip';
import { grantPermissions } from '../../../test/permissions';

function renderButton(props: Partial<Parameters<typeof Button>[0]> = {}) {
  const onClick = vi.fn();
  render(
    <TooltipProvider>
      <Button onClick={onClick} {...props}>
        Delete
      </Button>
    </TooltipProvider>,
  );
  return { onClick, button: screen.getByRole('button', { name: 'Delete' }) };
}

describe('Button permission gate', () => {
  it('behaves as a plain button without a permission', async () => {
    const { onClick, button } = renderButton();
    await userEvent.click(button);
    expect(button).toBeEnabled();
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('fires when the permission is held', async () => {
    grantPermissions(['volumes.delete']);
    const { onClick, button } = renderButton({ permission: 'volumes.delete' });
    await userEvent.click(button);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['no disabled prop', {}],
    ['disabled={false} from an idle mutation', { disabled: false }],
  ])('renders disabled and inert when denied, with %s', async (_label, extra) => {
    grantPermissions([]);
    const { onClick, button } = renderButton({ permission: 'volumes.delete', ...extra });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    await userEvent.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('names the missing permission in a tooltip reachable by keyboard', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    renderButton({ permission: ['volumes.delete', 'volumes.manage'] });
    await user.tab();
    expect(await screen.findAllByText('Requires the volumes.delete permission')).not.toHaveLength(0);
  });

  it('treats a wildcard holder as allowed', async () => {
    grantPermissions('*');
    const { onClick, button } = renderButton({ permission: 'anything.at_all' });
    await userEvent.click(button);
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
