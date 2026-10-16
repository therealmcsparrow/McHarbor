// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { HardwareDisks } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/HardwareDisks';
import type { Disk, VM } from '../../../plugins/Plugins/addon-kvm/frontend/src/types';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

const GIB = 1024 ** 3;

function disk(id: string, slot: number, extra: Partial<Disk> = {}): Disk {
  return {
    id,
    vmId: 'vm-1',
    poolId: 'pool',
    poolBackend: 'dir',
    poolTarget: '/pool',
    slot,
    bus: 'virtio-blk',
    volumePath: `/pool/${id}.qcow2`,
    format: 'qcow2',
    sizeBytes: 10 * GIB,
    ...extra,
  } as Disk;
}

const vm = { id: 'vm-1', environmentId: 'env-1', name: 'web', state: 'stopped', hostAvailable: true } as unknown as VM;

function mount(disks: Disk[] = [disk('data', 1)]) {
  renderWithProviders(<HardwareDisks vm={vm} disks={disks} mutable live={false} onAdd={vi.fn()} />);
}

describe('addon-kvm disk resize control', () => {
  const patches = vi.fn();

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.patch('*/api/vms/vm-1/disks/:diskId', async ({ params, request }) => {
        patches(params.diskId, await request.json());
        return respond();
      }),
    );
  }

  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  beforeEach(() => {
    patches.mockReset();
    grantPermissions(['vm.view', 'vm.manage']);
    server.use(http.get('*/api/vms*', () => HttpResponse.json({ success: true, data: [] })));
    answer(() => HttpResponse.json({ success: true, data: disk('data', 1, { sizeBytes: 11 * GIB }) }));
  });

  it('renders disabled, naming vm.manage, when the key is not held', async () => {
    grantPermissions(['vm.view']);
    const user = userEvent.setup();
    mount();
    const button = screen.getByRole('button', { name: 'Resize' });
    expect(button).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the vm.manage permission')).not.toHaveLength(0);
  });

  it('is unavailable for a disc, which is a file on the host', () => {
    mount([disk('iso', 2, { isCdrom: true })]);
    expect(screen.getByRole('button', { name: 'Resize' })).toBeDisabled();
  });

  it('starts one gigabyte above the current size and sends that size', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('button', { name: 'Resize' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByLabelText('New size (GB)')).toHaveValue(11);
    await user.click(within(dialog).getByRole('button', { name: 'Resize' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(patches).toHaveBeenCalledWith('data', { sizeGb: 11 });
  });

  it('shows the server message intact and keeps the dialog open on a refusal', async () => {
    const detail = 'the boot disk cannot be changed while the machine is running';
    answer(() =>
      HttpResponse.json({ success: false, code: 'err.vm.boot_disk_running', error: detail }, { status: 409 }),
    );
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('button', { name: 'Resize' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Resize' }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('The boot disk cannot be removed or changed while the machine is running.');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends one request on a double click while the first is still open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: disk('data', 1, { sizeBytes: 11 * GIB }) });
    });
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('button', { name: 'Resize' }));
    const dialog = await screen.findByRole('dialog');
    await user.dblClick(within(dialog).getByRole('button', { name: 'Resize' }));
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(patches).toHaveBeenCalledTimes(1);
  });
});
