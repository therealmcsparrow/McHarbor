// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
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

function run(id: string, operation: 'backup' | 'restore', idShift?: string) {
  return {
    id,
    containerId: 'ct-1',
    environmentId: 'env-1',
    operation,
    status: 'success',
    kind: 'full',
    consistency: 'filesystem',
    bytesTotal: 1024,
    idShift,
    destinations: [],
    startedAt: '2026-09-27T20:56:00Z',
    durationMs: 283,
  };
}

function answerRuns(runs: ReturnType<typeof run>[]) {
  server.use(http.get('*/api/system-containers/ct-1/backups', () => HttpResponse.json({ success: true, data: runs })));
}

describe('addon-lxc restore ownership outcome', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'ct', ctBundle, true, true);
  });

  beforeEach(() => {
    grantPermissions(['ct.backup', 'ct.restore', 'ct.manage', 'ct.view']);
    server.use(
      http.get('*/api/system-containers/ct-1/backup-plans', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/storage-locations', () => HttpResponse.json({ success: true, data: [] })),
    );
  });

  it("says a restore moved ownership into this container's id range", async () => {
    answerRuns([run('r-1', 'restore', 'shifted')]);
    renderWithProviders(<BackupsPanel container={container} />);

    expect(await screen.findByText('Owners moved')).toBeInTheDocument();
    expect(screen.getByLabelText(/Ownership moved to this container's id range/)).toBeInTheDocument();
  });

  it('names a same-range restore and one whose owners were left as archived', async () => {
    answerRuns([run('r-1', 'restore', 'none'), run('r-2', 'restore', 'skipped')]);
    renderWithProviders(<BackupsPanel container={container} />);

    expect(await screen.findByText('Owners unchanged')).toBeInTheDocument();
    expect(screen.getByText('Owners not moved')).toBeInTheDocument();
  });

  it('shows nothing for a backup, or a restore the agent never reported on', async () => {
    answerRuns([run('b-1', 'backup', 'shifted'), run('r-1', 'restore')]);
    renderWithProviders(<BackupsPanel container={container} />);

    expect(await screen.findAllByText('Restore')).not.toHaveLength(0);
    expect(screen.queryByText('Owners moved')).not.toBeInTheDocument();
    expect(screen.queryByText('Owners unchanged')).not.toBeInTheDocument();
    expect(screen.queryByText('Owners not moved')).not.toBeInTheDocument();
  });
});
