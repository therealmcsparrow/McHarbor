// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { InstanceBackupPanel } from './InstanceBackupPanel';
import { canSubmit } from './CreateExportDialog';
import type { InstanceExport } from '../hooks/useInstanceBackups';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const done: InstanceExport = {
  id: 'x1',
  status: 'completed',
  trigger: 'manual',
  createdBy: 'admin',
  fileName: 'x1-mcharbor-20260926.mchx',
  sizeBytes: 2048,
  keyKind: 'passphrase',
  keyId: 'sha256:abc',
  includeMetrics: false,
  note: 'before upgrade',
  error: '',
  verifyStatus: '',
  verifyError: '',
  createdAt: '2026-09-26T10:00:00Z',
};
const failed: InstanceExport = {
  ...done,
  id: 'x2',
  status: 'failed',
  fileName: '',
  note: 'nightly',
  error: 'settings changed while it was exported (3 rows counted, 4 read)',
};

const LOCATIONS = [
  {
    id: 'loc-nas',
    name: 'Office NAS',
    locationType: 'sftp',
    enabled: true,
    passiveMode: false,
    createdAt: '',
    updatedAt: '',
  },
  {
    id: 'loc-s3',
    name: 'Object store',
    locationType: 's3',
    enabled: true,
    passiveMode: false,
    createdAt: '',
    updatedAt: '',
  },
  {
    id: 'loc-off',
    name: 'Retired disk',
    locationType: 'local',
    enabled: false,
    passiveMode: false,
    createdAt: '',
    updatedAt: '',
  },
];

const deletes = vi.fn();
const creates = vi.fn();

function mount() {
  return renderWithProviders(<InstanceBackupPanel />);
}

function row(text: string) {
  return screen.getByText(text).closest('tr') as HTMLElement;
}

describe('InstanceBackupPanel', () => {
  beforeEach(() => {
    deletes.mockReset();
    creates.mockReset();
    grantPermissions(['instance_backup.view', 'instance_backup.manage']);
    server.use(
      http.get('*/api/instance-backups', () =>
        HttpResponse.json({ success: true, data: { items: [done, failed], total: 2, page: 1, per_page: 100 } }),
      ),
      http.get('*/api/instance-backups/restore/status', () => HttpResponse.json({ success: true, data: null })),
      http.get('*/api/storage-locations', () => HttpResponse.json({ success: true, data: LOCATIONS })),
      http.delete('*/api/instance-backups/:id', ({ params }) => {
        deletes(params.id);
        return HttpResponse.json({ success: true, data: { status: 'deleted' } });
      }),
      http.post('*/api/instance-backups', async ({ request }) => {
        creates(await request.json());
        return HttpResponse.json({ success: true, data: { ...done, id: 'x3', status: 'running' } }, { status: 201 });
      }),
    );
  });

  it('only submits a passphrase that is long enough and repeated', () => {
    const base = { kind: 'passphrase' as const, passphrase: '', confirm: '', keyFile: '', keyFileSaved: false };
    expect(canSubmit({ ...base, passphrase: 'short', confirm: 'short' })).toBe(false);
    expect(canSubmit({ ...base, passphrase: 'long enough one', confirm: 'long enough on' })).toBe(false);
    expect(canSubmit({ ...base, passphrase: 'long enough one', confirm: 'long enough one' })).toBe(true);
    expect(canSubmit({ ...base, kind: 'keyfile', keyFile: 'mcharbor-instance-key-v1:abc' })).toBe(false);
    expect(canSubmit({ ...base, kind: 'keyfile', keyFile: 'mcharbor-instance-key-v1:abc', keyFileSaved: true })).toBe(
      true,
    );
  });

  it('puts a failed export behind its error, not in a cell', async () => {
    mount();
    const failedRow = await screen.findByTestId('export-row-x2');
    expect(within(failedRow).getByText('Failed')).toBeInTheDocument();
    expect(within(failedRow).queryByText(/rows counted/)).toBeNull();
    await userEvent.setup().click(within(failedRow).getByRole('button', { name: /error/i }));
    expect(await screen.findByText(failed.error)).toBeInTheDocument();
  });

  it('narrows the history by status facet and by search', async () => {
    const user = userEvent.setup();
    mount();
    await screen.findByTestId('export-row-x2');
    const failedFacet = screen.getByRole('button', { name: /^Failed/ });
    await user.click(failedFacet);
    expect(screen.getByTestId('export-row-x2')).toBeInTheDocument();
    expect(screen.queryByTestId('export-row-x1')).toBeNull();
    await user.click(within(failedFacet.closest('fieldset') as HTMLElement).getByRole('button', { name: /^All/ }));
    await user.type(screen.getAllByPlaceholderText('Search exports...')[0] as HTMLElement, 'upgrade');
    await waitFor(() => expect(screen.queryByTestId('export-row-x2')).toBeNull());
    expect(screen.getByTestId('export-row-x1')).toBeInTheDocument();
  });

  it('deletes only after confirmation, once, and keeps the dialog on a failure', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(
      within(await screen.findByText('before upgrade').then((n) => n.closest('tr') as HTMLElement)).getByRole(
        'button',
        { name: 'Delete' },
      ),
    );
    expect(deletes).not.toHaveBeenCalled();

    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.delete('*/api/instance-backups/:id', async ({ params }) => {
        deletes(params.id);
        await held;
        return HttpResponse.json(
          { success: false, error: 'deleting the export file: permission denied' },
          { status: 500 },
        );
      }),
    );
    const confirm = screen.getByRole('button', { name: 'Delete export' });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    expect(deletes).toHaveBeenCalledWith('x1');
    expect(await screen.findByText('deleting the export file: permission denied')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete export' })).toBeInTheDocument();
  });

  it('renders the actions disabled, naming the permission, without instance_backup.manage', async () => {
    grantPermissions(['instance_backup.view']);
    const user = userEvent.setup();
    mount();
    const del = within(await screen.findByText('before upgrade').then((n) => n.closest('tr') as HTMLElement)).getByRole(
      'button',
      { name: 'Delete' },
    );
    expect(del).toBeDisabled();
    expect(screen.getAllByRole('button', { name: /Create export/ })[0] as HTMLElement).toBeDisabled();
    await user.hover(del.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the instance_backup.manage permission')).not.toHaveLength(0);
    expect(deletes).not.toHaveBeenCalled();
  });

  it('creates an export with a passphrase and shows the server refusal in place', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getAllByRole('button', { name: /Create export/ })[0] as HTMLElement);
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse battery');
    await user.type(within(dialog).getByLabelText('Repeat the passphrase'), 'correct horse battery');

    server.use(
      http.post('*/api/instance-backups', () =>
        HttpResponse.json({ success: false, code: 'err.invalid_body', data: { reason: 'busy' } }, { status: 409 }),
      ),
    );
    await user.click(within(dialog).getByRole('button', { name: 'Create export' }));
    expect(await within(dialog).findByText('An export is already running. Wait for it to finish.')).toBeInTheDocument();

    server.use(
      http.post('*/api/instance-backups', async ({ request }) => {
        creates(await request.json());
        return HttpResponse.json({ success: true, data: { ...done, id: 'x3', status: 'running' } }, { status: 201 });
      }),
    );
    await user.click(within(dialog).getByRole('button', { name: 'Create export' }));
    await waitFor(() => expect(creates).toHaveBeenCalledTimes(1));
    expect(creates).toHaveBeenCalledWith({ passphrase: 'correct horse battery', includeMetrics: false });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('sends every destination the operator picked, not only the first', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getAllByRole('button', { name: /Create export/ })[0] as HTMLElement);
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse battery');
    await user.type(within(dialog).getByLabelText('Repeat the passphrase'), 'correct horse battery');

    await user.click(within(dialog).getByRole('button', { name: 'Also copy to' }));
    const list = await screen.findByRole('listbox', { name: 'Also copy to' });
    expect(within(list).queryByText('Retired disk')).toBeNull();
    await user.click(within(list).getByText('Office NAS'));
    await user.click(within(list).getByText('Object store'));
    await user.keyboard('{Escape}');

    await user.click(within(dialog).getByRole('button', { name: 'Create export' }));
    await waitFor(() => expect(creates).toHaveBeenCalledTimes(1));
    expect(creates).toHaveBeenCalledWith({
      passphrase: 'correct horse battery',
      includeMetrics: false,
      locationIds: ['loc-nas', 'loc-s3'],
    });
  });

  it('shows every copy of an export, and puts a refused one behind its reason', async () => {
    const copied = {
      ...done,
      id: 'x6',
      note: 'two places',
      destinations: [
        {
          id: 'd1',
          locationId: 'loc-nas',
          locationName: 'Office NAS',
          locationType: 'sftp',
          status: 'success' as const,
          remoteKey: 'k1',
          error: '',
          bytesUploaded: 2048,
          bytesTotal: 2048,
        },
        {
          id: 'd2',
          locationId: 'loc-s3',
          locationName: 'Object store',
          locationType: 's3',
          status: 'failure' as const,
          remoteKey: 'k2',
          error: 'the bucket refused the object: AccessDenied',
          bytesUploaded: 0,
          bytesTotal: 2048,
        },
      ],
    };
    server.use(
      http.get('*/api/instance-backups', () =>
        HttpResponse.json({ success: true, data: { items: [copied], total: 1, page: 1, per_page: 100 } }),
      ),
    );
    const user = userEvent.setup();
    mount();
    const twoPlaces = (await screen.findByText('two places')).closest('tr') as HTMLElement;
    expect(within(twoPlaces).getByText('Office NAS')).toBeInTheDocument();
    expect(within(twoPlaces).getByText('Object store')).toBeInTheDocument();
    expect(within(twoPlaces).queryByText(/AccessDenied/)).toBeNull();
    await user.click(within(twoPlaces).getAllByRole('button', { name: /error/i })[0] as HTMLElement);
    expect(await screen.findByText('the bucket refused the object: AccessDenied')).toBeInTheDocument();
  });

  it('verifies with the key of the export and puts a failed verification behind its reason', async () => {
    const verifies = vi.fn();
    const failedVerify = {
      ...done,
      id: 'x5',
      note: 'weekly',
      verifyStatus: 'failed',
      verifyError: 'audit row a3 does not link to the row before it',
      verifiedAt: '2026-09-26T12:00:00Z',
    };
    server.use(
      http.get('*/api/instance-backups', () =>
        HttpResponse.json({ success: true, data: { items: [done, failedVerify], total: 2, page: 1, per_page: 100 } }),
      ),
      http.post('*/api/instance-backups/:id/verify', async ({ params, request }) => {
        verifies(params.id, await request.json());
        return HttpResponse.json({ success: true, data: { ...done, verifyStatus: 'running' } });
      }),
    );
    const user = userEvent.setup();
    mount();
    const weekly = (await screen.findByText('weekly')).closest('tr') as HTMLElement;
    expect(within(weekly).queryByText(/does not link/)).toBeNull();
    await user.click(within(weekly).getByRole('button', { name: /error/i }));
    expect(await screen.findByText('audit row a3 does not link to the row before it')).toBeInTheDocument();
    await user.keyboard('{Escape}');

    await user.click(within(row('before upgrade')).getByRole('button', { name: 'Verify' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse battery');
    await user.click(within(dialog).getByRole('button', { name: 'Verify' }));
    await waitFor(() => expect(verifies).toHaveBeenCalledTimes(1));
    expect(verifies).toHaveBeenCalledWith('x1', { passphrase: 'correct horse battery' });
  });
});
