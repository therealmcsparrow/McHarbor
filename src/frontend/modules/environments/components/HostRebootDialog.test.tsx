// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { describeDestructiveDialog } from '../../../test/destructive';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostRebootDialog } from '@resources/components/host/HostRebootDialog';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import type { HostRebootInventory } from '@resources/hooks/useHostReboot';

const TARGET: HostTarget = { kind: 'environment', envId: 'env-1' };
const PATH = '*/api/system/os-reboot';
const INVENTORY_PATH = '*/api/system/os-reboot/inventory';

const PLAIN: HostRebootInventory = {
  mcharborHost: false,
  containersKnown: true,
  containers: 2,
  containersReturning: 1,
  guestsKnown: false,
  guests: 0,
  swarmTasks: 0,
  kubernetesKnown: true,
};

function inventory(data: HostRebootInventory) {
  return http.get(INVENTORY_PATH, () => HttpResponse.json({ success: true, data }));
}

describeDestructiveDialog({
  name: 'HostRebootDialog',
  method: 'post',
  path: PATH,
  permission: 'host_updates.reboot',
  confirmLabel: 'Reboot machine',
  handlers: [inventory(PLAIN)],
  body: { confirm: true, now: true, drainedExternally: false },
  reportsSuccess: false,
  render: ({ onOpenChange }) => <HostRebootDialog target={TARGET} open onOpenChange={onOpenChange} />,
});

describe('HostRebootDialog options', () => {
  const calls = vi.fn();

  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['host_updates.reboot']);
    server.use(
      inventory(PLAIN),
      http.post(PATH, async ({ request }) => {
        calls(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'r1', status: 'scheduled' } });
      }),
    );
  });

  async function confirmButton() {
    const dialog = await screen.findByRole('dialog');
    return within(dialog).getByRole('button', { name: 'Reboot machine' });
  }

  it('says what it detected on the host', async () => {
    renderWithProviders(<HostRebootDialog target={TARGET} open onOpenChange={vi.fn()} />);
    expect(
      await screen.findByText('Detected on this host: 2 running, 1 started again afterwards.'),
    ).toBeInTheDocument();
    expect(screen.getByText(/never killed/)).toBeInTheDocument();
  });

  it('counts the guests every draining plugin reported, whatever their kind', async () => {
    server.use(inventory({ ...PLAIN, guestsKnown: true, guests: 3 }));
    renderWithProviders(<HostRebootDialog target={TARGET} open onOpenChange={vi.fn()} />);
    expect(
      await screen.findByText('Detected on this host: 3 guests running (virtual machines and system containers).'),
    ).toBeInTheDocument();
  });

  it('sends the chosen moment when the reboot is scheduled for later', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<HostRebootDialog target={TARGET} open onOpenChange={onOpenChange} />);
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'At a date and time' }));
    fireEvent.change(within(dialog).getByLabelText('Date and time'), {
      target: { value: '2030-01-02T03:04' },
    });
    await user.click(await confirmButton());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledWith({
      confirm: true,
      now: false,
      drainedExternally: false,
      scheduledAt: new Date('2030-01-02T03:04').toISOString(),
    });
  });

  it('holds the confirm until a detected Kubernetes node is confirmed as drained', async () => {
    server.use(inventory({ ...PLAIN, kubernetes: ['k3s-server'] }));
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<HostRebootDialog target={TARGET} open onOpenChange={onOpenChange} />);
    const dialog = await screen.findByRole('dialog');
    const drained = await within(dialog).findByRole('checkbox', {
      name: 'I have cordoned and drained this node',
    });
    expect(await confirmButton()).toBeDisabled();
    await user.click(drained);
    await user.click(await confirmButton());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledWith({ confirm: true, now: true, drainedExternally: true });
  });

  it('translates a refusal reason the server sends', async () => {
    server.use(
      http.post(PATH, () =>
        HttpResponse.json(
          { success: false, error: 'Forbidden', code: 'err.forbidden', data: { reason: 'reboot_in_progress' } },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<HostRebootDialog target={TARGET} open onOpenChange={vi.fn()} />);
    await user.click(await confirmButton());
    expect(await screen.findByText('A reboot of this host is already in progress.')).toBeInTheDocument();
  });

  it('never enables the confirm in demo mode', async () => {
    const user = userEvent.setup();
    renderWithProviders(<HostRebootDialog target={TARGET} open blocked onOpenChange={vi.fn()} />);
    const button = await confirmButton();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(calls).not.toHaveBeenCalled();
  });
});
