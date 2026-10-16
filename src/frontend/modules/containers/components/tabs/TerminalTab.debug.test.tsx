// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { TerminalTab } from './TerminalTab';

describe('TerminalTab debug shell affordance', () => {
  const installs = vi.fn();

  beforeEach(() => {
    installs.mockReset();
    server.use(
      http.post('*/api/containers/c1/shells', () => HttpResponse.json({ success: true, data: [] })),
      http.post('*/api/containers/c1/debug-shell', () => {
        installs();
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
  });

  it('renders disabled, naming containers.debug_shell, when the key is missing', async () => {
    grantPermissions(['terminal.access']);
    const user = userEvent.setup();
    renderWithProviders(<TerminalTab containerId="c1" isRunning active />);
    const button = await screen.findByRole('button', { name: /Debug shell/ });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the containers.debug_shell permission')).not.toHaveLength(0);
    expect(installs).not.toHaveBeenCalled();
  });

  it('asks for confirmation before anything is uploaded', async () => {
    grantPermissions(['containers.debug_shell']);
    const user = userEvent.setup();
    renderWithProviders(<TerminalTab containerId="c1" isRunning active />);
    await user.click(await screen.findByRole('button', { name: /Debug shell/ }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/\/\.mcharbor-debug\/busybox/)).toBeInTheDocument();
    expect(installs).not.toHaveBeenCalled();
  });
});
