// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it } from 'vitest';
import { BackupsPanel } from '../../../plugins/Plugins/addon-lxc/frontend/src/components/BackupsPanel';
import type { Container } from '../../../plugins/Plugins/addon-lxc/frontend/src/types';
import ctBundle from '../../../plugins/Plugins/addon-lxc/i18n/en/ct.json';
import { i18n } from '../i18n';
import { grantPermissions } from '../permissions';
import { renderWithProviders } from '../render';
import { server } from '../server';

const container = {
  id: 'ct-1',
  environmentId: 'env-1',
  name: 'web',
  state: 'stopped',
  hostAvailable: true,
  isTemplate: false,
} as unknown as Container;

const keyMissing = 'Default local backup: storage stream: no backup encryption key is installed on this host';

function failedRun(errorHint?: string) {
  return {
    id: 'run-1',
    containerId: 'ct-1',
    environmentId: 'env-1',
    operation: 'backup',
    status: 'failure',
    kind: 'full',
    consistency: 'filesystem',
    bytesTotal: 0,
    error: keyMissing,
    errorHint,
    destinations: [
      {
        id: 'd1',
        runId: 'run-1',
        locationId: 'default-local-backup',
        locationName: 'Default local backup',
        part: 'manifest',
        status: 'failure',
        error: 'storage stream: no backup encryption key is installed on this host',
      },
    ],
    startedAt: '2026-09-27T16:01:31Z',
    durationMs: 620,
  };
}

function answerRuns(run: ReturnType<typeof failedRun>) {
  server.use(http.get('*/api/system-containers/ct-1/backups', () => HttpResponse.json({ success: true, data: [run] })));
}

async function openError() {
  const button = await screen.findByRole('button', { name: 'View the error' });
  await userEvent.click(button);
  return screen.findByRole('dialog');
}

describe('addon-lxc backup run error', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'ct', ctBundle, true, true);
  });

  beforeEach(() => {
    grantPermissions(['ct.backup', 'ct.restore', 'ct.manage', 'ct.view']);
    server.use(
      http.get('*/api/system-containers/ct-1/backup-plans', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/storage-locations', () =>
        HttpResponse.json({
          success: true,
          data: [{ id: 'default-local-backup', name: 'Default local backup', type: 'local' }],
        }),
      ),
    );
  });

  it('shows the destination reason and names the page that fixes a missing key', async () => {
    answerRuns(failedRun('backup_key_missing'));
    renderWithProviders(<BackupsPanel container={container} />);

    const dialog = await openError();
    expect(within(dialog).getByText(keyMissing)).toBeInTheDocument();
    expect(
      within(dialog).getByText(/Install one under Settings → Storage → Backup encryption key/),
    ).toBeInTheDocument();
  });

  it('keeps the generic description when there is no known fix', async () => {
    answerRuns(failedRun());
    renderWithProviders(<BackupsPanel container={container} />);

    const dialog = await openError();
    expect(within(dialog).getByText(keyMissing)).toBeInTheDocument();
    expect(within(dialog).queryByText(/Backup encryption key, then run/)).not.toBeInTheDocument();
  });
});
