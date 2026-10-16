// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { MigrateVMDialog } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/MigrateVMDialog';
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

const reviewLabel = 'Review';
const submitLabel = 'Move machine';

const started = {
  id: 'mig-1',
  vmId: 'vm-1',
  sourceEnvironmentId: 'env-1',
  targetEnvironmentId: 'env-2',
  mode: 'cold',
  state: 'running',
  step: 'preparing',
  percent: 0,
  startedAt: '2026-09-30T18:00:00Z',
};

function capability(features: Record<string, boolean>) {
  return http.get('*/api/vms/capability', () =>
    HttpResponse.json({ success: true, data: { features, probedAt: '2026-09-30T18:00:00Z' } }),
  );
}

function mount() {
  const onClose = vi.fn();
  renderWithProviders(<MigrateVMDialog vm={vm} onClose={onClose} />);
  return { onClose };
}

async function dialogButton(name: string) {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).findByRole('button', { name });
}

/** Waits for the single target and its default pool to be picked, then reviews. */
async function review(user: ReturnType<typeof userEvent.setup>) {
  const next = await dialogButton(reviewLabel);
  await waitFor(() => expect(next).toBeEnabled());
  await user.click(next);
  return dialogButton(submitLabel);
}

describe('addon-kvm MigrateVMDialog', () => {
  const migrations = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
  });

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/vms/vm-1/migrate', async ({ request }) => {
        migrations(await request.json());
        return respond();
      }),
    );
  }

  beforeEach(() => {
    migrations.mockReset();
    grantPermissions(['vm.view', 'vm.migrate']);
    server.use(
      capability({ migrate_cold: true, migrate_live: true }),
      http.get('*/api/vms/vm-1/migration', () => HttpResponse.json({ success: true, data: null })),
      http.get('*/api/compute/environments', () =>
        HttpResponse.json({
          success: true,
          data: [
            { environmentId: 'env-1', name: 'rack-a', computeEnabled: true, available: true },
            { environmentId: 'env-2', name: 'rack-b', computeEnabled: true, available: true },
            { environmentId: 'env-3', name: 'offline', computeEnabled: true, available: false },
          ],
        }),
      ),
      http.get('*/api/compute/pools', () =>
        HttpResponse.json({
          success: true,
          data: [
            { id: 'pool-b', environmentId: 'env-2', name: 'fast', backend: 'zfs', target: '/tank', isDefault: true },
          ],
        }),
      ),
      http.get('*/api/compute/networks', () => HttpResponse.json({ success: true, data: [] })),
    );
    answer(() => HttpResponse.json({ success: true, data: started }, { status: 202 }));
  });

  it('sends nothing until the confirmation step is confirmed', async () => {
    const user = userEvent.setup();
    const { onClose } = mount();
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByRole('button', { name: submitLabel })).toBeNull();

    await review(user);
    expect(migrations).not.toHaveBeenCalled();
    await user.click(await dialogButton('Back'));
    await user.click(await dialogButton('Cancel'));
    expect(onClose).toHaveBeenCalled();
    expect(migrations).not.toHaveBeenCalled();
  });

  it('moves to the chosen host and pool, cold, then shows the progress', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await review(user));

    expect(await screen.findByText('Moving to rack-b.')).toBeInTheDocument();
    expect(migrations).toHaveBeenCalledTimes(1);
    expect(migrations.mock.calls[0]?.[0]).toEqual({
      targetEnvironmentId: 'env-2',
      targetPoolId: 'pool-b',
      targetPoolBackend: 'zfs',
      targetPoolTarget: '/tank',
      live: false,
    });
  });

  it('renders the move disabled, naming vm.migrate, when it is not held', async () => {
    grantPermissions(['vm.view']);
    const user = userEvent.setup();
    mount();
    const button = await review(user);
    expect(button).toBeDisabled();
    await user.click(button);
    expect(migrations).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the vm.migrate permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message intact when the move is refused', async () => {
    const detail = 'snapshot pre-upgrade still exists';
    answer(() =>
      HttpResponse.json({ success: false, code: 'err.vm.migrate_snapshots', error: detail }, { status: 409 }),
    );
    const user = userEvent.setup();
    const { onClose } = mount();
    await user.click(await review(user));

    const alert = await screen
      .findByText(/Machines with snapshots cannot be moved/, { selector: '[role="alert"] *' })
      .then((node) => node.closest('[role="alert"]') as HTMLElement);
    expect(alert).toHaveTextContent('Machines with snapshots cannot be moved. Delete or merge the snapshots first.');
    expect(alert).toHaveTextContent(detail);
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(await dialogButton(submitLabel)).toBeEnabled();
  });

  it('sends one request on a double click, with the response held open', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: started }, { status: 202 });
    });
    const user = userEvent.setup();
    mount();
    const button = await review(user);
    await user.dblClick(button);
    await user.click(button);
    release();
    expect(await screen.findByText('Moving to rack-b.')).toBeInTheDocument();
    expect(migrations).toHaveBeenCalledTimes(1);
  });

  it('offers no move when the host cannot migrate, and says why', async () => {
    server.use(capability({ migrate_cold: false, migrate_live: false }));
    mount();
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText(/migration/)).toBeInTheDocument();
    expect(await dialogButton(reviewLabel)).toBeDisabled();
  });
});
