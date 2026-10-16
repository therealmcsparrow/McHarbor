// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it } from 'vitest';
import { useAuth, type User } from '@core/auth/useAuth';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import ApprovalsPage from './ApprovalsPage';

const request = (id: string, requesterId: string, permissionKey: string) => ({
  id,
  requesterId,
  permissionKey,
  justification: 'incident',
  requestedMinutes: 30,
  status: 'pending',
  createdAt: '2026-09-26T10:00:00Z',
});

describe('On-call approvals', () => {
  afterEach(() => useAuth.setState({ user: null }));

  it('never offers the viewer their own elevation to approve', async () => {
    grantPermissions(['users.manage']);
    useAuth.setState({ user: { id: 'me', username: 'me' } as User });
    server.use(
      http.get('*/api/elevations/pending', () =>
        HttpResponse.json({
          success: true,
          data: { requests: [request('r1', 'me', 'containers.delete'), request('r2', 'other', 'stacks.delete')] },
        }),
      ),
      http.get('*/api/users', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/environments', () => HttpResponse.json({ success: true, data: [] })),
    );
    renderWithProviders(
      <MemoryRouter>
        <ApprovalsPage />
      </MemoryRouter>,
    );
    expect(await screen.findByText(/stacks\.delete/)).toBeInTheDocument();
    expect(screen.queryByText(/containers\.delete/)).toBeNull();
  });
});
