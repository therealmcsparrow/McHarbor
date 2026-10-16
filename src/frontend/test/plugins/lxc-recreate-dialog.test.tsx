// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { RecreateFromBackupDialog } from '../../../plugins/Plugins/addon-lxc/frontend/src/components/RecreateFromBackupDialog';
import type { BackupRun } from '../../../plugins/Plugins/addon-lxc/frontend/src/hooks/useCTBackups';
import type { RecreateDraft } from '../../../plugins/Plugins/addon-lxc/frontend/src/hooks/useRecreate';
import ctBundle from '../../../plugins/Plugins/addon-lxc/i18n/en/ct.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

const run = {
  id: 'run-1',
  containerId: 'ct-gone',
  containerName: 'web',
  environmentId: 'env-1',
  operation: 'backup',
  status: 'success',
  kind: 'full',
  consistency: 'filesystem',
  bytesTotal: 4096,
  destinations: [],
  startedAt: '2026-09-26T02:00:00Z',
  durationMs: 1000,
} as unknown as BackupRun;

const draft: RecreateDraft = {
  sourceRunId: 'run-1',
  sourceContainerId: 'ct-gone',
  takenAt: '2026-09-26T02:00:00Z',
  osFamily: 'debian',
  osVersion: '12',
  create: {
    environmentId: 'env-1',
    name: 'web',
    poolId: '',
    poolBackend: 'zfs',
    poolTarget: '/tank',
    initSystem: 'openrc',
    privileged: false,
    nesting: true,
    limits: { memoryMaxBytes: 1024 * 1024 * 1024, cpuMax: '200000 100000', pidsMax: 512, swapMaxBytes: 0 },
    nics: [{ networkId: 'net-1', bridge: 'br0', guestName: 'eth0', addressMode: 'dhcp' }],
    mounts: [{ kind: 'volume', source: 'data', destination: '/srv', readonly: true, shiftIds: true }],
  },
  warnings: ['nic_dropped'],
};

const submitLabel = 'Recreate container';

function mount() {
  const onOpenChange = vi.fn();
  const onCreated = vi.fn();
  renderWithProviders(
    <RecreateFromBackupDialog
      run={run}
      environments={[{ id: 'env-1', name: 'homelab' }]}
      onOpenChange={onOpenChange}
      onCreated={onCreated}
    />,
  );
  return { onOpenChange, onCreated };
}

async function submitButton() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).findByRole('button', { name: submitLabel });
}

async function nameInput() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).findByLabelText('Name');
}

async function poolResolved() {
  const dialog = await screen.findByRole('dialog');
  await within(dialog).findByText('tank · zfs · /tank');
}

describe('addon-lxc RecreateFromBackupDialog', () => {
  const recreates = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'ct', ctBundle, true, true);
  });

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/system-containers/backups/run-1/recreate', async ({ request }) => {
        recreates(await request.json());
        return respond();
      }),
    );
  }

  beforeEach(() => {
    recreates.mockReset();
    grantPermissions(['ct.restore', 'ct.manage']);
    server.use(
      http.get('*/api/system-containers/backups/run-1/manifest', () =>
        HttpResponse.json({ success: true, data: draft }),
      ),
      http.get('*/api/compute/pools', () =>
        HttpResponse.json({
          success: true,
          data: [
            {
              id: 'pool-other',
              environmentId: 'env-1',
              name: 'fast',
              backend: 'btrfs',
              target: '/fast',
              isDefault: true,
            },
            {
              id: 'pool-tank',
              environmentId: 'env-1',
              name: 'tank',
              backend: 'zfs',
              target: '/tank',
              isDefault: false,
            },
          ],
        }),
      ),
      http.get('*/api/compute/networks', () =>
        HttpResponse.json({
          success: true,
          data: [{ id: 'net-1', environmentId: 'env-1', name: 'lan', bridge: 'br0', isDefault: true }],
        }),
      ),
    );
    answer(() =>
      HttpResponse.json(
        { success: true, data: { container: { id: 'ct-new' }, run: { id: 'restore-1' } } },
        { status: 201 },
      ),
    );
  });

  it('pre-fills the definition stored with the backup', async () => {
    mount();
    expect(await nameInput()).toHaveValue('web');
    await poolResolved();
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('An interface with no bridge or parent adapter was left out.')).toBeInTheDocument();
    expect(within(dialog).getByText('lan')).toBeInTheDocument();
    expect(within(dialog).getByText('data → /srv')).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Allow containers inside this container')).toBeChecked();
    expect(within(dialog).getByDisplayValue('1024')).toBeInTheDocument();
    expect(within(dialog).getByDisplayValue('512')).toBeInTheDocument();
  });

  it('sends the edited definition, with the matched pool, then closes', async () => {
    const user = userEvent.setup();
    const { onOpenChange, onCreated } = mount();
    const input = await nameInput();
    await poolResolved();
    await user.clear(input);
    await user.type(input, 'web-restored');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Remove mount /srv' }));
    await user.click(await submitButton());

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('ct-new'));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(recreates).toHaveBeenCalledTimes(1);
    const body = recreates.mock.calls[0]?.[0];
    expect(body.create).toMatchObject({
      name: 'web-restored',
      environmentId: 'env-1',
      poolId: 'pool-tank',
      poolBackend: 'zfs',
      poolTarget: '/tank',
      initSystem: 'openrc',
      nesting: true,
      mounts: [],
      nics: [{ networkId: 'net-1', bridge: 'br0', guestName: 'eth0' }],
    });
    expect(body.create.limits).toMatchObject({ memoryMaxBytes: 1024 * 1024 * 1024, pidsMax: 512 });
    expect(body.osFamily).toBe('debian');
    expect(body.startAfterRestore).toBe(false);
  });

  it('renders the submit disabled, naming ct.manage, when only ct.restore is held', async () => {
    grantPermissions(['ct.restore']);
    const user = userEvent.setup();
    mount();
    await poolResolved();
    const button = await submitButton();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(recreates).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the ct.manage permission')).not.toHaveLength(0);
  });

  it('renders the submit disabled, naming ct.restore, when only ct.manage is held', async () => {
    grantPermissions(['ct.manage']);
    const user = userEvent.setup();
    mount();
    await poolResolved();
    const button = await submitButton();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(recreates).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the ct.restore permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message when the recreate fails', async () => {
    const detail = 'the full copy this backup builds on is no longer available';
    answer(() =>
      HttpResponse.json({ success: false, error: detail, code: 'err.ct.backup_chain_incomplete' }, { status: 409 }),
    );
    const user = userEvent.setup();
    const { onOpenChange, onCreated } = mount();
    await poolResolved();
    await user.click(await submitButton());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent("This backup's chain is incomplete, so it cannot be restored.");
    expect(alert).toHaveTextContent(detail);
    expect(onCreated).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(await submitButton()).toBeEnabled();
  });

  it('sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json(
        { success: true, data: { container: { id: 'ct-new' }, run: { id: 'restore-1' } } },
        { status: 201 },
      );
    });
    const user = userEvent.setup();
    const { onCreated } = mount();
    await poolResolved();
    const button = await submitButton();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('ct-new'));
    expect(recreates).toHaveBeenCalledTimes(1);
  });

  it('shows why the definition could not be read, and offers no submit', async () => {
    server.use(
      http.get('*/api/system-containers/backups/run-1/manifest', () =>
        HttpResponse.json(
          {
            success: false,
            error: 'manifest version 2 is not one this plugin reads',
            code: 'err.ct.backup_manifest_unreadable',
          },
          { status: 422 },
        ),
      ),
    );
    mount();
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent("This backup's container definition could not be read.");
    expect(alert).toHaveTextContent('manifest version 2 is not one this plugin reads');
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByRole('button', { name: submitLabel })).toBeNull();
  });
});
