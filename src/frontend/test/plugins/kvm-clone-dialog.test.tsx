// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { CloneVMDialog } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/CloneVMDialog';
import type { VM } from '../../../plugins/Plugins/addon-kvm/frontend/src/types';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

const vm = {
  id: 'vm-1',
  environmentId: 'env-1',
  name: 'web',
  state: 'stopped',
  hostAvailable: true,
  nics: [],
  disks: [],
} as unknown as VM;

const submitLabel = 'Clone';

function mount() {
  const onClose = vi.fn();
  const onCloned = vi.fn();
  renderWithProviders(<CloneVMDialog vm={vm} onClose={onClose} onCloned={onCloned} />);
  return { onClose, onCloned };
}

async function submitButton() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).getByRole('button', { name: submitLabel });
}

describe('addon-kvm CloneVMDialog', () => {
  const clones = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/vms/vm-1/clone', async ({ request }) => {
        clones(await request.json());
        return respond();
      }),
    );
  }

  beforeEach(() => {
    clones.mockReset();
    grantPermissions(['vm.view', 'vm.manage']);
    server.use(http.get('*/api/vms', () => HttpResponse.json({ success: true, data: [] })));
    answer(() => HttpResponse.json({ success: true, data: { id: 'vm-2', name: 'web-clone' } }, { status: 201 }));
  });

  it('sends the typed name and the linked choice, then closes', async () => {
    const user = userEvent.setup();
    const { onClose, onCloned } = mount();
    const dialog = await screen.findByRole('dialog');
    const input = within(dialog).getByLabelText('Name for the copy');
    await user.clear(input);
    await user.type(input, 'web-copy');
    await user.click(within(dialog).getByRole('button', { name: /Linked copy/ }));
    await user.click(await submitButton());

    await waitFor(() => expect(onCloned).toHaveBeenCalledWith('vm-2'));
    expect(onClose).toHaveBeenCalled();
    expect(clones).toHaveBeenCalledTimes(1);
    expect(clones.mock.calls[0]?.[0]).toEqual({ name: 'web-copy', linked: true });
  });

  it('renders the submit disabled, naming vm.manage, when it is not held', async () => {
    grantPermissions(['vm.view']);
    const user = userEvent.setup();
    mount();
    const button = await submitButton();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(clones).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the vm.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message intact when the clone fails', async () => {
    const detail = 'volume pool-a/web-copy.qcow2 already exists';
    answer(() => HttpResponse.json({ success: false, code: 'err.vm.name_taken', error: detail }, { status: 409 }));
    const user = userEvent.setup();
    const { onClose, onCloned } = mount();
    await user.click(await submitButton());

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('A virtual machine on this host already has that name.');
    expect(alert).toHaveTextContent(detail);
    expect(onClose).not.toHaveBeenCalled();
    expect(onCloned).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(await submitButton()).toBeEnabled();
  });

  it('sends one request on a double click, and shows the copy progress while it runs', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: { id: 'vm-2', name: 'web-clone' } }, { status: 201 });
    });
    server.use(
      http.get('*/api/vms', () =>
        HttpResponse.json({
          success: true,
          data: [{ id: 'vm-2', name: 'web-clone', cloneProgress: { percent: 40, step: 'copying' } }],
        }),
      ),
    );
    const user = userEvent.setup();
    const { onCloned } = mount();
    const button = await submitButton();
    await user.dblClick(button);
    await user.click(button);

    expect(await screen.findByText('Copying disks… 40%')).toBeInTheDocument();
    release();
    await waitFor(() => expect(onCloned).toHaveBeenCalledWith('vm-2'));
    expect(clones).toHaveBeenCalledTimes(1);
  });
});
