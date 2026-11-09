// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { BackupTempCard } from './BackupTempCard';

const status = (over: Record<string, unknown> = {}) => ({
  path: '/srv/spool/tmp',
  source: 'setting',
  settingPath: '/srv/spool/tmp',
  envOverride: false,
  envInvalidCode: '',
  defaultPath: '/var/lib/mcharbor/backups/tmp',
  sizeBytes: 2048,
  entries: 7,
  activeRuns: 0,
  retiredPaths: [],
  ...over,
});

describe('BackupTempCard', () => {
  beforeEach(() => {
    grantPermissions(['backups.manage']);
    server.use(
      http.get('*/api/about', () =>
        HttpResponse.json({ success: true, data: { version: '2026.11.02', native: true, sandboxed: false } }),
      ),
    );
  });

  it('shows the path, its source, the size and the entry count on load', async () => {
    server.use(
      http.get('*/api/container-backups/temp', () =>
        HttpResponse.json({ success: true, data: status({ retiredPaths: ['/old/a', '/old/b'] }) }),
      ),
    );
    renderWithProviders(<BackupTempCard />);
    expect(await screen.findByText('/srv/spool/tmp', { selector: 'code' })).toBeInTheDocument();
    expect(screen.getByText('Saved setting')).toBeInTheDocument();
    expect(screen.getByText('2 KB')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
    expect(screen.getByText('2 earlier locations may still hold leftovers.')).toBeInTheDocument();
  });

  it('a 403 shows an alert naming backups.manage as a global grant, not the controls', async () => {
    server.use(
      http.get('*/api/container-backups/temp', () =>
        HttpResponse.json({ success: false, error: 'Forbidden', code: 'forbidden' }, { status: 403 }),
      ),
    );
    renderWithProviders(<BackupTempCard />);
    expect(await screen.findByText(/backups\.manage as a global grant/)).toBeInTheDocument();
    expect(screen.queryByLabelText('Location')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();
  });

  it('shows the ignored-environment-value warning when the env var is invalid', async () => {
    server.use(
      http.get('*/api/container-backups/temp', () =>
        HttpResponse.json({
          success: true,
          data: status({ source: 'default', envInvalidCode: 'backup_temp_relative' }),
        }),
      ),
    );
    renderWithProviders(<BackupTempCard />);
    expect(await screen.findByText('Environment value ignored')).toBeInTheDocument();
  });
});
