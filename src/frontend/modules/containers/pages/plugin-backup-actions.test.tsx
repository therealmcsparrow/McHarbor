// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { BackupRunRow } from './backup-run-rows';
import { PluginRunActionsCell } from './plugin-backup-actions';

const actions = {
  plugin: 'vm-plugin',
  kind: 'backup-row-actions',
  sourceKind: 'vm',
  pages: {
    open: '/plugins/vm-plugin/{subject}/backups',
    restore: '/plugins/vm-plugin/{subject}/backups/restore/{id}',
    edit_plan: '/plugins/vm-plugin/{subject}/backups/plan/{id}',
  },
  routes: { delete_run: '/api/guests/backups/{id}', delete_plan: '/api/guests/backup-plans/{id}' },
  permissions: {
    view: 'guest.view',
    manage: 'guest.backup.manage',
    delete: 'guest.backup.delete',
    restore: 'guest.restore.run',
  },
};

// unavoidable cast: the cell reads only the fields set here, the rest of the row is table plumbing.
const run = {
  id: 'run-1',
  containerId: 'guest-7',
  sourceKind: 'vm',
  sourceSlug: 'vm-plugin',
  operation: 'backup',
  status: 'success',
} as BackupRunRow;

function Where() {
  return <p data-testid="where">{useLocation().pathname}</p>;
}

function renderCell(row: BackupRunRow = run) {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/backups']}>
      <Routes>
        <Route path="/backups" element={<PluginRunActionsCell run={row} />} />
        <Route path="*" element={<Where />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('PluginRunActionsCell', () => {
  const deleted = vi.fn();

  beforeEach(() => {
    deleted.mockReset();
    grantPermissions(['guest.view', 'guest.backup.delete', 'guest.restore.run']);
    server.use(
      http.delete('*/api/guests/backups/:id', ({ params }) => {
        deleted(params.id);
        return HttpResponse.json({ success: true });
      }),
    );
  });
  afterEach(() => clearPluginCapabilities());

  it('restores through the page the owning plugin declares', async () => {
    givePluginCapabilities([actions]);
    const user = userEvent.setup();
    renderCell();
    await user.click(screen.getByRole('button', { name: 'Restore backup' }));
    expect(screen.getByTestId('where')).toHaveTextContent('/plugins/vm-plugin/guest-7/backups/restore/run-1');
  });

  it('deletes through the declared route only after confirmation', async () => {
    givePluginCapabilities([actions]);
    const user = userEvent.setup();
    renderCell();
    await user.click(screen.getByRole('button', { name: 'Delete backup' }));
    expect(deleted).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Delete backup' }));
    await waitFor(() => expect(deleted).toHaveBeenCalledWith('run-1'));
  });

  it('renders nothing when the plugin is gone or the row belongs to another plugin', () => {
    givePluginCapabilities([actions], ['vm-plugin']);
    const gone = renderCell();
    expect(screen.queryAllByRole('button')).toHaveLength(0);
    gone.unmount();

    givePluginCapabilities([actions]);
    renderCell({ ...run, sourceSlug: 'other-plugin' });
    expect(screen.queryAllByRole('button')).toHaveLength(0);
  });
});
