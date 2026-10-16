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
    sizeBytes: 10 * 1024 ** 3,
    ...extra,
  } as Disk;
}

const bootDisk = disk('boot', 0, { isBoot: true });
const dataDisk = disk('data', 1);
const vm = { id: 'vm-1', environmentId: 'env-1', name: 'web', state: 'running', hostAvailable: true } as unknown as VM;

const bootLockedText = 'The boot disk cannot be removed or changed while the machine is running.';

function mount(props: { mutable: boolean; live: boolean; disks?: Disk[] }) {
  renderWithProviders(
    <HardwareDisks
      vm={vm}
      disks={props.disks ?? [bootDisk, dataDisk]}
      mutable={props.mutable}
      live={props.live}
      onAdd={vi.fn()}
    />,
  );
}

function deleteButtons() {
  return screen.getAllByRole('button', { name: 'Delete' });
}

describe('addon-kvm HardwareDisks while the machine runs', () => {
  const deletes = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.delete('*/api/vms/vm-1/disks/:diskId', ({ params }) => {
        deletes(params.diskId);
        return respond();
      }),
    );
  }

  beforeEach(() => {
    deletes.mockReset();
    grantPermissions(['vm.view', 'vm.manage', 'vm.delete']);
    server.use(http.get('*/api/vms*', () => HttpResponse.json({ success: true, data: [] })));
    answer(() => HttpResponse.json({ success: true, code: 'msg.vm.deleted' }));
  });

  it('keeps the boot disk locked with its reason, and the data disk removable', async () => {
    const user = userEvent.setup();
    mount({ mutable: false, live: true });
    const [boot, data] = deleteButtons();
    expect(boot).toBeDisabled();
    expect(data).toBeEnabled();
    await user.hover(boot!.parentElement as HTMLElement);
    expect(await screen.findAllByText(bootLockedText)).not.toHaveLength(0);
  });

  it('locks the disk first in the boot order too, not only the flagged one', () => {
    const ordered = disk('first', 2, { bootIndex: 1 });
    mount({ mutable: false, live: true, disks: [{ ...bootDisk, isBoot: false }, dataDisk, ordered] });
    const [plain, data, first] = deleteButtons();
    expect(first).toBeDisabled();
    expect(plain).toBeEnabled();
    expect(data).toBeEnabled();
  });

  it('keeps every delete disabled on a running machine whose host cannot hot-plug', () => {
    mount({ mutable: false, live: false });
    for (const button of deleteButtons()) expect(button).toBeDisabled();
  });

  it('sends nothing until the removal is confirmed', async () => {
    const user = userEvent.setup();
    mount({ mutable: false, live: true });
    await user.click(deleteButtons()[1]!);
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(deletes).not.toHaveBeenCalled();
  });

  it('renders delete disabled, naming vm.delete, when it is not held', async () => {
    grantPermissions(['vm.view', 'vm.manage']);
    const user = userEvent.setup();
    mount({ mutable: false, live: true });
    const data = deleteButtons()[1]!;
    expect(data).toBeDisabled();
    await user.hover(data.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the vm.delete permission')).not.toHaveLength(0);
  });

  it('shows the server message intact and keeps the dialog open when the guest needs a restart', async () => {
    const detail = 'this guest was started before it had hot-plug ports';
    answer(() =>
      HttpResponse.json({ success: false, code: 'err.vm.hotplug_restart_required', error: detail }, { status: 409 }),
    );
    const user = userEvent.setup();
    mount({ mutable: false, live: true });
    await user.click(deleteButtons()[1]!);
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Remove disk' }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('Restart it once to enable hot-plug.');
    expect(alert).toHaveTextContent(detail);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(deletes).toHaveBeenCalledWith('data');
  });

  it('sends one request on a double click while the first is still open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, code: 'msg.vm.deleted' });
    });
    const user = userEvent.setup();
    mount({ mutable: false, live: true });
    await user.click(deleteButtons()[1]!);
    const dialog = await screen.findByRole('dialog');
    const confirm = within(dialog).getByRole('button', { name: 'Remove disk' });
    await user.dblClick(confirm);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(deletes).toHaveBeenCalledTimes(1);
  });
});
