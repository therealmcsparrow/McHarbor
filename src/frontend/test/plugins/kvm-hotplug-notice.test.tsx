// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AddDiskDialog } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/AddDiskDialog';
import type { VM } from '../../../plugins/Plugins/addon-kvm/frontend/src/types';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

// The same partial-VM fixture shape kvm-hardware-hotplug.test.tsx uses.
const vm = {
  id: 'vm-1',
  environmentId: 'env-1',
  name: 'web',
  state: 'running',
  guestOsFamily: 'linux',
  hostAvailable: true,
} as unknown as VM;

const pool = { id: 'p1', name: 'default', backend: 'dir', target: '/pool', isDefault: true };

describe('addon-kvm live add when the guest never picks the device up', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  beforeEach(() => {
    grantPermissions(['vm.view', 'vm.manage']);
    server.use(
      http.get('*/api/compute/pools', () => HttpResponse.json({ success: true, data: [pool] })),
      http.get('*/api/vms*', () => HttpResponse.json({ success: true, data: [] })),
    );
  });

  function answerAdd(notice?: string) {
    server.use(
      http.post('*/api/vms/vm-1/disks', () =>
        HttpResponse.json(
          { success: true, code: 'msg.vm.disk_added', data: { id: 'd9', slot: 3, notice } },
          { status: 201 },
        ),
      ),
    );
  }

  it('keeps the dialog open with the warning instead of closing as if all went well', async () => {
    answerAdd('hotplug_unclaimed');
    const onClose = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<AddDiskDialog vm={vm} onClose={onClose} />);

    await user.click(await screen.findByRole('button', { name: 'Add disk' }));

    expect(await screen.findByRole('status')).toHaveTextContent('has not picked it up');
    expect(onClose).not.toHaveBeenCalled();
    const closeButtons = screen.getAllByRole('button', { name: 'Close' });
    await user.click(closeButtons[closeButtons.length - 1]!);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes straight away when the guest picked the disk up', async () => {
    answerAdd();
    const onClose = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<AddDiskDialog vm={vm} onClose={onClose} />);

    await user.click(await screen.findByRole('button', { name: 'Add disk' }));
    await vi.waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
