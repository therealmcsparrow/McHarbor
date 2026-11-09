// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { BackupTempCleanupResult, BackupTempStatus } from '../hooks/useBackupTemp';
import { BackupTempCleanup } from './BackupTempCleanup';

const TEMP: BackupTempStatus = {
  path: '/srv/spool/mcharbor',
  source: 'setting',
  settingPath: '/srv/spool/mcharbor',
  envOverride: false,
  envInvalidCode: '',
  defaultPath: '/var/lib/mcharbor/backups/tmp',
  sizeBytes: 5 * 1024 * 1024,
  entries: 7,
  activeRuns: 0,
  retiredPaths: [],
};

const RESULT: BackupTempCleanupResult = {
  removedEntries: 3,
  removedBytes: 2048,
  keptOwned: 1,
  keptRecent: 2,
  activeRuns: 1,
};

const calls = vi.fn();

function answer(response: () => Response | Promise<Response>) {
  server.use(
    http.post('*/api/container-backups/temp/cleanup', async () => {
      calls();
      return response();
    }),
  );
}

const ok =
  (data: BackupTempCleanupResult = RESULT) =>
  () =>
    HttpResponse.json({ success: true, data });

const trigger = () => screen.getByRole('button', { name: 'Clean up temp' });
const dialog = () => screen.findByRole('dialog');
const confirm = async () => within(await dialog()).getByRole('button', { name: 'Clean up temp' });

function mount(temp: BackupTempStatus = TEMP) {
  server.use(http.get('*/api/container-backups/temp', () => HttpResponse.json({ success: true, data: temp })));
  renderWithProviders(<BackupTempCleanup temp={temp} />);
}

describe('BackupTempCleanup', () => {
  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['backups.manage']);
    answer(ok());
  });

  it('keeps an empty status region in the page before the first clean-up', () => {
    mount();
    const region = screen.getByRole('status');
    expect(region).toBeInTheDocument();
    expect(region).toBeEmptyDOMElement();
  });

  it('shows the entry count, size and path before confirm, and sends nothing until then', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    const body = within(await dialog());
    expect(body.getByText(/removes 7 leftover entries \(5 MB\) from \/srv\/spool\/mcharbor/)).toBeInTheDocument();
    expect(calls).not.toHaveBeenCalled();
  });

  it('sends nothing when the dialog is cancelled', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    await user.click(within(await dialog()).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(calls).not.toHaveBeenCalled();
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
  });

  it('warns in the confirm that a running backup limits what is removed', async () => {
    const user = userEvent.setup();
    mount({ ...TEMP, activeRuns: 2 });
    await user.click(trigger());
    expect(
      within(await dialog()).getByText('A run is active, so only entries older than an hour are removed.'),
    ).toBeInTheDocument();
  });

  it('sends one request after confirm, closes, and announces the removed and kept counts with their reason', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    await user.click(await confirm());
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(calls).toHaveBeenCalledTimes(1);
    const status = screen.getByRole('status');
    await waitFor(() => expect(status).toHaveTextContent('Removed 3 entries (2 KB).'));
    expect(status).toHaveTextContent('Kept 1 in use by a running backup or restore.');
    expect(status).toHaveTextContent('Kept 2 newer than an hour.');
  });

  it('announces that nothing was removed when the server removed nothing', async () => {
    answer(ok({ removedEntries: 0, removedBytes: 0, keptOwned: 0, keptRecent: 4, activeRuns: 0 }));
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    await user.click(await confirm());
    const status = screen.getByRole('status');
    await waitFor(() => expect(status).toHaveTextContent('Nothing was removed.'));
    expect(status).toHaveTextContent('Kept 4 newer than an hour.');
  });

  it('renders the control disabled, naming backups.manage, and opens nothing when the key is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const button = trigger();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(calls).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the backups.manage permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and shows the server message intact when the clean-up fails', async () => {
    const message = 'Clean-up refused: /srv/spool/mcharbor is read-only';
    answer(() =>
      HttpResponse.json({ success: false, error: message, code: 'err.backup_temp.cleanup_failed' }, { status: 500 }),
    );
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    await user.click(await confirm());
    expect(await within(await dialog()).findByText(message)).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
    expect(await confirm()).toBeEnabled();
  });

  it('sends one request on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: RESULT });
    });
    const user = userEvent.setup();
    mount();
    await user.click(trigger());
    const button = await confirm();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(calls).toHaveBeenCalledTimes(1);
  });

  it('disables the control when there is nothing to clean up', async () => {
    const user = userEvent.setup();
    mount({ ...TEMP, entries: 0, sizeBytes: 0 });
    expect(trigger()).toBeDisabled();
    await user.click(trigger());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(calls).not.toHaveBeenCalled();
  });
});
