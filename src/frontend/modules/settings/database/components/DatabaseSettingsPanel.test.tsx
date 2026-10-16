// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { DatabaseSettingsPanel } from './DatabaseSettingsPanel';
import type { DatabaseConfig, DatabaseDriver } from '../types';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const tests = vi.fn();

function mount(driver: DatabaseDriver) {
  const config: DatabaseConfig = {
    driver,
    source: 'secret',
    target: 'redacted',
    envPinned: false,
    tables: 170,
    rows: 1200,
    configured: true,
    availableDrivers: ['sqlite', 'postgres', 'mysql', 'mssql'],
    restartSupported: true,
  };
  server.use(
    http.get('*/api/database/config', () => HttpResponse.json({ success: true, data: config })),
    http.get('*/api/database/check', () => HttpResponse.json({ success: true, data: null })),
    http.get('*/api/database/cleanup', () =>
      HttpResponse.json({ success: true, data: { available: false, totalBytes: 0, reason: 'none' } }),
    ),
    http.post('*/api/database/test', async ({ request }) => {
      tests(await request.json());
      return HttpResponse.json({
        success: true,
        data: { ok: false, driver: 'mssql', target: 'x', steps: [], existingTables: 0 },
      });
    }),
  );
  return renderWithProviders(<DatabaseSettingsPanel />);
}

describe('DatabaseSettingsPanel', () => {
  beforeEach(() => {
    tests.mockReset();
    grantPermissions(['database.view', 'database.manage']);
  });

  it('opens on the backend McHarbor is running on, marked in use', async () => {
    mount('mysql');
    const running = await screen.findByRole('radio', { name: /MySQL \/ MariaDB/ });
    expect(running).toBeChecked();
    expect(running.closest('label')).toHaveTextContent('In use');
    expect(screen.getByRole('radio', { name: /^PostgreSQL/ })).not.toBeChecked();
  });

  it('does not treat the in-use backend as a copy target until a move is asked for', async () => {
    const user = userEvent.setup();
    mount('postgres');
    expect(
      await screen.findByText(
        'McHarbor is running on PostgreSQL now. Pick a different backend above to copy every row to it, or move it to another PostgreSQL database.',
      ),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
    const testButton = screen.getByRole('button', { name: 'Test connection' });
    expect(testButton).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Migrate data' })).toBeDisabled();
    await user.click(testButton);
    expect(tests).not.toHaveBeenCalled();
  });

  it('moves a SQLite install to another file on request', async () => {
    const user = userEvent.setup();
    mount('sqlite');
    await user.click(await screen.findByRole('button', { name: 'Move to another SQLite database' }));
    expect(
      screen.getByText('Enter the new file or server. McHarbor refuses to copy onto the database it is running on.'),
    ).toBeInTheDocument();
    const path = screen.getByLabelText('File path');
    await user.clear(path);
    await user.type(path, '/mnt/fast/mcharbor.db');
    await user.click(screen.getByRole('button', { name: 'Test connection' }));
    await waitFor(() => expect(tests).toHaveBeenCalledTimes(1));
    expect(tests).toHaveBeenCalledWith(expect.objectContaining({ driver: 'sqlite', path: '/mnt/fast/mcharbor.db' }));
  });

  it('keeps the move button disabled without database.manage', async () => {
    grantPermissions(['database.view']);
    mount('sqlite');
    expect(await screen.findByRole('button', { name: /Move to another SQLite database/ })).toBeDisabled();
  });

  it('renders the SQL Server fields and submits them', async () => {
    const user = userEvent.setup();
    mount('sqlite');
    await user.click(await screen.findByRole('radio', { name: /Microsoft SQL Server/ }));

    expect(screen.getByRole('radio', { name: /Microsoft SQL Server/ })).toBeChecked();
    expect(screen.getByLabelText('Encryption')).toBeInTheDocument();
    expect(screen.getByText(/READ_COMMITTED_SNAPSHOT/)).toBeInTheDocument();

    await user.type(screen.getByLabelText('Host'), 'sql.local');
    await user.type(screen.getByLabelText('Database name'), 'mcharbor');
    await user.type(screen.getByLabelText('User'), 'app');
    await user.type(screen.getByLabelText('Password'), 'secret');
    await user.click(screen.getByRole('switch', { name: 'Trust the server certificate' }));
    await user.click(screen.getByRole('button', { name: 'Test connection' }));

    await waitFor(() => expect(tests).toHaveBeenCalledTimes(1));
    expect(tests.mock.calls[0]?.[0]).toMatchObject({
      driver: 'mssql',
      host: 'sql.local',
      port: 1433,
      name: 'mcharbor',
      user: 'app',
      password: 'secret',
      sslMode: 'true',
      trustServerCertificate: true,
    });
  });
});
