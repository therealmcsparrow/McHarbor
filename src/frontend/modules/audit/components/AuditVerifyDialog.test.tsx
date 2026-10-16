// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuditVerifyDialog } from './AuditVerifyDialog';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const passed = {
  verified: true,
  complete: true,
  keyConfigured: true,
  totalRows: 12,
  checkedRows: 12,
  hashedRows: 12,
  unsealedRows: 0,
  truncated: false,
  checkedAt: '2026-09-26T10:00:00Z',
  anchors: [
    {
      id: 'a1',
      timestamp: '2026-09-20T08:00:00Z',
      actor: 'local:root@harbor',
      details: '{"preRestoreTerminalHash":"abc"}',
    },
  ],
  prunedGaps: 3,
  prunedRows: 41,
  tombstones: 4,
  invalidTombstones: 0,
};

describe('AuditVerifyDialog', () => {
  const runs = vi.fn();

  beforeEach(() => {
    runs.mockReset();
    grantPermissions(['audit.view']);
  });

  it('does not verify until asked, then reports the restore anchors and the retention gaps it crossed', async () => {
    server.use(
      http.get('*/api/audit/verify', () => {
        runs();
        return HttpResponse.json({ success: true, data: passed });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<AuditVerifyDialog open onOpenChange={() => {}} />);
    expect(runs).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Run verification' }));
    expect(await screen.findByText('Chain verified')).toBeInTheDocument();
    expect(screen.getByText(/^local:root@harbor at /)).toBeInTheDocument();
    expect(screen.getByText('41')).toBeInTheDocument();
    expect(screen.getByText('Retention gaps crossed')).toBeInTheDocument();
    expect(runs).toHaveBeenCalledTimes(1);
  });

  it('reports a broken chain with the first broken row and the reason', async () => {
    server.use(
      http.get('*/api/audit/verify', () =>
        HttpResponse.json({
          success: true,
          data: { ...passed, verified: false, firstBrokenId: 'row-77', reason: 'previous hash mismatch', anchors: [] },
        }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<AuditVerifyDialog open onOpenChange={() => {}} />);
    await user.click(screen.getByRole('button', { name: 'Run verification' }));
    expect(await screen.findByText('Chain broken')).toBeInTheDocument();
    expect(screen.getByText(/First broken row: row-77/)).toBeInTheDocument();
    expect(screen.getByText(/Reason: previous hash mismatch/)).toBeInTheDocument();
    expect(screen.getByText('The chain passes through no instance restore.')).toBeInTheDocument();
  });

  it('shows the server message when verification cannot run', async () => {
    server.use(
      http.get('*/api/audit/verify', () =>
        HttpResponse.json({ success: false, error: 'database is locked while verifying' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<AuditVerifyDialog open onOpenChange={() => {}} />);
    await user.click(screen.getByRole('button', { name: 'Run verification' }));
    expect(await screen.findByText('Verification could not run')).toBeInTheDocument();
    expect(screen.getByText('database is locked while verifying')).toBeInTheDocument();
  });

  it('renders the run button disabled, naming audit.view, without the permission', async () => {
    grantPermissions([]);
    server.use(
      http.get('*/api/audit/verify', () => {
        runs();
        return HttpResponse.json({ success: true, data: passed });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<AuditVerifyDialog open onOpenChange={() => {}} />);
    const run = screen.getByRole('button', { name: 'Run verification' });
    expect(run).toBeDisabled();
    await user.hover(run.parentElement ?? run);
    expect(await screen.findAllByText('Requires the audit.view permission')).not.toHaveLength(0);
    await waitFor(() => expect(runs).not.toHaveBeenCalled());
  });
});
