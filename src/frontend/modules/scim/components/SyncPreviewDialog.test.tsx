// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { describeDestructiveDialog } from '../../../test/destructive';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { SyncPreviewDialog } from './SyncPreviewDialog';
import type { DryRunResult } from '../hooks/useEndpoints';

const plan = (overrides: Partial<NonNullable<DryRunResult['plan']>> = {}): DryRunResult => ({
  status: 'success',
  plan: {
    endpointId: 'e1',
    generatedAt: '2026-09-26T00:00:00Z',
    users: [
      {
        action: 'disable',
        scimId: 'd1',
        userName: 'dave',
        localUserId: 'u-d',
        localUsername: 'dave',
        reason: 'removed',
      },
      { action: 'update', scimId: 'a1', userName: 'alice', localUsername: 'alice', fields: ['email'] },
    ],
    groups: [{ scimId: 'g1', displayName: 'Ops', members: 2, localGroupId: 'lg' }],
    summary: {
      usersSeen: 7,
      linked: 0,
      unmatched: 0,
      updated: 1,
      disabled: 1,
      skipped: 0,
      unchanged: 5,
      groupsSeen: 1,
      pendingLink: 0,
    },
    blocked: false,
    ...overrides,
  },
});

function answerDryRun(result: DryRunResult) {
  return http.post('*/api/scim/endpoints/e1/dry-run', () => HttpResponse.json({ success: true, data: result }));
}

describeDestructiveDialog({
  name: 'SyncPreviewDialog',
  method: 'post',
  path: '*/api/scim/endpoints/e1/sync-now',
  permission: 'scim.manage',
  confirmLabel: 'Run sync now',
  handlers: [answerDryRun(plan())],
  render: ({ onOpenChange, onSuccess }) => (
    <SyncPreviewDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

describe('SyncPreviewDialog plan', () => {
  it('lists what the sync would change, with reasons', async () => {
    grantPermissions(['scim.manage']);
    server.use(answerDryRun(plan()));
    renderWithProviders(<SyncPreviewDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={vi.fn()} />);
    const changes = await screen.findByRole('list', { name: 'Changes' });
    expect(within(changes).getByText('dave')).toBeInTheDocument();
    expect(within(changes).getByText(/no longer in the directory/)).toBeInTheDocument();
    expect(within(changes).getByText(/changes email/)).toBeInTheDocument();
  });

  it('refuses to run a blocked sync', async () => {
    grantPermissions(['scim.manage']);
    server.use(answerDryRun(plan({ blocked: true, blockedReason: 'empty_directory' })));
    renderWithProviders(<SyncPreviewDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={vi.fn()} />);
    expect(await screen.findByText(/cannot disable every account at once/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run sync now' })).toBeDisabled();
  });

  it('shows the directory error in full', async () => {
    grantPermissions(['scim.manage']);
    const message =
      'pull users: scim endpoint returned 401 while reading https://idp.example.com/scim/v2/Users?startIndex=1&count=100';
    server.use(answerDryRun({ status: 'failed', error: message }));
    renderWithProviders(<SyncPreviewDialog endpoint={{ id: 'e1', name: 'Corp IdP' }} onOpenChange={vi.fn()} />);
    expect(await screen.findByText(message)).toBeInTheDocument();
  });
});
