// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { ScheduleList } from './ScheduleList';
import type { ExportSchedule } from '../hooks/useExportSchedules';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const nightly: ExportSchedule = {
  id: 's1',
  name: 'nightly',
  cron: '0 3 * * *',
  enabled: true,
  keyKind: 'passphrase',
  keyId: '',
  includeMetrics: false,
  locationIds: ['loc1'],
  retentionCount: 7,
  retentionDays: 0,
  verifyAfter: true,
  nextRunAt: '2026-09-27T03:00:00Z',
  lastRunAt: '2026-09-26T03:00:00Z',
  lastStatus: 'failed',
  lastError: 'copying to the storage location: sftp: permission denied',
  createdBy: 'admin',
  createdAt: '2026-09-20T00:00:00Z',
};

const deletes = vi.fn();
const saves = vi.fn();

function nightlyRow() {
  return screen.getByText('nightly').closest('tr') as HTMLElement;
}

describe('ScheduleList', () => {
  beforeEach(() => {
    deletes.mockReset();
    saves.mockReset();
    grantPermissions(['instance_backup.view', 'instance_backup.manage', 'storage_locations.view']);
    server.use(
      http.get('*/api/instance-backups/schedules', () => HttpResponse.json({ success: true, data: [nightly] })),
      http.get('*/api/storage-locations', () =>
        HttpResponse.json({
          success: true,
          data: [{ id: 'loc1', name: 'NAS', locationType: 'sftp', enabled: true }],
        }),
      ),
      http.delete('*/api/instance-backups/schedules/:id', ({ params }) => {
        deletes(params.id);
        return HttpResponse.json({
          success: true,
          data: { status: 'deleted' },
        });
      }),
      http.post('*/api/instance-backups/schedules', async ({ request }) => {
        saves(await request.json());
        return HttpResponse.json({ success: true, data: nightly }, { status: 201 });
      }),
    );
  });

  it('shows the schedule in words next to its cron, and the last failure behind its reason', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ScheduleList />);
    await screen.findByText('nightly');
    expect(within(nightlyRow()).getByText('0 3 * * *')).toBeInTheDocument();
    expect(within(nightlyRow()).getByText('Every day at 03:00')).toBeInTheDocument();
    expect(within(nightlyRow()).queryByText(/permission denied/)).toBeNull();
    await user.click(within(nightlyRow()).getByRole('button', { name: /error/i }));
    expect(await screen.findByText('copying to the storage location: sftp: permission denied')).toBeInTheDocument();
  });

  it('deletes only after confirmation, once, and keeps the dialog with the server message on failure', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ScheduleList />);
    await screen.findByText('nightly');
    await user.click(within(nightlyRow()).getByRole('button', { name: 'Delete' }));
    expect(deletes).not.toHaveBeenCalled();
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.delete('*/api/instance-backups/schedules/:id', async ({ params }) => {
        deletes(params.id);
        await held;
        return HttpResponse.json(
          {
            success: false,
            error: 'deleting schedule s1: database is locked',
          },
          { status: 500 },
        );
      }),
    );
    const confirm = screen.getByRole('button', { name: 'Delete schedule' });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('deleting schedule s1: database is locked')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete schedule' })).toBeInTheDocument();
  });

  it('renders every action disabled, naming the permission, without instance_backup.manage', async () => {
    grantPermissions(['instance_backup.view']);
    const user = userEvent.setup();
    renderWithProviders(<ScheduleList />);
    await screen.findByText('nightly');
    for (const name of ['Run now', 'Edit schedule', 'Delete']) {
      expect(within(nightlyRow()).getByRole('button', { name })).toBeDisabled();
    }
    const del = within(nightlyRow()).getByRole('button', { name: 'Delete' });
    await user.hover(del.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the instance_backup.manage permission')).not.toHaveLength(0);
  });

  it('narrows the list by the enabled, off and failing facets', async () => {
    const weekly: ExportSchedule = {
      ...nightly,
      id: 's2',
      name: 'weekly',
      cron: '0 4 * * 0',
      enabled: false,
      lastStatus: 'completed',
      lastError: '',
    };
    server.use(
      http.get('*/api/instance-backups/schedules', () => HttpResponse.json({ success: true, data: [nightly, weekly] })),
    );
    const user = userEvent.setup();
    renderWithProviders(<ScheduleList />);
    await screen.findByTestId('schedule-row-s2');
    await user.click(screen.getByRole('button', { name: /Last run failed/ }));
    expect(screen.getByTestId('schedule-row-s1')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-row-s2')).toBeNull();
    await user.click(screen.getByRole('button', { name: /^Off/ }));
    expect(screen.getByTestId('schedule-row-s2')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-row-s1')).toBeNull();
  });

  it('creates a schedule with its passphrase and retention', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ScheduleList />);
    await screen.findByText('nightly');
    await user.click(screen.getByRole('button', { name: /Add schedule/ }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Name'), 'weekly');
    const save = within(dialog).getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse battery');
    await user.click(save);
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves.mock.calls[0]?.[0]).toMatchObject({
      name: 'weekly',
      cron: '0 3 * * *',
      passphrase: 'correct horse battery',
      retentionCount: 7,
      verifyAfter: true,
      enabled: true,
    });
  });
});
