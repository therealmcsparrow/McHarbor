// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { BackupRestoreDialog } from '../../../plugins/Plugins/addon-lxc/frontend/src/components/BackupRestoreDialog';
import type { BackupRun } from '../../../plugins/Plugins/addon-lxc/frontend/src/hooks/useCTBackups';
import type { Container } from '../../../plugins/Plugins/addon-lxc/frontend/src/types';
import ctBundle from '../../../plugins/Plugins/addon-lxc/i18n/en/ct.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

const container = {
  id: 'ct-1',
  environmentId: 'env-1',
  name: 'web',
  state: 'stopped',
  hostAvailable: true,
  poolId: 'pool-1',
  poolBackend: 'btrfs',
  poolTarget: '/pool',
  rootfsPath: '/pool/ct-1',
  initSystem: 'systemd',
  privileged: false,
  nesting: false,
  limits: {},
  autostart: false,
  autostartOrder: 0,
  autostartDelaySeconds: 0,
  isTemplate: false,
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-01T00:00:00Z',
} as unknown as Container;

const run = {
  id: 'run-1',
  containerId: 'ct-1',
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

const restoreLabel = 'Restore this backup';

function mount() {
  const onOpenChange = vi.fn();
  const onSuccess = vi.fn();
  renderWithProviders(
    <BackupRestoreDialog container={container} run={run} onOpenChange={onOpenChange} onSuccess={onSuccess} />,
  );
  return { onOpenChange, onSuccess };
}

async function confirmButton() {
  const dialog = await screen.findByRole('dialog');
  return within(dialog).getByRole('button', { name: restoreLabel });
}

async function typeName(user: ReturnType<typeof userEvent.setup>, value: string) {
  const dialog = await screen.findByRole('dialog');
  await user.type(within(dialog).getByLabelText('Type web to confirm'), value);
}

describe('addon-lxc BackupRestoreDialog', () => {
  const restores = vi.fn();

  beforeAll(() => {
    i18n.addResourceBundle('en', 'ct', ctBundle, true, true);
  });

  function answer(respond: () => Response | Promise<Response>) {
    server.use(
      http.post('*/api/system-containers/backups/run-1/restore', async ({ request }) => {
        restores(await request.json());
        return respond();
      }),
    );
  }

  beforeEach(() => {
    restores.mockReset();
    grantPermissions(['ct.restore']);
    answer(() => HttpResponse.json({ success: true, data: { id: 'restore-1' } }));
  });

  it('sends nothing until the container name is typed', async () => {
    const user = userEvent.setup();
    mount();
    expect(await confirmButton()).toBeDisabled();
    await user.click(await confirmButton());
    expect(restores).not.toHaveBeenCalled();
  });

  it('stays disabled when the typed name is wrong', async () => {
    const user = userEvent.setup();
    mount();
    await typeName(user, 'wbe');
    expect(await confirmButton()).toBeDisabled();
    await user.click(await confirmButton());
    expect(restores).not.toHaveBeenCalled();
  });

  it('sends the typed name once the confirmation matches, then closes', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await typeName(user, 'web');
    await user.click(await confirmButton());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(restores).toHaveBeenCalledTimes(1);
    expect(restores).toHaveBeenCalledWith({ confirmName: 'web' });
  });

  it('renders the confirm disabled, naming ct.restore, when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    await typeName(user, 'web');
    const button = await confirmButton();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(restores).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the ct.restore permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message when the restore fails', async () => {
    const detail = 'the full copy this backup builds on is no longer available';
    answer(() =>
      HttpResponse.json({ success: false, error: detail, code: 'err.ct.backup_chain_incomplete' }, { status: 409 }),
    );
    const user = userEvent.setup();
    const { onOpenChange, onSuccess } = mount();
    await typeName(user, 'web');
    await user.click(await confirmButton());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent("This backup's chain is incomplete, so it cannot be restored.");
    expect(alert).toHaveTextContent(detail);
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(await confirmButton()).toBeEnabled();
  });

  it('sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: { id: 'restore-1' } });
    });
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await typeName(user, 'web');
    const button = await confirmButton();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(restores).toHaveBeenCalledTimes(1);
  });
});
