// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { BackupRunsPanel } from './BackupRunsPanel';

function run(overrides: Partial<ContainerBackupRun>): ContainerBackupRun {
  return {
    id: 'run',
    operation: 'backup',
    environmentId: 'env-1',
    containerId: 'c-1',
    status: 'success',
    archivePath: '/data/backups/run/archive.tar',
    archiveSize: 2048,
    destinations: [],
    includedItems: [],
    availableItems: [],
    startedAt: '2026-10-09T08:00:00Z',
    durationMs: 1000,
    createdAt: '2026-10-09T08:00:00Z',
    updatedAt: '2026-10-09T08:00:00Z',
    ...overrides,
  };
}

const uploaded = run({ id: 'run-up', origin: 'upload' });
const planned = run({
  id: 'run-plan',
  planName: 'nightly',
  destinations: [
    {
      id: 'dest-1',
      runId: 'run-plan',
      storageLocationName: 'Local',
      locationType: 'local',
      status: 'success',
      path: '/data/backups/run-plan',
      bytesUploaded: 1,
      bytesTotal: 1,
      createdAt: '2026-10-09T08:00:00Z',
      updatedAt: '2026-10-09T08:00:00Z',
    },
  ],
});

function mount(runs: ContainerBackupRun[]) {
  const onRestore = vi.fn();
  renderWithProviders(
    <BackupRunsPanel
      runs={runs}
      onRestore={onRestore}
      onDelete={vi.fn()}
      onCancel={vi.fn()}
      onRetryUpload={vi.fn()}
      onUploadRestore={vi.fn()}
      deletePending={false}
      cancelPending={false}
    />,
  );
  return { onRestore };
}

describe('BackupRunsPanel uploaded runs', () => {
  it('marks only a run with origin upload as Uploaded', () => {
    grantPermissions(['restore.run']);
    mount([uploaded, planned]);
    expect(screen.getAllByText('Uploaded')).toHaveLength(1);
    expect(screen.getByTestId('run-uploaded-run-up')).toBeInTheDocument();
    expect(screen.queryByTestId('run-uploaded-run-plan')).not.toBeInTheDocument();
  });

  // The bug this guards: a local-only run has no destination row, and the destination row
  // was the only place Restore lived, so an uploaded archive could not be restored again.
  it('offers Restore on a run with a stored archive and no destinations, and calls onRestore with it', async () => {
    grantPermissions(['restore.run']);
    const { onRestore } = mount([uploaded]);
    await userEvent.setup().click(screen.getByTestId('run-restore-run-up'));
    expect(onRestore).toHaveBeenCalledTimes(1);
    expect(onRestore).toHaveBeenCalledWith(expect.objectContaining({ id: 'run-up' }));
  });

  it('does not duplicate Restore when a destination already offers it', () => {
    grantPermissions(['restore.run']);
    mount([planned]);
    expect(screen.queryByTestId('run-restore-run-plan')).not.toBeInTheDocument();
  });

  it('offers no Restore without a stored archive or after a failure', () => {
    grantPermissions(['restore.run']);
    mount([
      run({ id: 'no-archive', origin: 'upload', archivePath: undefined }),
      run({ id: 'failed', origin: 'upload', status: 'failure' }),
    ]);
    expect(screen.queryByTestId('run-restore-no-archive')).not.toBeInTheDocument();
    expect(screen.queryByTestId('run-restore-failed')).not.toBeInTheDocument();
  });

  it('disables Restore and names restore.run when the permission is missing', async () => {
    grantPermissions([]);
    const { onRestore } = mount([uploaded]);
    const button = screen.getByTestId('run-restore-run-up');
    expect(button).toBeDisabled();
    await userEvent.setup().hover(button.parentElement ?? button);
    expect(await screen.findAllByText(/restore\.run/)).not.toHaveLength(0);
    expect(onRestore).not.toHaveBeenCalled();
  });
});
