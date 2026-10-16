// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { UploadRestoreDialog } from './UploadRestoreDialog';

class FakeXhr {
  static all: FakeXhr[] = [];
  upload: { onprogress?: (event: unknown) => void; onload?: () => void } = {};
  onload?: () => void;
  onerror?: () => void;
  onabort?: () => void;
  responseText = '';
  status = 0;
  withCredentials = false;
  constructor() {
    FakeXhr.all.push(this);
  }
  open() {}
  setRequestHeader() {}
  send() {}
  progress(loaded: number, total: number) {
    this.upload.onprogress?.({ lengthComputable: true, loaded, total });
    if (loaded >= total) this.upload.onload?.();
  }
  respond(status: number, body: unknown) {
    this.status = status;
    this.responseText = JSON.stringify(body);
    this.onload?.();
  }
}

function installFakeXhr() {
  FakeXhr.all = [];
  vi.stubGlobal('XMLHttpRequest', FakeXhr);
  return {
    requests: FakeXhr.all,
    first: () => {
      const request = FakeXhr.all[0];
      if (!request) throw new Error('no upload started');
      return request;
    },
    restore: () => vi.unstubAllGlobals(),
  };
}

const stagedRun = {
  id: 'run-up',
  environmentId: 'env-1',
  containerId: 'c-1',
  operation: 'backup',
  status: 'success',
  requiresSecretKey: true,
  availableItems: ['filesystem', 'mount:/data'],
};

function mount() {
  const onStaged = vi.fn();
  const onOpenChange = vi.fn();
  renderWithProviders(
    <UploadRestoreDialog containerId="c-1" dialogOpen onOpenChange={onOpenChange} onStaged={onStaged} />,
  );
  return { onStaged, onOpenChange };
}

async function pickFile(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await screen.findByRole('dialog');
  await user.upload(
    within(dialog).getByLabelText('Backup file'),
    new File(['archive'], 'web.mcharbor.tar', { type: 'application/x-tar' }),
  );
  return dialog;
}

describe('UploadRestoreDialog', () => {
  // The bug this guards: the upload restored everything at once, so the archive was never
  // read and no selection could be made. Reading must stage only, then hand the run on.
  it('stages the upload and hands the run and key to the restore dialog without restoring', async () => {
    grantPermissions(['restore.run']);
    const restoredAll = vi.fn();
    const staged = vi.fn();
    server.use(
      http.post('/api/containers/:id/backups/restore-upload', () => {
        restoredAll();
        return HttpResponse.json({ success: true, data: { runId: 'x', restored: [] } });
      }),
      http.post('/api/containers/:id/backups/upload', ({ params }) => {
        staged(params.id);
        return HttpResponse.json({ success: true, data: stagedRun });
      }),
    );
    const user = userEvent.setup();
    const { onStaged } = mount();
    const dialog = await pickFile(user);
    await user.type(within(dialog).getByLabelText('Backup security key'), 'secret-key');
    await user.click(within(dialog).getByRole('button', { name: 'Read backup' }));

    await waitFor(() => expect(onStaged).toHaveBeenCalledTimes(1));
    expect(onStaged).toHaveBeenCalledWith(expect.objectContaining({ id: 'run-up' }), 'secret-key');
    expect(staged).toHaveBeenCalledWith('c-1');
    expect(staged).toHaveBeenCalledTimes(1);
    expect(restoredAll).not.toHaveBeenCalled();
  });

  it('is disabled and names restore.run when the permission is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    const dialog = await pickFile(user);
    const read = within(dialog).getByRole('button', { name: 'Read backup' });
    expect(read).toBeDisabled();
    await user.hover(read.parentElement ?? read);
    expect(await screen.findAllByText(/restore\.run/)).not.toHaveLength(0);
  });

  it('keeps the dialog open and hands nothing on when the server refuses the archive', async () => {
    grantPermissions(['restore.run']);
    server.use(
      http.post('/api/containers/:id/backups/upload', () =>
        HttpResponse.json(
          {
            success: false,
            error: 'The backup security key does not match this archive.',
            code: 'err.container.backup_restore_key_invalid',
          },
          { status: 400 },
        ),
      ),
    );
    const user = userEvent.setup();
    const { onStaged, onOpenChange } = mount();
    const dialog = await pickFile(user);
    await user.click(within(dialog).getByRole('button', { name: 'Read backup' }));

    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Read backup' })).toBeEnabled());
    expect(onStaged).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('sends one upload on a double click while the first is still in flight', async () => {
    grantPermissions(['restore.run']);
    const xhr = installFakeXhr();
    const user = userEvent.setup();
    const { onStaged } = mount();
    const dialog = await pickFile(user);
    await user.dblClick(within(dialog).getByRole('button', { name: 'Read backup' }));

    await waitFor(() => expect(xhr.requests).toHaveLength(1));
    xhr.first().respond(200, { success: true, data: stagedRun });
    await waitFor(() => expect(onStaged).toHaveBeenCalledTimes(1));
    expect(xhr.requests).toHaveLength(1);
    xhr.restore();
  });

  it('shows the percentage and bytes while uploading, then the checking state once every byte is sent', async () => {
    grantPermissions(['restore.run']);
    const xhr = installFakeXhr();
    const user = userEvent.setup();
    mount();
    const dialog = await pickFile(user);
    await user.click(within(dialog).getByRole('button', { name: 'Read backup' }));
    await waitFor(() => expect(xhr.requests).toHaveLength(1));
    expect(within(dialog).getByRole('progressbar')).toBeInTheDocument();

    act(() => xhr.first().progress(512, 2048));
    expect(await within(dialog).findByText('Uploading 25% (512 B of 2 KB)')).toBeInTheDocument();

    act(() => xhr.first().progress(2048, 2048));
    expect(await within(dialog).findByText('Checking the archive…')).toBeInTheDocument();
    expect(within(dialog).queryByText(/Uploading [0-9]+%/)).not.toBeInTheDocument();
    xhr.restore();
  });

  it('keeps the dialog open and shows no progress after the server refuses the archive', async () => {
    grantPermissions(['restore.run']);
    const xhr = installFakeXhr();
    const user = userEvent.setup();
    const { onStaged } = mount();
    const dialog = await pickFile(user);
    await user.click(within(dialog).getByRole('button', { name: 'Read backup' }));
    await waitFor(() => expect(xhr.requests).toHaveLength(1));
    act(() => xhr.first().progress(2048, 2048));
    xhr.first().respond(400, { success: false, error: 'The backup security key does not match this archive.' });

    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Read backup' })).toBeEnabled());
    expect(within(dialog).queryByRole('progressbar')).not.toBeInTheDocument();
    expect(onStaged).not.toHaveBeenCalled();
    xhr.restore();
  });
});
