// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostRebootCard } from '@resources/components/host/HostRebootCard';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import type { HostRebootState } from '@resources/hooks/useHostReboot';

const TARGET: HostTarget = { kind: 'environment', envId: 'env-1' };
const PATH = '*/api/system/os-reboot';

function state(data: HostRebootState) {
  return http.get(PATH, () => HttpResponse.json({ success: true, data }));
}

describe('HostRebootCard', () => {
  const deletes = vi.fn();

  beforeEach(() => {
    deletes.mockReset();
    grantPermissions(['host_updates.reboot']);
    server.use(
      http.delete(PATH, () => {
        deletes();
        return HttpResponse.json({ success: true, data: { cancelled: true } });
      }),
    );
  });

  it('stays out of the way when no reboot is required or recorded', async () => {
    server.use(state({}));
    const { container } = renderWithProviders(
      <HostRebootCard target={TARGET} rebootRequired={false} disabled={false} />,
    );
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it('offers the reboot when the host needs one', async () => {
    server.use(state({}));
    renderWithProviders(<HostRebootCard target={TARGET} rebootRequired disabled={false} />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Reboot machine' })).toBeEnabled());
  });

  it('renders the reboot disabled in demo mode', async () => {
    server.use(state({ blockedReason: 'demo_mode' }));
    const user = userEvent.setup();
    renderWithProviders(<HostRebootCard target={TARGET} rebootRequired disabled={false} />);
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Reboot machine' }).parentElement).toHaveAttribute('tabindex', '0'),
    );
    const button = screen.getByRole('button', { name: 'Reboot machine' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Demo mode never reboots a host.')).not.toHaveLength(0);
  });

  it('puts a failed reboot behind the error icon, with the translated reason and detail', async () => {
    server.use(
      state({
        reboot: {
          id: 'r1',
          status: 'failed',
          scheduledAt: '2030-01-02T03:04:00Z',
          drainedExternally: false,
          failureReason: 'container_timeout',
          lastError: 'these containers were still running after 10m0s and were left running, not killed: postgres',
        },
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<HostRebootCard target={TARGET} rebootRequired disabled={false} />);
    expect(await screen.findByText('Failed')).toBeInTheDocument();
    expect(screen.queryByText(/not killed: postgres/)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'View the error' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/did not stop in time/)).toBeInTheDocument();
    expect(within(dialog).getByText(/not killed: postgres/)).toBeInTheDocument();
  });

  it('cancels a scheduled reboot with one request', async () => {
    server.use(
      state({
        reboot: { id: 'r1', status: 'scheduled', scheduledAt: '2030-01-02T03:04:00Z', drainedExternally: false },
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<HostRebootCard target={TARGET} rebootRequired disabled={false} />);
    expect(await screen.findByText('Scheduled')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel reboot' }));
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
  });
});
