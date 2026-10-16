// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import DiagnosticsPage from '../pages/DiagnosticsPage';
import type { CheckResult, DiagnosticsReport, PreUpgradeReport } from '@resources/hooks/useSupport';
import { EncryptionKeyBanner, KEY_BANNER_DISMISSED } from './EncryptionKeyBanner';
import { PreUpgradeCheckDialog } from '@resources/components/support/PreUpgradeCheckDialog';

const check = (
  id: string,
  status: CheckResult['status'],
  reason: string,
  details?: Record<string, string>,
): CheckResult => ({
  id,
  status,
  reason,
  details,
  durationMs: 3,
});

const report = (checks: CheckResult[]): DiagnosticsReport => ({
  generatedAt: '2026-09-26T20:00:00Z',
  version: '2026.09.01',
  build: '1',
  checks,
});

describe('Diagnostics workbench', () => {
  const runs = vi.fn();
  beforeEach(() => {
    runs.mockReset();
    grantPermissions(['support.view']);
  });

  it('shows each verdict with its remediation, and re-runs a single check in place', async () => {
    server.use(
      http.get('*/api/support/diagnostics', ({ request }) => {
        const only = new URL(request.url).searchParams.get('check');
        runs(only);
        if (only === 'encryption_key')
          return HttpResponse.json({ success: true, data: report([check('encryption_key', 'pass', 'env')]) });
        return HttpResponse.json({
          success: true,
          data: report([
            check('database', 'pass', 'ok', { driver: 'sqlite' }),
            check('encryption_key', 'warn', 'auto_generated'),
          ]),
        });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <MemoryRouter>
        <DiagnosticsPage />
      </MemoryRouter>,
    );
    expect(await screen.findByText(/generated into DATA_DIR\/\.encryption_key/)).toBeTruthy();
    expect(screen.getByText('1 passed, 1 warnings, 0 failed')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Run Data encryption key' }));
    await waitFor(() =>
      expect(
        within(screen.getByTestId('check-encryption_key')).getByText(
          'The data encryption key comes from ENCRYPTION_KEY.',
        ),
      ).toBeTruthy(),
    );
    expect(runs).toHaveBeenLastCalledWith('encryption_key');
    expect(within(screen.getByTestId('check-database')).getByText('sqlite')).toBeTruthy();
  });

  it('asks for support.view instead of calling the server when it is missing', () => {
    grantPermissions([]);
    renderWithProviders(
      <MemoryRouter>
        <DiagnosticsPage />
      </MemoryRouter>,
    );
    expect(screen.getByText('Requires the support.view permission')).toBeTruthy();
    expect(runs).not.toHaveBeenCalled();
  });
});

describe('Pre-upgrade check', () => {
  const selfUpdates = vi.fn();
  const preUpgrade = (targets: string[]): PreUpgradeReport => ({
    currentVersion: '2026.09.01',
    targetVersion: targets[0] ?? '',
    databaseDriver: 'sqlite',
    checks: [check('plugin_compat', 'warn', 'incompatible', { plugins: 'addon-old' }), check('busy', 'pass', 'idle')],
    plugins: [{ slug: 'addon-old', version: '1.0.0', range: '>=2.0.0 <3.0.0', compatible: 'no' }],
  });

  beforeEach(() => {
    selfUpdates.mockReset();
    grantPermissions(['updates.view']);
    server.use(
      http.post('*/api/versions/self-update', () => {
        selfUpdates();
        return HttpResponse.json({ success: true });
      }),
    );
  });

  it('reports incompatible plugins and rollback guidance for the target, and changes nothing', async () => {
    const targets: string[] = [];
    server.use(
      http.get('*/api/support/pre-upgrade', ({ request }) => {
        targets.push(new URL(request.url).searchParams.get('target') ?? '');
        return HttpResponse.json({ success: true, data: preUpgrade(targets) });
      }),
    );
    renderWithProviders(<PreUpgradeCheckDialog open onOpenChange={() => undefined} target="2026.10.01" />);
    expect(await screen.findByText('Not compatible')).toBeTruthy();
    expect(screen.getByText(/Some plugins do not declare support for the target version/)).toBeTruthy();
    expect(screen.getByText(/SQLite migrations only move forward/)).toBeTruthy();
    expect(targets).toEqual(['2026.10.01']);
    expect(selfUpdates).not.toHaveBeenCalled();
  });
});

describe('Encryption key banner', () => {
  const lookups = vi.fn();
  function answer(source: 'env' | 'file') {
    server.use(
      http.get('*/api/support/encryption-key', () => {
        lookups();
        return HttpResponse.json({ success: true, data: { source, fileSeen: source === 'file' } });
      }),
    );
  }
  beforeEach(() => {
    lookups.mockReset();
    grantPermissions(['support.view']);
  });

  it('warns an admin when the key was generated into DATA_DIR, and stays dismissed', async () => {
    answer('file');
    const user = userEvent.setup();
    const { unmount } = renderWithProviders(
      <MemoryRouter>
        <EncryptionKeyBanner />
      </MemoryRouter>,
    );
    expect(await screen.findByText('The data encryption key only exists inside the data directory')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByText('The data encryption key only exists inside the data directory')).toBeNull();
    expect(localStorage.getItem(KEY_BANNER_DISMISSED)).toBe('1');
    unmount();
    lookups.mockReset();
    renderWithProviders(
      <MemoryRouter>
        <EncryptionKeyBanner />
      </MemoryRouter>,
    );
    expect(screen.queryByText('The data encryption key only exists inside the data directory')).toBeNull();
    expect(lookups).not.toHaveBeenCalled();
  });

  it('says nothing when the key comes from ENCRYPTION_KEY', async () => {
    answer('env');
    renderWithProviders(
      <MemoryRouter>
        <EncryptionKeyBanner />
      </MemoryRouter>,
    );
    await waitFor(() => expect(lookups).toHaveBeenCalled());
    expect(screen.queryByText('The data encryption key only exists inside the data directory')).toBeNull();
  });

  it('does not ask the server for users without support.view', () => {
    grantPermissions([]);
    answer('file');
    const { client } = renderWithProviders(
      <MemoryRouter>
        <EncryptionKeyBanner />
      </MemoryRouter>,
    );
    expect(client.isFetching()).toBe(0);
    expect(lookups).not.toHaveBeenCalled();
    expect(screen.queryByText('The data encryption key only exists inside the data directory')).toBeNull();
  });
});
