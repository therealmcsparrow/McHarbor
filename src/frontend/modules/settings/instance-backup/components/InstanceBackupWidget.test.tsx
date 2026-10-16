// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { i18n, initTestI18n } from '../../../../test/i18n';
import widgetEn from '../../../../../../widgets/instance-backup-status/i18n/en.json';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import InstanceBackupStatusWidget, { freshness } from '@widgets/instance-backup-status/InstanceBackupStatusWidget';
import type { InstanceExport } from '../hooks/useInstanceBackups';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const base: InstanceExport = {
  id: 'x1',
  status: 'completed',
  trigger: 'schedule',
  createdBy: 'schedule',
  fileName: 'a',
  sizeBytes: 1,
  keyKind: 'passphrase',
  keyId: '',
  includeMetrics: false,
  note: '',
  error: '',
  verifyStatus: 'passed',
  verifyError: '',
  createdAt: '2026-09-26T00:00:00Z',
};

function widget() {
  return renderWithProviders(
    <MemoryRouter>
      <InstanceBackupStatusWidget colSpan={6} typeId="instance-backup-status" />
    </MemoryRouter>,
  );
}

describe('InstanceBackupStatusWidget', () => {
  beforeAll(async () => {
    await initTestI18n();
    i18n.addResourceBundle('en', 'dashboard', widgetEn, true, true);
  });

  it('calls an instance stale when its last completed export is older than the threshold', () => {
    const now = Date.parse('2026-09-26T12:00:00Z');
    expect(freshness([{ ...base, completedAt: '2026-09-26T03:00:00Z' }], now).stale).toBe(false);
    expect(freshness([{ ...base, completedAt: '2026-09-23T03:00:00Z' }], now).stale).toBe(true);
    expect(freshness([{ ...base, status: 'failed' }], now).stale).toBe(true);
    expect(freshness([], now).stale).toBe(true);
  });

  it('asks for nothing without instance_backup.view, and says which permission it needs', async () => {
    grantPermissions([]);
    const requests = vi.fn();
    server.use(
      http.get('*/api/instance-backups*', () => {
        requests();
        return HttpResponse.json({ success: true, data: [] });
      }),
    );
    widget();
    expect(await screen.findByText('Needs the instance_backup.view permission')).toBeInTheDocument();
    expect(requests).not.toHaveBeenCalled();
  });

  it('counts failing schedules and names the last export', async () => {
    grantPermissions(['instance_backup.view']);
    server.use(
      http.get('*/api/instance-backups', () =>
        HttpResponse.json({
          success: true,
          data: {
            items: [{ ...base, completedAt: new Date().toISOString(), verifiedAt: new Date().toISOString() }],
            total: 1,
            page: 1,
            per_page: 100,
          },
        }),
      ),
      http.get('*/api/instance-backups/schedules', () =>
        HttpResponse.json({
          success: true,
          data: [
            {
              id: 's2',
              name: 'hourly',
              cron: '0 * * * *',
              enabled: true,
              lastStatus: 'completed',
              lastError: '',
              lastRunAt: new Date().toISOString(),
              retentionCount: 7,
              retentionDays: 0,
            },
            {
              id: 's1',
              name: 'nightly',
              cron: '0 3 * * *',
              enabled: true,
              lastStatus: 'failed',
              lastError: 'x',
              retentionCount: 7,
              retentionDays: 0,
            },
          ],
        }),
      ),
    );
    widget();
    expect(await screen.findByText('nightly')).toBeInTheDocument();
    expect(screen.getByText('Last run failed')).toBeInTheDocument();
    expect(screen.getByText(/^Last export/)).toBeInTheDocument();
    expect(screen.getByText(/^Last verified/)).toBeInTheDocument();
    expect(screen.getByText('1')).toBeInTheDocument();
  });
});
