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
import { canPlan, type RestoreForm } from './RestoreSourceStep';
import type { InstanceExport } from '../hooks/useInstanceBackups';
import type { RestorePlan, RestoreStatus } from '../hooks/useInstanceRestore';

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
  fileName: 'x1-a.mchx',
  sizeBytes: 10,
  keyKind: 'passphrase',
  keyId: 'sha256:1',
  includeMetrics: false,
  note: 'nightly',
  error: '',
  verifyStatus: '',
  verifyError: '',
  createdAt: '2026-09-26T10:00:00Z',
};

const plan: RestorePlan = {
  manifest: {
    version: '2026.09.01',
    build: '1',
    backend: 'sqlite',
    createdAt: '2026-09-26T10:00:00Z',
    createdBy: 'admin',
    hostname: 'harbor',
    tables: [],
    plugins: [],
    blobs: [],
    includesMetrics: false,
    key: { kind: 'passphrase', keyId: 'k' },
  },
  mode: 'full',
  categories: [
    {
      name: 'access',
      selected: true,
      tables: [{ name: 'users', targetRows: 1, artifactRows: 4, exists: true }],
    },
  ],
  restoresKeys: true,
  restoresSecret: false,
  restoresFiles: true,
  sameKey: false,
  plugins: [{ slug: 'comm-ntfy', version: '2026.09.02', enabled: true }],
  refusals: [],
  warnings: ['sessions_end'],
};

const starts = vi.fn();

function row(text: string) {
  return screen.getByText(text).closest('tr') as HTMLElement;
}

async function openPlan(user: ReturnType<typeof userEvent.setup>) {
  renderWithProviders(<InstanceBackupPanel />);
  await screen.findByText('nightly');
  await user.click(within(row('nightly')).getByRole('button', { name: 'Restore' }));
  const dialog = await screen.findByRole('dialog');
  await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse battery');
  await user.click(within(dialog).getByRole('button', { name: 'Check export' }));
  return dialog;
}

describe('instance restore', () => {
  beforeEach(() => {
    starts.mockReset();
    grantPermissions(['instance_backup.view', 'instance_backup.manage', 'instance_backup.restore']);
    server.use(
      http.get('*/api/instance-backups', () =>
        HttpResponse.json({
          success: true,
          data: { items: [done], total: 1, page: 1, per_page: 100 },
        }),
      ),
      http.get('*/api/instance-backups/restore/status', () => HttpResponse.json({ success: true, data: null })),
      http.post('*/api/instance-backups/restore/plan', () => HttpResponse.json({ success: true, data: plan })),
      http.post('*/api/instance-backups/restore', async ({ request }) => {
        starts(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'r1', state: 'running' } }, { status: 201 });
      }),
    );
  });

  it('needs a source, a key and a scope before it checks anything', () => {
    const base: RestoreForm = {
      exportId: 'x1',
      file: null,
      keyKind: 'passphrase',
      passphrase: 'p',
      keyFile: '',
      mode: 'full',
      categories: [],
      includeAudit: false,
      conflict: 'replace',
    };
    expect(canPlan(base)).toBe(true);
    expect(canPlan({ ...base, exportId: '' })).toBe(false);
    expect(canPlan({ ...base, passphrase: '' })).toBe(false);
    expect(canPlan({ ...base, mode: 'selective' })).toBe(false);
    expect(canPlan({ ...base, mode: 'selective', categories: ['access'] })).toBe(true);
  });

  it('shows the plan and restores once, only after the confirmation word', async () => {
    const user = userEvent.setup();
    const dialog = await openPlan(user);
    expect(await within(dialog).findByText('Users, roles and policies')).toBeInTheDocument();
    expect(within(dialog).getByText('Everyone is signed out.')).toBeInTheDocument();
    const submit = within(dialog).getByRole('button', { name: 'Restore now' });
    expect(submit).toBeDisabled();
    await user.type(within(dialog).getByLabelText('Type RESTORE to confirm'), 'restore');
    expect(submit).toBeDisabled();
    await user.clear(within(dialog).getByLabelText('Type RESTORE to confirm'));
    await user.type(within(dialog).getByLabelText('Type RESTORE to confirm'), 'RESTORE');
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/instance-backups/restore', async ({ request }) => {
        starts(await request.json());
        await held;
        return HttpResponse.json({ success: true, data: { id: 'r1', state: 'running' } }, { status: 201 });
      }),
    );
    await user.dblClick(submit);
    await user.click(submit);
    release();
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(starts).toHaveBeenCalledTimes(1);
    expect(starts).toHaveBeenCalledWith({
      source: { exportId: 'x1' },
      passphrase: 'correct horse battery',
      mode: 'full',
      includeAudit: false,
    });
  });

  it('refuses from the plan and never offers the restore', async () => {
    server.use(
      http.post('*/api/instance-backups/restore/plan', () =>
        HttpResponse.json({
          success: true,
          data: { ...plan, refusals: ['newer_schema'] },
        }),
      ),
    );
    const user = userEvent.setup();
    const dialog = await openPlan(user);
    expect(
      await within(dialog).findByText('This export comes from a newer McHarbor. Upgrade this instance first.'),
    ).toBeInTheDocument();
    expect(within(dialog).queryByLabelText('Type RESTORE to confirm')).toBeNull();
    expect(within(dialog).getByRole('button', { name: 'Restore now' })).toBeDisabled();
  });

  it('says in words when the key is wrong, and keeps the dialog', async () => {
    server.use(
      http.post('*/api/instance-backups/restore/plan', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'err.invalid_body',
            data: { reason: 'wrong_key' },
          },
          { status: 400 },
        ),
      ),
    );
    const user = userEvent.setup();
    const dialog = await openPlan(user);
    expect(
      await within(dialog).findByText('That passphrase or key file does not open this export.'),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Check export' })).toBeInTheDocument();
  });

  it('renders restore disabled, naming the permission, without instance_backup.restore', async () => {
    grantPermissions(['instance_backup.view']);
    const user = userEvent.setup();
    renderWithProviders(<InstanceBackupPanel />);
    await screen.findByText('nightly');
    const restore = within(row('nightly')).getByRole('button', {
      name: 'Restore',
    });
    expect(restore).toBeDisabled();
    await user.hover(restore.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the instance_backup.restore permission')).not.toHaveLength(0);
  });

  it('puts a failed plugin reinstall behind its error', async () => {
    const status: RestoreStatus = {
      id: 'r1',
      state: 'done',
      actor: 'admin',
      mode: 'full',
      startedAt: '2026-09-26T10:00:00Z',
      result: { rows: 10, tables: 2 },
      plugins: [
        {
          slug: 'comm-ntfy',
          version: '2026.09.02',
          action: 'failed',
          error: 'version 2026.09.02 of comm-ntfy is no longer in the catalog',
        },
      ],
    };
    server.use(
      http.get('*/api/instance-backups/restore/status', () => HttpResponse.json({ success: true, data: status })),
    );
    const user = userEvent.setup();
    renderWithProviders(<InstanceBackupPanel />);
    expect(await screen.findByText('Restored')).toBeInTheDocument();
    expect(screen.queryByText(/no longer in the catalog/)).toBeNull();
    await user.click(screen.getByRole('button', { name: /error/i }));
    expect(await screen.findByText('version 2026.09.02 of comm-ntfy is no longer in the catalog')).toBeInTheDocument();
  });
});
