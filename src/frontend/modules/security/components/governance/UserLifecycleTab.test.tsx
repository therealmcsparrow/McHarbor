// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { UserLifecycleTab } from './UserLifecycleTab';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

type Body = { action: string; userIds: string[]; dryRun: boolean; reason: string };

function lifecycleUser(id: string, username: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    username,
    displayName: null,
    authProvider: 'local',
    disabled: false,
    lockedUntil: null,
    locked: false,
    mfaEnabled: true,
    lastLogin: null,
    inactive: false,
    lastElevatedAt: null,
    recentlyElevated: false,
    breakGlass: false,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

const users = [
  lifecycleUser('u1', 'alice'),
  lifecycleUser('u2', 'bob', { locked: true, authProvider: 'oidc' }),
  lifecycleUser('u3', 'carol', { mfaEnabled: false }),
];

const report = {
  inactiveDays: 90,
  elevatedDays: 7,
  counts: { total: 3, disabled: 0, locked: 1, noMfa: 1, inactive: 0, recentlyElevated: 0 },
  users,
};

describe('UserLifecycleTab', () => {
  const previews = vi.fn();

  beforeEach(() => {
    previews.mockReset();
    grantPermissions(['account.manage', 'users.manage']);
    server.use(
      http.get('*/api/table-views/user-lifecycle', () =>
        HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
      ),
      http.get('*/api/users/lifecycle', () => HttpResponse.json({ success: true, data: report })),
      http.post('*/api/users/bulk/account', async ({ request }) => {
        const body = (await request.json()) as Body;
        previews(body);
        return HttpResponse.json({
          success: true,
          data: {
            action: body.action,
            dryRun: true,
            items: body.userIds.map((userId) => ({ userId, username: userId, outcome: 'would_apply' })),
            summary: { total: body.userIds.length, applied: body.userIds.length, skipped: 0, failed: 0 },
          },
        });
      }),
    );
  });

  it('renders the users through TablePage and narrows them with the search field', async () => {
    const user = userEvent.setup();
    renderWithProviders(<UserLifecycleTab />);
    expect(await screen.findByTestId('lifecycle-user-u1')).toBeInTheDocument();
    expect(screen.getByTestId('lifecycle-user-u2')).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText('Search users'), 'carol');
    await waitFor(() => expect(screen.queryByTestId('lifecycle-user-u1')).not.toBeInTheDocument());
    expect(screen.getByTestId('lifecycle-user-u3')).toBeInTheDocument();
  });

  it('narrows the rows to a lifecycle state through its facet', async () => {
    const user = userEvent.setup();
    renderWithProviders(<UserLifecycleTab />);
    await screen.findByTestId('lifecycle-user-u1');

    await user.click(within(screen.getByRole('group', { name: 'Filters' })).getByRole('button', { name: /Locked/ }));
    await waitFor(() => expect(screen.queryByTestId('lifecycle-user-u1')).not.toBeInTheDocument());
    expect(screen.getByTestId('lifecycle-user-u2')).toBeInTheDocument();
    expect(screen.queryByTestId('lifecycle-user-u3')).not.toBeInTheDocument();
  });

  it('sends exactly the selected users to the bulk preview from the batch action', async () => {
    const user = userEvent.setup();
    renderWithProviders(<UserLifecycleTab />);
    await user.click(within(await screen.findByTestId('lifecycle-user-u1')).getByRole('checkbox'));
    await user.click(within(screen.getByTestId('lifecycle-user-u3')).getByRole('checkbox'));
    expect(previews).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Disable' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Disable 2 users?')).toBeInTheDocument();
    await waitFor(() => expect(previews).toHaveBeenCalledTimes(1));
    expect(previews.mock.calls[0]?.[0]).toMatchObject({ action: 'disable', dryRun: true, userIds: ['u1', 'u3'] });
  });

  it('keeps a batch action the user may not run visible but disabled', async () => {
    grantPermissions(['account.manage']);
    const user = userEvent.setup();
    renderWithProviders(<UserLifecycleTab />);
    await user.click(within(await screen.findByTestId('lifecycle-user-u1')).getByRole('checkbox'));

    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Disable' })).toBeEnabled();
  });
});
