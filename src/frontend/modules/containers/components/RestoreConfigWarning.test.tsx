// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { RestoreRunDialog } from './RestoreRunDialog';

const run: ContainerBackupRun = {
  id: 'run-1',
  operation: 'backup',
  environmentId: 'env-1',
  containerId: 'c1',
  status: 'success',
  archivePath: '/backups/run-1/mcharbor.tar',
  archiveSize: 1024,
  destinations: [],
  startedAt: '2026-10-06T18:00:00Z',
  durationMs: 1000,
  createdAt: '2026-10-06T18:00:00Z',
  updatedAt: '2026-10-06T18:00:01Z',
};

function options(items: { key: string; type: string; label: string }[]) {
  server.use(
    http.post('*/api/container-backups/runs/:id/restore-options', () =>
      HttpResponse.json({
        success: true,
        data: { runId: 'run-1', items: items.map((i) => ({ description: '', default: true, required: false, ...i })) },
      }),
    ),
  );
}

describe('restoring container configuration', () => {
  beforeEach(() => grantPermissions(['restore.view', 'restore.run']));

  // Configuration is applied by recreating the container; the operator is told
  // before pressing Restore, not after the container was replaced.
  it('warns that the container is recreated when configuration is part of the restore', async () => {
    options([
      { key: 'config', type: 'config', label: 'Container configuration' },
      { key: 'mount:/data', type: 'mount', label: '/data' },
    ]);
    renderWithProviders(<RestoreRunDialog run={run} containerId="c1" onClose={vi.fn()} />);
    expect(await screen.findByText('This recreates the container')).toBeInTheDocument();
  });

  it('does not warn when the backup holds no configuration', async () => {
    options([{ key: 'mount:/data', type: 'mount', label: '/data' }]);
    renderWithProviders(<RestoreRunDialog run={run} containerId="c1" onClose={vi.fn()} />);
    expect(await screen.findByText('/data')).toBeInTheDocument();
    expect(screen.queryByText('This recreates the container')).not.toBeInTheDocument();
  });
});
